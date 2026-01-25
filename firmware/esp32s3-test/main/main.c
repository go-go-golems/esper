#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sdkconfig.h"

#include "esp_console.h"
#include "esp_err.h"
#include "esp_log.h"
#include "esp_system.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "esper_test";

static void delay_ms(uint32_t ms) {
    vTaskDelay(pdMS_TO_TICKS(ms));
}

static int cmd_logdemo(int argc, char **argv) {
    (void)argc;
    (void)argv;
    ESP_LOGI(TAG, "logdemo: info line");
    ESP_LOGW(TAG, "logdemo: warning line");
    ESP_LOGE(TAG, "logdemo: error line");
    printf("plain printf line (no ESP_LOG)\n");
    return 0;
}

static int cmd_partial(int argc, char **argv) {
    (void)argc;
    (void)argv;
    // Print without newline, then finish later.
    printf("partial: this line starts without newline...");
    fflush(stdout);
    for (volatile int i = 0; i < 2000000; i++) {
        // busy wait ~small delay; avoids needing FreeRTOS includes for this minimal demo
    }
    printf("done\n");
    return 0;
}

static uint8_t hex_nibble(uint8_t v) {
    return (uint8_t)(v < 10 ? ('0' + v) : ('a' + (v - 10)));
}

static int cmd_gdbstub(int argc, char **argv) {
    (void)argc;
    (void)argv;
    // Emit a minimal valid gdb stop-reason packet: $T05#<checksum>
    // Checksum is the 8-bit sum of bytes between '$' and '#'.
    const char *payload = "T05";
    uint8_t sum = 0;
    for (const char *p = payload; *p; p++) sum = (uint8_t)(sum + (uint8_t)*p);
    char pkt[16];
    int n = snprintf(pkt, sizeof(pkt), "$%s#%c%c", payload, (char)hex_nibble(sum >> 4), (char)hex_nibble(sum & 0xF));
    if (n > 0) {
        // Note: this is not a real gdb stub session; it's just to exercise detection logic.
        printf("%s\n", pkt);
    }
    return 0;
}

static int cmd_coredumpfake(int argc, char **argv) {
    (void)argc;
    (void)argv;
    // Exercise core dump recognizers without requiring real core dump configuration.
    printf("Press Enter to print core dump to UART...\n");
    printf("================= CORE DUMP START =================\n");
    // base64-ish payload (not a real core dump)
    printf("aGVsbG8gd29ybGQK\n");
    printf("c29tZSBtb3JlIGRhdGEK\n");
    printf("================= CORE DUMP END =================\n");
    return 0;
}

static int cmd_coredumpfakeslow(int argc, char **argv) {
    // Exercise core dump recognizers with a slow/long payload so a "capture in progress"
    // overlay has time to be observed.
    int lines = 200;
    int delay_every = 10;
    int delay_ms_each = 50;
    if (argc >= 2) {
        int v = atoi(argv[1]);
        if (v > 0) lines = v;
    }

    printf("Press Enter to print core dump to UART...\n");
    printf("================= CORE DUMP START =================\n");

    for (int i = 0; i < lines; i++) {
        // base64-ish payload (not a real core dump). Keep it single-line.
        printf("Y29yZWR1bXAtc2xvdy1saW5lLTAwMDAwMDAwMDAwMDAwMC0lMDNkCg== %03d\n", i);
        if (delay_every > 0 && (i % delay_every) == 0) {
            delay_ms((uint32_t)delay_ms_each);
        }
    }

    printf("================= CORE DUMP END =================\n");
    return 0;
}

static int cmd_emitall(int argc, char **argv) {
    // One-shot deterministic event suite. If the optional arg "panic" is provided,
    // trigger panic last (device will reboot).
    bool do_panic = false;
    if (argc >= 2 && strcmp(argv[1], "panic") == 0) {
        do_panic = true;
    }

    cmd_logdemo(0, NULL);
    cmd_partial(0, NULL);
    cmd_gdbstub(0, NULL);
    // Keep capture-in-progress overlay visible for a moment.
    char *slow_argv[] = {(char *)"coredumpfakeslow", (char *)"250"};
    cmd_coredumpfakeslow(2, slow_argv);

    if (do_panic) {
        cmd_panic(0, NULL);
    }
    return 0;
}

static int cmd_panic(int argc, char **argv) {
    (void)argc;
    (void)argv;
    ESP_LOGE(TAG, "panic: triggering abort()");
    abort();
    return 0;
}

static void register_cmd(const char *name, const char *help, esp_console_cmd_func_t fn) {
    esp_console_cmd_t cmd = {0};
    cmd.command = name;
    cmd.help = help;
    cmd.func = fn;
    ESP_ERROR_CHECK(esp_console_cmd_register(&cmd));
}

static void console_start(void) {
#if !CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG
    ESP_LOGW(TAG, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG disabled; no REPL started");
    return;
#else
    esp_console_repl_t *repl = NULL;
    esp_console_repl_config_t repl_cfg = ESP_CONSOLE_REPL_CONFIG_DEFAULT();
    repl_cfg.prompt = "esper> ";
    repl_cfg.task_stack_size = 4096;

    esp_console_dev_usb_serial_jtag_config_t hw_cfg = ESP_CONSOLE_DEV_USB_SERIAL_JTAG_CONFIG_DEFAULT();
    esp_err_t err = esp_console_new_repl_usb_serial_jtag(&hw_cfg, &repl_cfg, &repl);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_console_new_repl_usb_serial_jtag failed: %s", esp_err_to_name(err));
        return;
    }

    esp_console_register_help_command();
    register_cmd("logdemo", "Print sample I/W/E logs", &cmd_logdemo);
    register_cmd("partial", "Print a partial line (no newline for a bit)", &cmd_partial);
    register_cmd("gdbstub", "Emit a valid $T..#.. gdb stop-reason packet (for detection)", &cmd_gdbstub);
    register_cmd("coredumpfake", "Emit core dump markers + dummy base64 payload", &cmd_coredumpfake);
    register_cmd("coredumpfakeslow", "Emit slow core dump markers + long dummy payload (for capture overlay)", &cmd_coredumpfakeslow);
    register_cmd("emitall", "Emit logdemo+partial+gdbstub+coredumpfakeslow (optionally: emitall panic)", &cmd_emitall);
    register_cmd("panic", "Trigger abort() to generate a panic/backtrace", &cmd_panic);

    err = esp_console_start_repl(repl);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_console_start_repl failed: %s", esp_err_to_name(err));
        return;
    }

    ESP_LOGI(TAG, "esp_console started over USB Serial/JTAG (try: help, logdemo, partial, coredumpfake, gdbstub, panic)");
#endif
}

void app_main(void) {
    ESP_LOGI(TAG, "boot: esper test firmware");
    console_start();
}
