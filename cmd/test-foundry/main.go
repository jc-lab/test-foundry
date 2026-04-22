// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"fmt"
	"os"

	"github.com/jc-lab/test-foundry/internal/cli"
)

func main() {
	rootCmd := cli.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
