package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-go-golems/esper/pkg/commands/scancmd"
	"github.com/go-go-golems/esper/pkg/monitor"
	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var monitorCfg monitor.Config

	rootCmd := &cobra.Command{
		Use:           "esper",
		Short:         "ESP32 serial monitor + utilities",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return monitor.Run(cmd.Context(), monitorCfg)
		},
	}

	rootCmd.Flags().StringVar(&monitorCfg.Port, "port", "", "Serial port (or glob like /dev/serial/by-id/*)")
	rootCmd.Flags().IntVar(&monitorCfg.Baud, "baud", 115200, "Baud rate")
	rootCmd.Flags().StringVar(&monitorCfg.ElfPath, "elf", "", "Path to app ELF for decoding (optional)")
	rootCmd.Flags().StringVar(&monitorCfg.ToolchainPrefix, "toolchain-prefix", "", "Toolchain prefix (e.g. xtensa-esp32s3-elf-) for addr2line (optional)")

	scanCmd, err := scancmd.NewScanCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper: create scan command: %v\n", err)
		os.Exit(1)
	}
	cobraScanCmd, err := cli.BuildCobraCommand(scanCmd,
		cli.WithParserConfig(cli.CobraParserConfig{
			ShortHelpLayers: []string{schema.DefaultSlug},
			MiddlewaresFunc: cli.CobraCommandDefaultMiddlewares,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper: build scan cobra command: %v\n", err)
		os.Exit(1)
	}
	rootCmd.AddCommand(cobraScanCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "esper: %v\n", err)
		os.Exit(1)
	}
}
