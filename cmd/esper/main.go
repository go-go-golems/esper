package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/go-go-golems/esper/pkg/commands/devicescmd"
	"github.com/go-go-golems/esper/pkg/commands/matrixhttpcmd"
	"github.com/go-go-golems/esper/pkg/commands/scancmd"
	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/esper/pkg/monitor"
	"github.com/go-go-golems/esper/pkg/tail"
	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var monitorCfg monitor.Config
	var tailCfg tail.Config

	rootCmd := &cobra.Command{
		Use:           "esper",
		Short:         "ESP32 serial monitor + utilities",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := monitorCfg
			if cfg.Port != "" {
				resolved, err := resolvePort(cmd.Context(), cfg.Port)
				if err != nil {
					return err
				}
				cfg.Port = resolved
			}
			return monitor.Run(cmd.Context(), cfg)
		},
	}

	rootCmd.Flags().StringVar(&monitorCfg.Port, "port", "", "Serial port (or glob like /dev/serial/by-id/*)")
	rootCmd.Flags().IntVar(&monitorCfg.Baud, "baud", 115200, "Baud rate")
	rootCmd.Flags().StringVar(&monitorCfg.ElfPath, "elf", "", "Path to app ELF for decoding (optional)")
	rootCmd.Flags().StringVar(&monitorCfg.ToolchainPrefix, "toolchain-prefix", "", "Toolchain prefix (e.g. xtensa-esp32s3-elf-) for addr2line (optional)")

	// Non-TUI tail mode.
	tailCfg.CoreDumpAutoEnter = true
	tailCfg.CoreDumpMute = true
	tailCmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream serial output to stdout (non-TUI), with esper decoding/color pipeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := tailCfg
			resolved, err := resolvePort(cmd.Context(), cfg.Port)
			if err != nil {
				return err
			}
			cfg.Port = resolved
			return tail.Run(cmd.Context(), cfg, os.Stdout, os.Stderr)
		},
	}
	tailCmd.Flags().StringVar(&tailCfg.Port, "port", "", "Serial port (or glob like /dev/serial/by-id/*; or a device nickname)")
	tailCmd.Flags().IntVar(&tailCfg.Baud, "baud", 115200, "Baud rate")
	tailCmd.Flags().DurationVar(&tailCfg.Timeout, "timeout", 0*time.Second, "Exit after this duration (0 = run until Ctrl-C)")
	tailCmd.Flags().BoolVar(&tailCfg.StdinRaw, "stdin-raw", false, "Forward stdin to device as raw bytes (bidirectional). Ctrl-] exits.")
	tailCmd.Flags().StringVar(&tailCfg.ElfPath, "elf", "", "Path to app ELF for decoding (optional)")
	tailCmd.Flags().StringVar(&tailCfg.ToolchainPrefix, "toolchain-prefix", "", "Toolchain prefix (e.g. xtensa-esp32s3-elf-) for addr2line (optional)")

	tailCmd.Flags().BoolVar(&tailCfg.NoAutoColor, "no-autocolor", false, "Disable monitor-inserted ESP-IDF auto-coloring")
	tailCmd.Flags().BoolVar(&tailCfg.NoBacktrace, "no-backtrace", false, "Disable panic/backtrace decoded output")
	tailCmd.Flags().BoolVar(&tailCfg.NoGDB, "no-gdb", false, "Disable GDB stub detection notices")
	tailCmd.Flags().BoolVar(&tailCfg.NoCoreDump, "no-coredump", false, "Disable core dump buffering/decoding logic")
	tailCmd.Flags().BoolVar(&tailCfg.CoreDumpAutoEnter, "coredump-auto-enter", tailCfg.CoreDumpAutoEnter, "Auto-send Enter when core dump prompt is detected")
	tailCmd.Flags().BoolVar(&tailCfg.CoreDumpMute, "coredump-mute", tailCfg.CoreDumpMute, "Mute normal output while core dump capture is in progress")
	tailCmd.Flags().BoolVar(&tailCfg.NoCoreDumpDecode, "no-coredump-decode", false, "Never attempt to decode core dumps (even if --elf is provided)")

	tailCmd.Flags().BoolVar(&tailCfg.Timestamps, "timestamps", false, "Prefix each output line with HH:MM:SS")
	tailCmd.Flags().BoolVar(&tailCfg.PrefixPort, "prefix-port", false, "Prefix each output line with the resolved port path")
	tailCmd.Flags().StringVar(&tailCfg.LogFile, "log-file", "", "Tee output to this file (in addition to stdout)")
	tailCmd.Flags().BoolVar(&tailCfg.LogAppend, "log-append", false, "Append to --log-file instead of truncating")
	tailCmd.Flags().BoolVar(&tailCfg.NoStdout, "no-stdout", false, "Disable stdout output (requires --log-file)")

	rootCmd.AddCommand(tailCmd)

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

	matrixHTTPCmd, err := matrixhttpcmd.NewMatrixHTTPCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper: create matrix-http command: %v\n", err)
		os.Exit(1)
	}
	cobraMatrixHTTPCmd, err := cli.BuildCobraCommand(matrixHTTPCmd,
		cli.WithParserConfig(cli.CobraParserConfig{
			ShortHelpLayers: []string{schema.DefaultSlug},
			MiddlewaresFunc: cli.CobraCommandDefaultMiddlewares,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper: build matrix-http cobra command: %v\n", err)
		os.Exit(1)
	}
	rootCmd.AddCommand(cobraMatrixHTTPCmd)

	devicesCmd, err := devicescmd.NewDevicesCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper: build devices command: %v\n", err)
		os.Exit(1)
	}
	rootCmd.AddCommand(devicesCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "esper: %v\n", err)
		os.Exit(1)
	}
}

func resolvePort(ctx context.Context, port string) (string, error) {
	if port == "" {
		return "", nil
	}

	// Allow ttyACM0 style on Linux.
	if runtime.GOOS == "linux" && !strings.Contains(port, "/") && strings.HasPrefix(port, "tty") {
		return "/dev/" + port, nil
	}

	// If it doesn't look like a path or a glob, treat it as a nickname.
	if !strings.Contains(port, "/") && !strings.ContainsAny(port, "*?[") && !strings.HasPrefix(port, "tty") {
		res, err := devices.ResolveNicknameToPort(ctx, port)
		if err != nil {
			return "", err
		}
		return res.PortPath, nil
	}

	return port, nil
}
