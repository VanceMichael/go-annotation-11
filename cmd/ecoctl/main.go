// Command ecoctl 是生态环境损害赔偿案件管理平台的命令行入口。
package main

import (
	"os"

	"ecoclaim/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
