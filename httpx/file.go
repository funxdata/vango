package httpx

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// PostFile 以 multipart 形式上传本地文件，formName 对应表单字段名。
//
// 大文件建议自行构造请求并流式写入，避免一次性读入内存。
func PostFile(uri, formName, filePath string) (*http.Response, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("httpx: open %s: %w", filePath, err)
	}
	defer file.Close()

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	part, err := writer.CreateFormFile(formName, filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("httpx: create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("httpx: copy file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("httpx: close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, uri, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return defaultClient.Do(req)
}
