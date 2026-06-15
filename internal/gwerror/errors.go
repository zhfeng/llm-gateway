package gwerror

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Error struct {
	Status   int
	Type     string
	Code     string
	Message  string
	Provider string
	Raw      []byte
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func New(status int, typ, message string) *Error {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	if typ == "" {
		typ = typeForStatus(status)
	}
	return &Error{Status: status, Type: typ, Message: message}
}

func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var ge *Error
	if errors.As(err, &ge) {
		return ge
	}
	return New(http.StatusInternalServerError, "server_error", err.Error())
}

func typeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return "timeout_error"
	default:
		if status >= 500 {
			return "provider_error"
		}
		return "invalid_request_error"
	}
}

func WriteOpenAI(w http.ResponseWriter, err error) {
	ge := FromError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ge.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message":  ge.Message,
			"type":     ge.Type,
			"code":     ge.Code,
			"provider": ge.Provider,
		},
	})
}

func WriteAnthropic(w http.ResponseWriter, err error) {
	ge := FromError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ge.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":     ge.Type,
			"message":  ge.Message,
			"provider": ge.Provider,
		},
	})
}
