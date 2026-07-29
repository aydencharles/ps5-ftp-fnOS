package web

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
)

const (
	maxEntryNameLength = 255
)

type profileRequest struct {
	Name     string `json:"name" validate:"required,max=100"`
	Host     string `json:"host" validate:"required,max=255,ftp_host"`
	Port     int    `json:"port" validate:"required,gte=1,lte=65535"`
	Username string `json:"username" validate:"max=255"`
	Password string `json:"password" validate:"max=1024"`
	BasePath string `json:"base_path" validate:"required,max=4096"`
	Preset   string `json:"preset" validate:"required,oneof=zftpd ftpsrv custom"`
}

func (r *profileRequest) normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Host = strings.TrimSpace(r.Host)
	r.Username = strings.TrimSpace(r.Username)
	r.Preset = strings.TrimSpace(r.Preset)
}

func (r profileRequest) profile(id string) domain.Profile {
	return domain.Profile{
		ID: id, Name: r.Name, Host: r.Host, Port: r.Port, Username: r.Username,
		Password: r.Password, BasePath: r.BasePath, Preset: r.Preset,
	}
}

type sourceLocatorRequest struct {
	RootID string `json:"root_id" validate:"required,max=128"`
	Path   string `json:"path" validate:"required,max=4096"`
}

func (r sourceLocatorRequest) locator() domain.SourceLocator {
	return domain.SourceLocator{RootID: r.RootID, Path: r.Path}
}

type destinationLocatorRequest struct {
	RootID string `json:"root_id" validate:"required,max=128"`
	Path   string `json:"path" validate:"max=4096"`
}

func (r destinationLocatorRequest) locator() domain.SourceLocator {
	return domain.SourceLocator{RootID: r.RootID, Path: r.Path}
}

type taskRequest struct {
	Type           string                 `json:"type" validate:"omitempty,oneof=upload download"`
	ProfileID      string                 `json:"profile_id" validate:"required,len=32,hexadecimal"`
	Sources        []sourceLocatorRequest `json:"sources" validate:"required,min=1,max=1000,dive"`
	Destination    *string                `json:"destination" validate:"required,max=4096"`
	ConflictPolicy string                 `json:"conflict_policy" validate:"required,oneof=smart overwrite fail"`
}

func (r *taskRequest) normalize() {
	r.Type = strings.TrimSpace(r.Type)
	r.ProfileID = strings.TrimSpace(r.ProfileID)
	r.ConflictPolicy = strings.TrimSpace(r.ConflictPolicy)
	for i := range r.Sources {
		r.Sources[i].RootID = strings.TrimSpace(r.Sources[i].RootID)
	}
}

func (r taskRequest) task() domain.Task {
	sources := make([]domain.SourceLocator, len(r.Sources))
	for i := range r.Sources {
		sources[i] = r.Sources[i].locator()
	}
	return domain.Task{
		Type: r.Type, ProfileID: r.ProfileID, Sources: sources,
		Destination: *r.Destination, ConflictPolicy: r.ConflictPolicy,
	}
}

type operationRequest struct {
	Action      string `json:"action" validate:"required,oneof=mkdir rename move delete"`
	Path        string `json:"path" validate:"required,max=4096"`
	Destination string `json:"destination,omitempty" validate:"omitempty,max=4096"`
	Recursive   bool   `json:"recursive,omitempty"`
	ConfirmName string `json:"confirm_name,omitempty" validate:"max=255"`
	IsDir       bool   `json:"is_dir,omitempty"`
}

func (r *operationRequest) normalize() {
	r.Action = strings.TrimSpace(r.Action)
}

type extractionRequest struct {
	Source            sourceLocatorRequest      `json:"source"`
	DestinationParent destinationLocatorRequest `json:"destination_parent"`
	Password          string                    `json:"password" validate:"max=1024"`
	DeleteSources     bool                      `json:"delete_sources"`
}

