// Package cli 实现 vango 命令行工具。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/funxdata/vango/version"
)

// Usage 是命令行的帮助信息。
const Usage = `vango - Go 应用框架命令行工具

用法:
  vango <command> [arguments]

命令:
  new <name>    创建一个新的 vango 项目
  version       打印版本信息
  help          显示本帮助

new 的参数:
  -module string  项目 module 路径（默认与项目名相同）
  -vango string   依赖的 vango 版本（默认 v0.1.0）
  -dir string     项目创建目录（默认为项目名）
`

// Run 执行命令行，返回进程退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, Usage)
		return 0
	}

	switch args[0] {
	case "new":
		if err := runNew(args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "vango new: %v\n", err)
			return 1
		}
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "vango %s\n", version.Complete())
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, Usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], Usage)
		return 2
	}
}

func runNew(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(stderr)

	modulePath := flags.String("module", "", "项目的 module 路径")
	vangoVersion := flags.String("vango", "v0.1.0", "依赖的 vango 版本")
	dir := flags.String("dir", "", "项目创建目录")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return errors.New("missing project name")
	}

	name := flags.Arg(0)
	if *modulePath == "" {
		*modulePath = name
	}
	if *dir == "" {
		*dir = name
	}

	if err := Scaffold(*dir, Options{
		Name:         name,
		ModulePath:   *modulePath,
		VangoVersion: *vangoVersion,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "created %s\n\n", *dir)
	fmt.Fprintf(stdout, "下一步:\n  cd %s\n  go mod tidy\n  go run .\n", *dir)
	return nil
}

// Main 用 os.Args 与标准输出执行命令行并退出。
func Main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
