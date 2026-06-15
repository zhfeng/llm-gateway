package provider

import (
	"encoding/json"
	"log/slog"
)

func logJSON(message string, args ...any) {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] == "payload" {
			if data, err := json.Marshal(args[i+1]); err == nil {
				args[i+1] = string(data)
			}
		}
	}
	slog.Info(message, args...)
}
