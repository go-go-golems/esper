# esper

`esper` is an ESP-IDF-compatible serial monitor, implemented in Go.

Goals (Phase 1):
- Terminal-first, minimal UI (Bubble Tea).
- `idf.py monitor`-compatible behaviors for:
  - log colors (I/W/E auto-color + correct reset across partial lines),
  - detection/handling of panic backtraces, core dumps, and GDB stub events.

This repo also includes a small ESP32-S3 test firmware to exercise the monitor.

