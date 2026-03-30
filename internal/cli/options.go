package cli

import (
	"encoding/json"
	"os"
)

// Options holds persistent TUI settings saved to a JSON file.
type Options struct {
	NoLogs     bool `json:"no_logs"`
	TempServer bool `json:"temp_server"`
}

func loadOptions(path string) Options {
	data, err := os.ReadFile(path)
	if err != nil {
		return Options{}
	}
	var o Options
	if err := json.Unmarshal(data, &o); err != nil {
		return Options{}
	}
	return o
}

func saveOptions(path string, o Options) error {
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
