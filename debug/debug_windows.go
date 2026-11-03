//go:build windows

// Package debug 在 Windows 上不提供 SIGUSR1 诊断能力，仅保留接口以保持跨平台一致。
package debug

import (
	"os"
	"path/filepath"
)

// TraceFile 返回诊断信息的输出路径。
//
// Windows 不会生成诊断信息，但路径仍然可用，方便调用方统一处理。
func TraceFile() string {
	return filepath.Join(os.TempDir(), "vango-stack-trace.log")
}
