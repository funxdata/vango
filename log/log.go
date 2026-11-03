// Package log 基于标准库 log/slog 提供结构化日志的默认配置。
package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 描述日志器的构建参数。
type Options struct {
	// Level 支持 debug/info/warn/error，空值等价于 info。
	Level string
	// Format 支持 json/text，默认 json。
	Format string
	// Output 默认 os.Stdout。
	Output io.Writer
	// AddSource 为 true 时记录调用位置，有明显性能开销，仅建议调试时开启。
	AddSource bool
}

// New 按 Options 构建一个 *slog.Logger。
func New(opts Options) *slog.Logger {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     ParseLevel(opts.Level),
		AddSource: opts.AddSource,
	}

	var handler slog.Handler
	if strings.EqualFold(opts.Format, "text") {
		handler = slog.NewTextHandler(out, handlerOpts)
	} else {
		handler = slog.NewJSONHandler(out, handlerOpts)
	}
	return slog.New(handler)
}

// Discard 返回一个丢弃所有日志的 logger，便于测试。
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ParseLevel 把字符串级别转换为 slog.Level，无法识别时返回 slog.LevelInfo。
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "d":
		return slog.LevelDebug
	case "warn", "warning", "w":
		return slog.LevelWarn
	case "error", "e":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// With 为 logger 追加固定的键值对，等价于 logger.With(args...)。
func With(logger *slog.Logger, args ...any) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger.With(args...)
}
