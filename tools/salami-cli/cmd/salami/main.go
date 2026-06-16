package main

import (
	"fmt"
	"os"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/commands"
)

func main() {
	cmd, err := commands.NewRootCmd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err = cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
