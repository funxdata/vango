package web

import (
	"net/http"

	"github.com/funxdata/vango/errs"
)

// Response 是 vango 的统一响应体。
//
// 成功时 Code 为 0；失败时 Code 与 HTTP 状态码保持一致，
// 客户端可以据此做统一处理，也可以直接依赖 HTTP 状态码。
//
// 失败响应还会带上稳定的业务错误码 ErrorCode（例如 "USER_NOT_FOUND"），
// 它来自 errs.Error.Code，适合客户端做精确判断。
type Response struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// CodeOK 表示成功的业务码。
const CodeOK = 0

// Page 是分页响应的数据部分。
type Page struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// OK 返回 200 成功响应。
func OK(c *Context, data any) error {
	return c.JSON(http.StatusOK, Response{
		Code:      CodeOK,
		Message:   "ok",
		Data:      data,
		RequestID: c.RequestID(),
	})
}

// Created 返回 201 响应。
func Created(c *Context, data any) error {
	return c.JSON(http.StatusCreated, Response{
		Code:      CodeOK,
		Message:   "created",
		Data:      data,
		RequestID: c.RequestID(),
	})
}

// OKPage 返回分页数据。
func OKPage(c *Context, items any, total int64, page, pageSize int) error {
	return OK(c, Page{Items: items, Total: total, Page: page, PageSize: pageSize})
}

// Fail 把任意错误写成统一错误响应。
func Fail(c *Context, err error) error {
	e := errs.From(err)
	if e == nil {
		return OK(c, nil)
	}
	return c.JSON(e.Status, Response{
		Code:      e.Status,
		Message:   e.Message,
		ErrorCode: e.Code,
		Data:      e.Details,
		RequestID: c.RequestID(),
	})
}

func badRequest(message string) error {
	return errs.BadRequest(message)
}
