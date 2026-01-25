package scancmd

import (
	"context"
	"strings"

	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/esper/pkg/scan"
	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/fields"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/go-go-golems/glazed/pkg/cmds/values"
	"github.com/go-go-golems/glazed/pkg/middlewares"
	"github.com/go-go-golems/glazed/pkg/types"
)

type ScanCommand struct {
	*cmds.CommandDescription
}

var _ cmds.GlazeCommand = (*ScanCommand)(nil)

type ScanSettings struct {
	All                 bool   `glazed.parameter:"all"`
	PreferByID          bool   `glazed.parameter:"prefer-by-id"`
	ProbeEsptool        bool   `glazed.parameter:"probe-esptool"`
	EsptoolConnectMode  string `glazed.parameter:"esptool-connect-mode"`
	EsptoolConnectTries int    `glazed.parameter:"esptool-connect-attempts"`
	EsptoolAfter        string `glazed.parameter:"esptool-after"`
}

func NewScanCommand() (*ScanCommand, error) {
	glazedLayer, err := schema.NewGlazedSchema()
	if err != nil {
		return nil, err
	}

	commandSettingsLayer, err := cli.NewCommandSettingsLayer()
	if err != nil {
		return nil, err
	}

	cmdDesc := cmds.NewCommandDescription(
		"scan",
		cmds.WithShort("Scan for ESP32 serial consoles"),
		cmds.WithLong(`
Scan serial consoles on Linux, and score devices that look like Espressif chips.

By default, scan probes chip identity via esptool for authoritative identification
(WARNING: may reset device). Disable with --probe-esptool=false.

Examples:
  esper scan
  esper scan --all
  esper scan --output json
  esper scan --fields preferred_path,vidpid,chip_description,usb_mode
  esper scan --probe-esptool=false
`),
		cmds.WithFlags(
			fields.New("all",
				fields.TypeBool,
				fields.WithDefault(false),
				fields.WithHelp("Show all serial ports, not only ESP32-ish devices"),
			),
			fields.New("prefer-by-id",
				fields.TypeBool,
				fields.WithDefault(true),
				fields.WithHelp("Prefer stable /dev/serial/by-id paths in output"),
			),
			fields.New("probe-esptool",
				fields.TypeBool,
				fields.WithDefault(true),
				fields.WithHelp("Probe chip identity via esptool (WARNING: may reset device into download mode). Disable with --probe-esptool=false"),
			),
			fields.New("esptool-connect-mode",
				fields.TypeString,
				fields.WithDefault("default_reset"),
				fields.WithHelp("esptool connect_mode (default_reset|usb_reset|no_reset|no_reset_no_sync)"),
			),
			fields.New("esptool-connect-attempts",
				fields.TypeInteger,
				fields.WithDefault(3),
				fields.WithHelp("esptool connect attempts"),
			),
			fields.New("esptool-after",
				fields.TypeString,
				fields.WithDefault("hard_reset"),
				fields.WithHelp("What to do after probing (hard_reset|no_reset)"),
			),
		),
		cmds.WithLayersList(glazedLayer, commandSettingsLayer),
	)

	return &ScanCommand{CommandDescription: cmdDesc}, nil
}

func (c *ScanCommand) RunIntoGlazeProcessor(
	ctx context.Context,
	vals *values.Values,
	gp middlewares.Processor,
) error {
	settings := &ScanSettings{}
	if err := values.DecodeSectionInto(vals, schema.DefaultSlug, settings); err != nil {
		return err
	}

	ports, err := scan.ScanLinux(ctx, scan.Options{
		All:                 settings.All,
		PreferByID:          settings.PreferByID,
		ProbeEsptool:        settings.ProbeEsptool,
		EsptoolConnectMode:  settings.EsptoolConnectMode,
		EsptoolConnectTries: settings.EsptoolConnectTries,
		EsptoolAfter:        settings.EsptoolAfter,
	})
	if err != nil {
		return err
	}

	reg, _, regErr := devices.Load()
	// Treat registry errors as non-fatal for scan output; scan itself should still work.
	if regErr != nil {
		reg = &devices.Registry{}
	}

	for _, p := range ports {
		reasons := strings.Join(p.Reasons, "; ")

		var (
			nickname    string
			deviceName  string
			description string
		)
		if reg != nil && p.Serial != "" {
			if e := reg.FindByUSBSerial(p.Serial); e != nil {
				nickname = e.Nickname
				deviceName = e.Name
				description = e.Description
			}
		}

		var (
			esptoolOK          bool
			esptoolError       string
			chipName           string
			chipDescription    string
			secureDownloadMode bool
			features           []string
			crystalMHz         int
			usbMode            string
			mac                string
		)
		if p.Esptool != nil {
			esptoolOK = p.Esptool.OK
			esptoolError = p.Esptool.Error
			chipName = p.Esptool.ChipName
			chipDescription = p.Esptool.ChipDescription
			secureDownloadMode = p.Esptool.SecureDLMode
			features = p.Esptool.Features
			crystalMHz = p.Esptool.CrystalMHz
			usbMode = p.Esptool.USBMode
			mac = p.Esptool.MAC
		}

		row := types.NewRow(
			types.MRP("device", p.Device),
			types.MRP("by_id", p.ByID),
			types.MRP("preferred_path", p.PreferredPath),
			types.MRP("nickname", nickname),
			types.MRP("device_name", deviceName),
			types.MRP("device_description", description),
			types.MRP("vid", p.VID),
			types.MRP("pid", p.PID),
			types.MRP("vidpid", p.VIDPID()),
			types.MRP("manufacturer", p.Manufacturer),
			types.MRP("product", p.Product),
			types.MRP("serial", p.Serial),
			types.MRP("score", p.Score),
			types.MRP("reasons", reasons),
			types.MRP("probe_esptool", settings.ProbeEsptool),
			types.MRP("esptool_ok", esptoolOK),
			types.MRP("esptool_error", esptoolError),
			types.MRP("chip_name", chipName),
			types.MRP("chip_description", chipDescription),
			types.MRP("secure_download_mode", secureDownloadMode),
			types.MRP("features", features),
			types.MRP("crystal_mhz", crystalMHz),
			types.MRP("usb_mode", usbMode),
			types.MRP("mac", mac),
		)

		if err := gp.AddRow(ctx, row); err != nil {
			return err
		}
	}

	return nil
}
