package provider

import (
	"errors"
	"net/http"

	"github.com/zhfeng/llm-gateway/internal/gwerror"
)

type AdmissionKind string

const (
	AdmissionConcurrencyLimit AdmissionKind = "concurrency_limit"
	AdmissionCircuitOpen      AdmissionKind = "circuit_open"
)

type AdmissionError struct {
	Kind string
	Err  *gwerror.Error
}

func (e *AdmissionError) Error() string { return e.Err.Error() }
func (e *AdmissionError) Unwrap() error { return e.Err }

func IsAdmissionError(err error) bool {
	var admission *AdmissionError
	return errors.As(err, &admission)
}

func newConcurrencyLimitError(provider string) error {
	return &AdmissionError{Kind: string(AdmissionConcurrencyLimit), Err: &gwerror.Error{Status: http.StatusTooManyRequests, Type: "rate_limit_error", Message: "provider concurrency limit exceeded", Provider: provider}}
}

func newCircuitOpenError(provider string) error {
	return &AdmissionError{Kind: string(AdmissionCircuitOpen), Err: &gwerror.Error{Status: http.StatusServiceUnavailable, Type: "provider_error", Message: "provider circuit breaker is open", Provider: provider}}
}
