package web

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/store"
)

const (
	codeSuccess            = 0
	codeInvalidRequest     = 1000
	codeProfileNotFound    = 1001
	codeResourceNotFound   = 1002
	codeResourceConflict   = 1003
	codePS5OperationFailed = 2001
	codeInternalError      = 9000
)

const (
	messageSuccess            = "success"
	messageInvalidRequest     = "请求参数无效"
	messageProfileNotFound    = "PS5 配置不存在或已被删除，请重新配置"
	messageResourceNotFound   = "请求的资源不存在"
	messageResourceConflict   = "当前状态不允许此操作"
	messagePS5OperationFailed = "PS5 操作失败，请检查连接和路径"
	messageInternalError      = "服务内部错误，请稍后重试"
)

type responseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type classifiedError struct {
	code int
	err  error
}

func (e *classifiedError) Error() string { return e.err.Error() }
func (e *classifiedError) Unwrap() error { return e.err }

func invalidRequestError(err error) error {
	return &classifiedError{code: codeInvalidRequest, err: err}
}

func resourceConflictError(err error) error {
	return &classifiedError{code: codeResourceConflict, err: err}
}

func ps5OperationError(err error) error {
	return &classifiedError{code: codePS5OperationFailed, err: err}
}

func successResponse(w http.ResponseWriter, data any) {
	jsonResponse(w, http.StatusOK, responseEnvelope{Code: codeSuccess, Message: messageSuccess, Data: data})
}

func businessResponse(w http.ResponseWriter, code int, message string, data any) {
	jsonResponse(w, http.StatusBadRequest, responseEnvelope{Code: code, Message: message, Data: data})
}

func internalResponse(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	jsonResponse(w, http.StatusInternalServerError, responseEnvelope{Code: codeInternalError, Message: messageInternalError, Data: nil})
}

func fail(w http.ResponseWriter, status int, err error) {
	var invalid *requestValidationError
	var classified *classifiedError
	switch {
	case errors.As(err, &invalid):
		businessResponse(w, codeInvalidRequest, messageInvalidRequest, map[string]any{"fields": invalid.fields})
	case errors.Is(err, store.ErrProfileNotFound):
		businessResponse(w, codeProfileNotFound, messageProfileNotFound, nil)
	case errors.Is(err, sql.ErrNoRows):
		businessResponse(w, codeResourceNotFound, messageResourceNotFound, nil)
	case errors.Is(err, store.ErrStateConflict):
		businessResponse(w, codeResourceConflict, messageResourceConflict, nil)
	case errors.As(err, &classified) && classified.code == codeInvalidRequest:
		businessResponse(w, codeInvalidRequest, messageInvalidRequest, nil)
	case errors.As(err, &classified) && classified.code == codeResourceConflict:
		businessResponse(w, codeResourceConflict, messageResourceConflict, nil)
	case errors.As(err, &classified) && classified.code == codePS5OperationFailed:
		businessResponse(w, codePS5OperationFailed, messagePS5OperationFailed, nil)
	case status == http.StatusBadRequest:
		businessResponse(w, codeInvalidRequest, messageInvalidRequest, nil)
	default:
		internalResponse(w, err)
	}
}
