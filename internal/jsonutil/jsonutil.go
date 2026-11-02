// Package jsonutil encodes command output as JSON.
package jsonutil

import "encoding/json"

// Marshal encodes v compactly, or indented by two spaces when pretty is set.
func Marshal(v any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}
