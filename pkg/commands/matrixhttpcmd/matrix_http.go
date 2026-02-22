package matrixhttpcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-go-golems/glazed/pkg/cli"
	"github.com/go-go-golems/glazed/pkg/cmds"
	"github.com/go-go-golems/glazed/pkg/cmds/fields"
	"github.com/go-go-golems/glazed/pkg/cmds/schema"
	"github.com/go-go-golems/glazed/pkg/cmds/values"
	"github.com/go-go-golems/glazed/pkg/middlewares"
	"github.com/go-go-golems/glazed/pkg/types"
)

type MatrixHTTPCommand struct {
	*cmds.CommandDescription
}

var _ cmds.GlazeCommand = (*MatrixHTTPCommand)(nil)

type MatrixHTTPSettings struct {
	BaseURL   string `glazed.parameter:"base-url"`
	Action    string `glazed.parameter:"action"`
	Text      string `glazed.parameter:"text"`
	Mode      string `glazed.parameter:"mode"`
	FPS       int    `glazed.parameter:"fps"`
	PauseMS   int    `glazed.parameter:"pause-ms"`
	TimeoutMS int    `glazed.parameter:"timeout-ms"`
}

func NewMatrixHTTPCommand() (*MatrixHTTPCommand, error) {
	glazedLayer, err := schema.NewGlazedSchema()
	if err != nil {
		return nil, err
	}

	commandSettingsLayer, err := cli.NewCommandSettingsLayer()
	if err != nil {
		return nil, err
	}

	cmdDesc := cmds.NewCommandDescription(
		"matrix-http",
		cmds.WithShort("Control MAX7219 matrix firmware over HTTP"),
		cmds.WithLong(`
Control the ESP32 matrix HTTP API from the host.

Actions:
  status   -> GET  /api/matrix/status
  text     -> POST /api/matrix/text      {"text":"..."}
  anim     -> POST /api/matrix/anim      {"mode":"scroll|wave|drop","text":"...","fps":N,"pause_ms":N}
  stop     -> POST /api/matrix/stop      {}

Examples:
  esper matrix-http --base-url http://192.168.3.119 --action status
  esper matrix-http --base-url http://192.168.3.119 --action text --text HELLO
  esper matrix-http --base-url http://192.168.3.119 --action anim --mode wave --text WIFI --fps 20 --pause-ms 250
  esper matrix-http --base-url http://192.168.3.119 --action stop
`),
		cmds.WithFlags(
			fields.New("base-url",
				fields.TypeString,
				fields.WithDefault("http://192.168.3.119"),
				fields.WithHelp("Matrix firmware base URL, for example http://192.168.3.119"),
			),
			fields.New("action",
				fields.TypeChoice,
				fields.WithChoices("status", "text", "anim", "stop"),
				fields.WithDefault("status"),
				fields.WithHelp("HTTP action to execute"),
			),
			fields.New("text",
				fields.TypeString,
				fields.WithDefault(""),
				fields.WithHelp("Text payload for action=text|anim"),
			),
			fields.New("mode",
				fields.TypeChoice,
				fields.WithChoices("scroll", "wave", "drop"),
				fields.WithDefault("scroll"),
				fields.WithHelp("Animation mode for action=anim"),
			),
			fields.New("fps",
				fields.TypeInteger,
				fields.WithDefault(15),
				fields.WithHelp("Animation FPS for action=anim"),
			),
			fields.New("pause-ms",
				fields.TypeInteger,
				fields.WithDefault(250),
				fields.WithHelp("Animation loop pause (ms) for action=anim"),
			),
			fields.New("timeout-ms",
				fields.TypeInteger,
				fields.WithDefault(5000),
				fields.WithHelp("HTTP request timeout in milliseconds"),
			),
		),
		cmds.WithLayersList(glazedLayer, commandSettingsLayer),
	)

	return &MatrixHTTPCommand{CommandDescription: cmdDesc}, nil
}

func (c *MatrixHTTPCommand) RunIntoGlazeProcessor(
	ctx context.Context,
	vals *values.Values,
	gp middlewares.Processor,
) error {
	settings := &MatrixHTTPSettings{}
	if err := values.DecodeSectionInto(vals, schema.DefaultSlug, settings); err != nil {
		return err
	}

	base := strings.TrimRight(settings.BaseURL, "/")
	if base == "" {
		return fmt.Errorf("base-url must not be empty")
	}

	method, path, payload, err := prepareRequest(settings)
	if err != nil {
		return err
	}

	reqURL := base + path
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}

	timeout := time.Duration(settings.TimeoutMS) * time.Millisecond
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return err
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	respBody := string(respBytes)

	var respJSON map[string]interface{}
	if err := json.Unmarshal(respBytes, &respJSON); err != nil {
		respJSON = map[string]interface{}{}
	}

	okField := false
	if v, ok := respJSON["ok"].(bool); ok {
		okField = v
	}

	row := types.NewRow(
		types.MRP("base_url", base),
		types.MRP("action", settings.Action),
		types.MRP("method", method),
		types.MRP("path", path),
		types.MRP("url", reqURL),
		types.MRP("http_status", resp.StatusCode),
		types.MRP("ok", okField),
		types.MRP("response", respJSON),
		types.MRP("response_body", respBody),
	)
	if err := gp.AddRow(ctx, row); err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("request failed: %s: %s", resp.Status, respBody)
	}

	return nil
}

func prepareRequest(s *MatrixHTTPSettings) (method string, path string, payload []byte, err error) {
	switch s.Action {
	case "status":
		return http.MethodGet, "/api/matrix/status", nil, nil
	case "text":
		p, e := json.Marshal(map[string]interface{}{
			"text": s.Text,
		})
		return http.MethodPost, "/api/matrix/text", p, e
	case "anim":
		p, e := json.Marshal(map[string]interface{}{
			"mode":     s.Mode,
			"text":     s.Text,
			"fps":      s.FPS,
			"pause_ms": s.PauseMS,
		})
		return http.MethodPost, "/api/matrix/anim", p, e
	case "stop":
		return http.MethodPost, "/api/matrix/stop", []byte(`{}`), nil
	default:
		return "", "", nil, fmt.Errorf("unsupported action: %s", s.Action)
	}
}
