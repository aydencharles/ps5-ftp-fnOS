package ftpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"syscall"
	"time"

	ftp "github.com/jlaffaye/ftp"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
)

const probeTimeout = 15 * time.Second

// ProbeCheck is one observable stage of a connection test. Keeping these
// stages explicit lets the UI show where a connection stopped instead of
// exposing an undifferentiated network error.
type ProbeCheck struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// ProbeFailure contains a stable machine-readable code and user-facing
// recovery guidance. Detail retains the original error for troubleshooting.
type ProbeFailure struct {
	Stage       string   `json:"stage"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Message     string   `json:"message"`
	Detail      string   `json:"detail,omitempty"`
	Suggestions []string `json:"suggestions"`
}

type ProbeResult struct {
	OK         bool          `json:"ok"`
	Checks     []ProbeCheck  `json:"checks"`
	Failure    *ProbeFailure `json:"failure,omitempty"`
	DurationMS int64         `json:"duration_ms"`
	TestedAt   time.Time     `json:"tested_at"`
}

func newProbeResult() ProbeResult {
	return ProbeResult{Checks: []ProbeCheck{
		{ID: "resolve", Status: "pending"},
		{ID: "connect", Status: "pending"},
		{ID: "authenticate", Status: "pending"},
		{ID: "directory", Status: "pending"},
	}}
}

func passCheck(result *ProbeResult, index int, started time.Time, detail string) {
	result.Checks[index].Status = "passed"
	result.Checks[index].Detail = detail
	result.Checks[index].DurationMS = time.Since(started).Milliseconds()
}

func failProbe(result *ProbeResult, index int, started, probeStarted time.Time, failure ProbeFailure) ProbeResult {
	result.Checks[index].Status = "failed"
	result.Checks[index].Detail = failure.Message
	result.Checks[index].DurationMS = time.Since(started).Milliseconds()
	for i := index + 1; i < len(result.Checks); i++ {
		result.Checks[i].Status = "skipped"
	}
	result.Failure = &failure
	result.DurationMS = time.Since(probeStarted).Milliseconds()
	result.TestedAt = time.Now().UTC()
	return *result
}

// Probe verifies the complete path required by the application: resolving the
// host, receiving an FTP greeting, logging in, and listing the configured base
// directory through a passive data connection.
func Probe(parent context.Context, p domain.Profile) ProbeResult {
	started := time.Now()
	result := newProbeResult()
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()

	host := strings.Trim(strings.TrimSpace(p.Host), "[]")
	stepStarted := time.Now()
	addresses, err := resolveHost(ctx, host)
	if err != nil {
		return failProbe(&result, 0, stepStarted, started, diagnoseProbeError("resolve", p, err))
	}
	passCheck(&result, 0, stepStarted, strings.Join(addresses, "、"))

	stepStarted = time.Now()
	address := net.JoinHostPort(host, strconv.Itoa(p.Port))
	conn, err := dialForProbe(ctx, address)
	if err != nil {
		return failProbe(&result, 1, stepStarted, started, diagnoseProbeError("connect", p, err))
	}
	client := &Client{conn: conn}
	defer client.Close()
	passCheck(&result, 1, stepStarted, "FTP 服务已响应")

	stepStarted = time.Now()
	username := p.Username
	if username == "" {
		username = "anonymous"
	}
	if err = conn.Login(username, p.Password); err != nil {
		return failProbe(&result, 2, stepStarted, started, diagnoseProbeError("authenticate", p, err))
	}
	passCheck(&result, 2, stepStarted, fmt.Sprintf("已以 %s 登录", username))

	stepStarted = time.Now()
	entries, err := client.List(p.BasePath)
	if err != nil {
		return failProbe(&result, 3, stepStarted, started, diagnoseProbeError("directory", p, err))
	}
	passCheck(&result, 3, stepStarted, fmt.Sprintf("%s 可访问，共 %d 项", p.BasePath, len(entries)))

	result.OK = true
	result.DurationMS = time.Since(started).Milliseconds()
	result.TestedAt = time.Now().UTC()
	return result
}

func dialForProbe(ctx context.Context, address string) (*ftp.ServerConn, error) {
	dialer := net.Dialer{Timeout: remainingTimeout(ctx)}
	dial := func(network, target string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, target)
		if err != nil {
			return nil, err
		}
		if deadline, ok := ctx.Deadline(); ok {
			if err = conn.SetDeadline(deadline); err != nil {
				_ = conn.Close()
				return nil, err
			}
		}
		return conn, nil
	}
	return ftp.Dial(address, ftp.DialWithDialFunc(dial), ftp.DialWithContext(ctx), ftp.DialWithDisabledEPSV(false))
}

func resolveHost(ctx context.Context, host string) ([]string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}, nil
	}
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, &net.DNSError{Name: host, IsNotFound: true}
	}
	if len(addresses) > 3 {
		addresses = addresses[:3]
	}
	return addresses, nil
}

func remainingTimeout(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			return remaining
		}
	}
	return probeTimeout
}

func diagnoseProbeError(stage string, p domain.Profile, err error) ProbeFailure {
	failure := ProbeFailure{Stage: stage, Detail: err.Error()}
	address := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) || isTimeout(err) {
		failure.Code = "timeout"
		failure.Title = "连接超时"
		failure.Message = fmt.Sprintf("在 15 秒内未能完成对 %s 的检测。", address)
		failure.Suggestions = []string{"确认 PS5 处于开机状态且 FTP 服务仍在运行", "确认 PS5 与 NAS 位于可互通的局域网", "检查路由器、防火墙或访客网络是否阻止了设备互访"}
		return failure
	}

	if stage == "resolve" {
		failure.Code = "dns_failed"
		failure.Title = "无法解析主机地址"
		failure.Message = fmt.Sprintf("NAS 无法将“%s”解析为可连接的 IP 地址。", p.Host)
		failure.Suggestions = []string{"检查 IP 地址或主机名是否填写正确", "建议直接填写 PS5 当前的局域网 IPv4 地址", "如果使用主机名，请确认 NAS 的 DNS 能解析该名称"}
		return failure
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		failure.Code = "connection_refused"
		failure.Title = "FTP 端口拒绝连接"
		failure.Message = fmt.Sprintf("已找到目标设备，但 %s 没有接受 FTP 连接。", address)
		failure.Suggestions = []string{"在 PS5 上启动 zftpd 或 ftpsrv", "核对端口：zftpd 通常为 2120，ftpsrv 通常为 2121", "确认填写的是 FTP 端口，而不是其他服务端口"}
		return failure
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		failure.Code = "network_unreachable"
		failure.Title = "目标网络不可达"
		failure.Message = fmt.Sprintf("NAS 当前没有可用路由到达 %s。", address)
		failure.Suggestions = []string{"确认 PS5 和 NAS 连接到同一局域网", "检查 PS5 的 IP 地址是否在休眠、重连网络后发生变化", "关闭会隔离局域网设备的访客网络或 AP 隔离"}
		return failure
	}

	var ftpError *textproto.Error
	if errors.As(err, &ftpError) {
		switch {
		case stage == "authenticate" && (ftpError.Code == ftp.StatusNotLoggedIn || ftpError.Code == ftp.StatusInvalidCredentials):
			failure.Code = "authentication_failed"
			failure.Title = "FTP 登录被拒绝"
			failure.Message = "FTP 服务已连接，但用户名或密码未通过验证。"
			failure.Suggestions = []string{"核对该 FTP 服务配置的用户名和密码", "无需账号的 PS5 FTP 服务通常可使用 anonymous", "如果刚修改了账号，请保存配置后重新检测"}
			return failure
		case stage == "directory" && ftpError.Code == ftp.StatusFileUnavailable:
			failure.Code = "base_path_unavailable"
			failure.Title = "基础目录不可访问"
			failure.Message = fmt.Sprintf("登录成功，但 FTP 服务无法列出基础目录 %s。", p.BasePath)
			failure.Suggestions = []string{"确认基础目录存在且使用以 / 开头的绝对路径", "尝试先将基础目录改为 /", "确认当前 FTP 账号拥有该目录的访问权限"}
			return failure
		case stage == "directory" && (ftpError.Code == ftp.StatusCanNotOpenDataConnection || ftpError.Code == ftp.StatusTransfertAborted):
			failure.Code = "passive_mode_failed"
			failure.Title = "被动模式数据连接失败"
			failure.Message = "FTP 登录成功，但目录列表所需的数据连接无法建立。"
			failure.Suggestions = []string{"确认 NAS 与 PS5 之间的防火墙允许 FTP 被动数据连接", "避免让两台设备分处互相隔离的 VLAN 或访客网络", "重启 PS5 上的 FTP 服务后再次检测"}
			return failure
		case ftpError.Code == ftp.StatusNotAvailable:
			failure.Code = "service_unavailable"
			failure.Title = "FTP 服务暂不可用"
			failure.Message = "目标端返回 421，并主动关闭了 FTP 会话。"
			failure.Suggestions = []string{"确认 PS5 上的 FTP 服务仍在运行", "稍后重试或重启 FTP 服务", "检查 FTP 服务是否限制了同时连接数"}
			return failure
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		failure.Code = "connection_closed"
		failure.Title = "FTP 服务提前断开连接"
		failure.Message = "目标端在完成检测前关闭了 FTP 连接。"
		failure.Suggestions = []string{"确认端口对应的是 FTP 服务", "重启 PS5 上的 FTP 服务", "检查 FTP 服务日志中是否存在崩溃或连接限制"}
		return failure
	}

	if stage == "authenticate" {
		failure.Code = "authentication_failed"
		failure.Title = "FTP 登录失败"
		failure.Message = "FTP 服务已连接，但未能完成用户登录。"
		failure.Suggestions = []string{"核对用户名和密码", "确认 FTP 服务支持标准 USER/PASS 登录", "重启 PS5 上的 FTP 服务后重试"}
		return failure
	}
	if stage == "directory" {
		failure.Code = "directory_check_failed"
		failure.Title = "目录访问检测失败"
		failure.Message = fmt.Sprintf("登录成功，但无法读取基础目录 %s。", p.BasePath)
		failure.Suggestions = []string{"确认基础目录存在并可访问", "尝试将基础目录改为 /", "检查 FTP 被动模式的数据连接是否被防火墙拦截"}
		return failure
	}
	failure.Code = "connection_failed"
	failure.Title = "无法建立 FTP 会话"
	failure.Message = fmt.Sprintf("%s 已响应网络请求，但没有完成标准 FTP 连接。", address)
	failure.Suggestions = []string{"确认主机和端口指向 PS5 FTP 服务", "核对 zftpd 或 ftpsrv 的实际监听端口", "重启 FTP 服务后重新检测"}
	return failure
}

func isTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