func (r *extractionRequest) normalize() {
	r.Source.RootID = strings.TrimSpace(r.Source.RootID)
	r.DestinationParent.RootID = strings.TrimSpace(r.DestinationParent.RootID)
}

type extractionRetryRequest struct {
	Password string `json:"password" validate:"max=1024"`
}

type settingsRequest struct {
	TransferWorkers int `json:"transfer_workers" validate:"required,gte=1,lte=4"`
}

type idParameter struct {
	ID string `json:"id" validate:"required,len=32,hexadecimal"`
}

type requestValidationError struct {
	fields map[string]string
	first  string
}

func (e *requestValidationError) Error() string { return e.first }

func newRequestValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("ftp_host", validFTPHost)
	v.RegisterStructValidation(validateOperationRequest, operationRequest{})
	return v
}

func validateOperationRequest(level validator.StructLevel) {
	request := level.Current().Interface().(operationRequest)
	if (request.Action == "rename" || request.Action == "move") && request.Destination == "" {
		level.ReportError(request.Destination, "destination", "Destination", "required", "")
	}
	if request.Action == "delete" && request.IsDir && request.Recursive && request.ConfirmName == "" {
		level.ReportError(request.ConfirmName, "confirm_name", "ConfirmName", "required", "")
	}
	if request.Action == "mkdir" && !validRemoteEntryPath(request.Path) {
		level.ReportError(request.Path, "path", "Path", "remote_name", "")
	}
	if (request.Action == "rename" || request.Action == "move") && request.Destination != "" && !validRemoteEntryPath(request.Destination) {
		level.ReportError(request.Destination, "destination", "Destination", "remote_name", "")
	}
}

func validRemoteEntryPath(value string) bool {
	value = strings.ReplaceAll(value, `\`, "/")
	if value == "" || strings.HasSuffix(value, "/") || strings.ContainsRune(value, 0) {
		return false
	}
	name := path.Base(value)
	return name != "." && name != ".." && len([]rune(name)) <= maxEntryNameLength
}

func validFTPHost(field validator.FieldLevel) bool {
	host := field.Field().String()
	if host == "" || strings.ContainsAny(host, `/\\[]`) {
		return false
	}
	for _, r := range host {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	if !strings.Contains(host, ":") {
		return !strings.Contains(host, "%")
	}
	address := host
	if before, _, ok := strings.Cut(address, "%"); ok {
		address = before
	}
	return net.ParseIP(address) != nil
}

func (s *Server) validateRequest(value any) error {
	err := s.validator.Struct(value)
	if err == nil {
		return nil
	}
	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return err
	}
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err
	}
	problem := &requestValidationError{fields: make(map[string]string)}
	for _, fieldError := range validationErrors {
		field := fieldError.Namespace()
		if dot := strings.IndexByte(field, '.'); dot >= 0 {
			field = field[dot+1:]
		}
		message := validationMessage(field, fieldError)
		problem.fields[field] = message
		if problem.first == "" {
			problem.first = message
		}
	}
	return problem
}

func (s *Server) withValidID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.validateRequest(idParameter{ID: r.PathValue("id")}); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		next(w, r)
	}
}

func validationMessage(field string, issue validator.FieldError) string {
	switch issue.Tag() {
	case "required", "required_if":
		return fmt.Sprintf("参数 %s 为必填项", field)
	case "oneof":
		return fmt.Sprintf("参数 %s 的值不受支持", field)
	case "hexadecimal", "ftp_host":
		return fmt.Sprintf("参数 %s 格式不正确", field)
	case "remote_name":
		return fmt.Sprintf("参数 %s 的文件名无效", field)
	case "len":
		return fmt.Sprintf("参数 %s 长度必须为 %s", field, issue.Param())
	case "min", "gte":
		return fmt.Sprintf("参数 %s 不能小于 %s", field, issue.Param())
	case "max", "lte":
		return fmt.Sprintf("参数 %s 不能大于 %s", field, issue.Param())
	default:
		return fmt.Sprintf("参数 %s 校验失败", field)
	}
}
