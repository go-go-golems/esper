package devicescmd

import (
	"fmt"
	"os"

	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/spf13/cobra"
)

func NewDevicesCommand() (*cobra.Command, error) {
	devicesCmd := &cobra.Command{
		Use:   "devices",
		Short: "Manage the per-user device registry",
	}

	listCmd, err := NewListCommand()
	if err != nil {
		return nil, err
	}
	cobraListCmd, err := cli.BuildCobraCommand(listCmd,
		cli.WithParserConfig(cli.CobraParserConfig{
			ShortHelpLayers: []string{schema.DefaultSlug},
			MiddlewaresFunc: cli.CobraCommandDefaultMiddlewares,
		}),
	)
	if err != nil {
		return nil, err
	}
	devicesCmd.AddCommand(cobraListCmd)

	devicesCmd.AddCommand(newPathCommand())
	devicesCmd.AddCommand(newSetCommand())
	devicesCmd.AddCommand(newRemoveCommand())
	devicesCmd.AddCommand(newResolveCommand())

	return devicesCmd, nil
}

func newPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the registry config path",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := devices.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, p)
			return nil
		},
	}
}

func newSetCommand() *cobra.Command {
	var e devices.DeviceEntry

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Create or update a device registry entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, _, err := devices.Load()
			if err != nil {
				return err
			}
			if err := reg.Upsert(e); err != nil {
				return err
			}
			path, err := devices.Save(reg)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "saved %q\n", path)
			return nil
		},
	}

	cmd.Flags().StringVar(&e.USBSerial, "usb-serial", "", "USB serial string (from `esper scan` column serial)")
	cmd.Flags().StringVar(&e.Nickname, "nickname", "", "Human-friendly nickname used for resolution")
	cmd.Flags().StringVar(&e.Name, "name", "", "Descriptive device name")
	cmd.Flags().StringVar(&e.Description, "description", "", "Longer description/notes")
	cmd.Flags().StringVar(&e.PreferredPath, "preferred-path", "", "Optional preferred device path (usually /dev/serial/by-id/...)")

	_ = cmd.MarkFlagRequired("usb-serial")
	_ = cmd.MarkFlagRequired("nickname")

	return cmd
}

func newRemoveCommand() *cobra.Command {
	var usbSerial string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a device registry entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, _, err := devices.Load()
			if err != nil {
				return err
			}
			if !reg.RemoveByUSBSerial(usbSerial) {
				return fmt.Errorf("no entry with usb_serial=%q", usbSerial)
			}
			path, err := devices.Save(reg)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "saved %q\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&usbSerial, "usb-serial", "", "USB serial string to remove")
	_ = cmd.MarkFlagRequired("usb-serial")
	return cmd
}

func newResolveCommand() *cobra.Command {
	var nickname string
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve a nickname to a current port path",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := devices.ResolveNicknameToPort(cmd.Context(), nickname)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, res.PortPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&nickname, "nickname", "", "Nickname to resolve")
	_ = cmd.MarkFlagRequired("nickname")
	return cmd
}
