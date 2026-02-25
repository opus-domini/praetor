package main

import (
	"os"

	"github.com/opus-domini/praetor/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
