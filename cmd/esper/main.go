package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-go-golems/esper/pkg/monitor"
)

func main() {
	var (
		port          = flag.String("port", "", "Serial port (or glob like /dev/serial/by-id/*)")
		baud          = flag.Int("baud", 115200, "Baud rate")
		elf           = flag.String("elf", "", "Path to app ELF for decoding (optional)")
		toolchainPref = flag.String("toolchain-prefix", "", "Toolchain prefix (e.g. xtensa-esp32s3-elf-) for addr2line (optional)")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := monitor.Run(ctx, monitor.Config{
		Port:            *port,
		Baud:            *baud,
		ElfPath:         *elf,
		ToolchainPrefix: *toolchainPref,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "esper: %v\n", err)
		os.Exit(1)
	}
}

