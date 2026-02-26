package main

import (
	"os"
	"strings"

	"github.com/opus-domini/praetor/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		if strings.Contains(err.Error(), "unknown command") {
			// Cobra already printed the error; show help so the user
			// can see the available commands without a second invocation.
			root.PrintErrln()
			_ = root.Help()
		}
		os.Exit(1)
	}
}
