# Development

## Common Commands

```bash
make build
make test
```

Equivalent Go commands:

```bash
go build -o bin/llm-gateway ./cmd/llm-gateway
go test ./...
```

## Run Locally

```bash
go run ./cmd/llm-gateway -config config.example.json
```

Or with the built binary:

```bash
./bin/llm-gateway -config config.example.json
```

A local provider-specific test config can live under `bin/` and remain untracked:

```bash
./bin/llm-gateway -config bin/config.volcengine.example.json
```

## Formatting

```bash
gofmt -w cmd internal
```

## Test Targets

Core package tests:

```bash
go test ./internal/config ./internal/models ./internal/provider ./internal/httpapi
```

Full suite:

```bash
go test ./...
```

Race checks for routing/health/concurrency code:

```bash
go test -race ./internal/provider ./internal/httpapi ./internal/health ./internal/models
```

## Manual Smoke Test

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY"
```

Anthropic-compatible message:

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"code-main","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}'
```

OpenAI-compatible chat completion:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"code-main","messages":[{"role":"user","content":"hello"}]}'
```
