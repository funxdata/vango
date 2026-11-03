//go:build !windows

// Package debug 通过 SIGUSR1 信号输出运行时诊断信息。
//
//	go run . &            # 启动业务进程
//	kill -USR1 <pid>      # 触发诊断
//	cat $TMPDIR/vango-stack-trace.log
package debug

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/funxdata/vango/version"
)

const debugInfo = `
version      - current binary version
goroutine    - stack traces of all current goroutines
heap         - a sampling of all heap allocations
threadcreate - stack traces that led to the creation of new OS threads
block        - stack traces that led to blocking on synchronization primitives
`

// TraceFile 返回诊断信息的输出路径。
func TraceFile() string {
	return filepath.Join(os.TempDir(), "vango-stack-trace.log")
}

func init() {
	start := time.Now()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)

	go func() {
		for range signals {
			writeTrace(start)
		}
	}()
}

func writeTrace(start time.Time) {
	file, err := os.OpenFile(TraceFile(), os.O_RDWR|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return
	}
	defer file.Close()

	fmt.Fprint(file, debugInfo)
	fmt.Fprint(file, "\n\nVERSION\n\n")
	fmt.Fprintf(file, "version ----------- %s\n", version.Complete())
	fmt.Fprintf(file, "start at: --------- %s\n", start)

	fmt.Fprint(file, "\n\nGOROUTINE\n\n")
	_ = pprof.Lookup("goroutine").WriteTo(file, 2)
	fmt.Fprint(file, "\n\nHEAP\n\n")
	_ = pprof.Lookup("heap").WriteTo(file, 1)
	fmt.Fprint(file, "\n\nTHREADCREATE\n\n")
	_ = pprof.Lookup("threadcreate").WriteTo(file, 1)
	fmt.Fprint(file, "\n\nBLOCK\n\n")
	_ = pprof.Lookup("block").WriteTo(file, 1)
}
