package operatorinput

import (
	"bytes"
	"encoding/json"
)

// StripResizeMessages removes xterm.js resize JSON frames from operator input.
// Browsers send these as text WebSocket frames; command-mode agents must not run them as shell input.
func StripResizeMessages(data []byte) []byte {
	var out bytes.Buffer
	i := 0
	for i < len(data) {
		if data[i] == '{' {
			end := jsonObjectEnd(data[i:])
			if end > 0 {
				var hdr struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data[i:i+end], &hdr) == nil && hdr.Type == "resize" {
					i += end
					continue
				}
			}
		}
		out.WriteByte(data[i])
		i++
	}
	return out.Bytes()
}

func jsonObjectEnd(b []byte) int {
	depth := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}

// ForCommandMode returns data with resize control messages removed when mode is "command".
func ForCommandMode(mode string, data []byte) []byte {
	if mode != "command" {
		return data
	}
	return StripResizeMessages(data)
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// ParseResize returns cols, rows, true when data is an xterm resize control frame.
func ParseResize(data []byte) (cols, rows int, ok bool) {
	if len(data) == 0 || data[0] != '{' {
		return 0, 0, false
	}
	var msg resizeMsg
	if json.Unmarshal(data, &msg) != nil || msg.Type != "resize" || msg.Cols <= 0 || msg.Rows <= 0 {
		return 0, 0, false
	}
	return msg.Cols, msg.Rows, true
}

func IsResizeMessage(data []byte) bool {
	_, _, ok := ParseResize(data)
	return ok
}
