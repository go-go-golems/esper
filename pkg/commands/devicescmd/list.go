package devicescmd

import (
	"context"

	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/go-go-golems/glazed/pkg/cmds/values"
	"github.com/go-go-golems/glazed/pkg/middlewares"
	"github.com/go-go-golems/glazed/pkg/types"
)

type ListCommand struct {
	*cmds.CommandDescription
}

var _ cmds.GlazeCommand = (*ListCommand)(nil)

func NewListCommand() (*ListCommand, error) {
	glazedLayer, err := schema.NewGlazedSchema()
	if err != nil {
		return nil, err
	}

	commandSettingsLayer, err := cli.NewCommandSettingsLayer()
	if err != nil {
		return nil, err
	}

	cmdDesc := cmds.NewCommandDescription(
		"list",
		cmds.WithShort("List known devices from the registry"),
		cmds.WithLong(`
List devices from the per-user registry (XDG config).

Examples:
  esper devices list
  esper devices list --output json
  esper devices list --fields usb_serial,nickname,name
`),
		cmds.WithLayersList(glazedLayer, commandSettingsLayer),
	)

	return &ListCommand{CommandDescription: cmdDesc}, nil
}

func (c *ListCommand) RunIntoGlazeProcessor(
	ctx context.Context,
	vals *values.Values,
	gp middlewares.Processor,
) error {
	_ = vals

	reg, path, err := devices.Load()
	if err != nil {
		return err
	}

	for _, d := range reg.Devices {
		row := types.NewRow(
			types.MRP("config_path", path),
			types.MRP("usb_serial", d.USBSerial),
			types.MRP("nickname", d.Nickname),
			types.MRP("name", d.Name),
			types.MRP("description", d.Description),
			types.MRP("preferred_path", d.PreferredPath),
		)
		if err := gp.AddRow(ctx, row); err != nil {
			return err
		}
	}

	return nil
}
