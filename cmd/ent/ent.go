// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/neko-sc/ent/cmd/internal/base"

	"github.com/spf13/cobra"
)

func main() {
	log.SetFlags(0)
	cmd := &cobra.Command{Use: "ent"}
	cmd.AddCommand(
		base.NewCmd(),
		base.DescribeCmd(),
		base.GenerateCmd(),
		base.InitCmd(),
		base.SchemaCmd(),
	)
	cobra.CheckErr(cmd.Execute())
}
