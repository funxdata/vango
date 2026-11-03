// Package errs 定义 vango 统一的结构化错误。
//
// 框架内所有可直接返回给客户端的错误都应使用 *errs.Error。它是 error 的实现，
// 同时携带 HTTP 状态码、稳定的业务错误码与可选的附加细节，web 层会据此生成响应体。
package errs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Error 是可以安全地返回给客户端的结构化错误。
type Error struct {
	// Status 是 HTTP 状态码，例如 404。
	Status int `json:"status"`
	// Code 是稳定的业务错误码，例如 "USER_NOT_FOUND"。客户端应基于它做判断。
	Code string `json:"code"`
	// Message 是面向调用方的错误描述，不应包含内部实现细节。
	Message string `json:"message"`
	// Details 是可选的结构化附加信息，例如字段级校验错误。
	Details any `json:"details,omitempty"`

	cause error
}

// New 构造一个结构化错误。
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.cause != nil {
		return fmt.Sprintf("%s (%s): %v", e.Message, e.Code, e.cause)
	}
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// Unwrap 让 errors.Is / errors.As 可以穿透到底层错误。
func (e *Error) Unwrap() error { return e.cause }

// WithCause 附加底层错误并返回自身，便于链式书写。
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// WithDetails 附加结构化细节并返回自身。
func (e *Error) WithDetails(details any) *Error {
	e.Details = details
	return e
}

// WithMessage 覆盖错误描述并返回自身。
func (e *Error) WithMessage(message string) *Error {
	e.Message = message
	return e
}

// 常用错误的快捷构造器。
func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(message string) *Error {
	return New(http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(message string) *Error {
	return New(http.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(message string) *Error {
	return New(http.StatusNotFound, "NOT_FOUND", message)
}

func Conflict(message string) *Error {
	return New(http.StatusConflict, "CONFLICT", message)
}

func UnprocessableEntity(message string) *Error {
	return New(http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message)
}

func TooManyRequests(message string) *Error {
	return New(http.StatusTooManyRequests, "TOO_MANY_REQUESTS", message)
}

func Internal(message string) *Error {
	return New(http.StatusInternalServerError, "INTERNAL", message)
}

func Unavailable(message string) *Error {
	return New(http.StatusServiceUnavailable, "UNAVAILABLE", message)
}

// From 把任意 error 归一化为 *Error。
//
// 已经包装过的 *Error 原样返回；其余错误统一映射为 500，避免把内部细节泄露给客户端，
// 原始错误通过 Unwrap 保留，调用方仍可在日志中看到完整信息。
func From(err error) *Error {
	if err == nil {
		return nil
	}

	var e *Error
	if errors.As(err, &e) {
		return e
	}

	switch {
	case errors.Is(err, context.Canceled):
		return New(499, "CLIENT_CLOSED_REQUEST", "client closed request").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return New(http.StatusGatewayTimeout, "TIMEOUT", "request timed out").WithCause(err)
	default:
		return Internal("internal server error").WithCause(err)
	}
}
