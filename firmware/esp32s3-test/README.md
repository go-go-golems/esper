# esper ESP32-S3 Test Firmware

Minimal ESP-IDF firmware to exercise `esper` monitor behaviors:

- ESP-IDF log lines at I/W/E levels (auto-color).
- Partial line output (no newline for a while).
- Panic/backtrace generation.
- GDB stub “stop reason” sequence emission (for detection).
- Core dump marker emission (for buffering/decoder paths).

Console:
- Uses `esp_console` REPL over **USB Serial/JTAG**.
- Prompt: `esper> `

## Build

```bash
source "$HOME/esp/esp-idf-5.4.1/export.sh"
cd firmware/esp32s3-test
idf.py set-target esp32s3
idf.py build
```

## Flash + Monitor (ESP-IDF monitor)

```bash
idf.py flash monitor
```

## Flash + esper (Go monitor)

In another terminal:

```bash
cd ../../
go run ./cmd/esper -port '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_*' -baud 115200 -elf firmware/esp32s3-test/build/esp32s3-test.elf -toolchain-prefix xtensa-esp32s3-elf-
```

Then in the `esper` UI, type commands such as:
- `help`
- `logdemo`
- `partial`
- `gdbstub`
- `coredumpfake`
- `panic` (will crash/reboot)

