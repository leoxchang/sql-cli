package main

import (
	"os"

	"github.com/qiezi999/sql-cli/internal/cli"
)

func main() {
	exitCode := cli.Run(os.Args)
	os.Exit(exitCode)
}