package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-go-golems/esper/pkg/monitor"
	"github.com/go-go-golems/esper/pkg/scan"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		runScan()
		return
	}

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

func runScan() {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	var (
		all        = fs.Bool("all", false, "Show all serial ports, not only ESP32-ish devices")
		jsonOut    = fs.Bool("json", false, "Print JSON")
		serialByID = fs.Bool("prefer-by-id", true, "Prefer stable /dev/serial/by-id paths in output")
	)
	_ = fs.Parse(os.Args[2:])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ports, err := scan.ScanLinux(ctx, scan.Options{
		All:        *all,
		PreferByID: *serialByID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "esper scan: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(ports); err != nil {
			fmt.Fprintf(os.Stderr, "esper scan: json: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(ports) == 0 {
		fmt.Println("no serial ports found")
		return
	}

	fmt.Printf("%-45s  %-12s  %-9s  %s\n", "PORT", "VID:PID", "SCORE", "DETAILS")
	for _, p := range ports {
		path := p.Device
		if p.PreferredPath != "" {
			path = p.PreferredPath
		}
		details := p.Product
		if details == "" {
			details = p.Manufacturer
		}
		if details == "" {
			details = "-"
		}
		if len(p.Reasons) > 0 {
			details = details + " (" + p.Reasons[0] + ")"
		}
		fmt.Printf("%-45s  %-12s  %-9d  %s\n", path, p.VIDPID(), p.Score, details)
	}
}
