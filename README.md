# esper

`esper` is an ESP-IDF-compatible serial monitor, implemented in Go.

Goals (Phase 1):
- Terminal-first, minimal UI (Bubble Tea).
- `idf.py monitor`-compatible behaviors for:
  - log colors (I/W/E auto-color + correct reset across partial lines),
  - detection/handling of panic backtraces, core dumps, and GDB stub events.

This repo also includes a small ESP32-S3 test firmware to exercise the monitor.

## Scan ports (Linux)

```bash
go run ./cmd/esper scan
go run ./cmd/esper scan --json
go run ./cmd/esper scan --all
```

## Run monitor

```bash
go run ./cmd/esper -port '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_*'
```

## Tail serial output (non-TUI)

`esper tail` streams the serial output to stdout after running it through esper's pipeline (ESP-IDF auto-color, core dump handling, panic backtrace decoding, GDB stub detection).

```bash
go run ./cmd/esper tail \
  --port '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_*' \
  --baud 115200 \
  --timeout 10s
```

Optional decoding (requires local toolchain):

```bash
go run ./cmd/esper tail \
  --port '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_*' \
  --elf firmware/esp32s3-test/build/esp32s3-test.elf \
  --toolchain-prefix xtensa-esp32s3-elf- \
  --timeout 10s
```

Bidirectional (no TUI), raw stdin forwarding:

```bash
# Ctrl-] exits.
go run ./cmd/esper tail \
  --port '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_*' \
  --stdin-raw
```
