// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"

	"github.com/neko-sc/ent/cmd/internal/base"
	"github.com/neko-sc/ent/entc/gen"

	"github.com/spf13/cobra"
)

func main() {
	log.SetFlags(0)
	cmd := &cobra.Command{Use: "entc"}
	cmd.AddCommand(
		base.NewCmd(),
		base.DescribeCmd(),
		base.GenerateCmd(migrate),
		base.InitCmd(),
	)
	_ = cmd.Execute()
}

func migrate(c *gen.Config) {
	var (
		target = filepath.Join(c.Target, "generate.go")
		oldCmd = []byte("github.com/neko-sc/ent/cmd/entc")
	)
	buf, err := os.ReadFile(target)
	if err != nil || !bytes.Contains(buf, oldCmd) {
		return
	}
	_ = os.WriteFile(target, bytes.ReplaceAll(buf, oldCmd, []byte("github.com/neko-sc/ent/cmd/ent")), 0644)
}
