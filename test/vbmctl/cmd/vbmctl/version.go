//go:build vbmctl
// +build vbmctl

// Provides the implementation of the "vbmctl version" command.
package main

import (
	"fmt"
	"os"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Long:  "Print the version of vbmctl.",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintf(os.Stdout, "vbmctl version %s\n", config.Version)
		},
	}
}
