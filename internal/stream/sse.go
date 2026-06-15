package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Event struct {
	Name string
	Data []byte
}

func ReadEvents(ctx context.Context, r io.Reader, emit func(Event) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var name string
	var data bytes.Buffer

	flush := func() error {
		if data.Len() == 0 {
			name = ""
			return nil
		}
		event := Event{Name: name, Data: bytes.TrimSuffix(data.Bytes(), []byte("\n"))}
		name = ""
		data.Reset()
		return emit(event)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if ok {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			name = value
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func WriteEvent(w http.ResponseWriter, event string, value any) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return err
		}
		data = encoded
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func Flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
