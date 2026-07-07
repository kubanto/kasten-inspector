package report

import (
	"encoding/json"
	"os"
)

// WriteJSON writes the report as indented JSON.
func WriteJSON(path string, data *Data) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
