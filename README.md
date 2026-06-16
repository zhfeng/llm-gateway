# llm-gateway

A Go LLM gateway that exposes Anthropic-compatible and OpenAI-compatible endpoints for Claude Code, Codex-style agents, SDK clients, and other tools.

## Endpoints

- `GET /healthz`
- `GET /v1/models`
- `POST /v1/messages`
- `POST /v1/chat/completions`

The gateway accepts Anthropic-style and OpenAI-style client requests, resolves the requested model through a unified model registry, and forwards the request to an `anthropic_compatible` or `openai_compatible` provider backend.

Longer architecture, configuration, and operations notes live in [`docs/`](docs/).

## Run

```bash
go run ./cmd/llm-gateway -config config.example.json
```

Optional address override:

```bash
go run ./cmd/llm-gateway -config config.example.json -addr 127.0.0.1:8080
```

## Architecture

`llm-gateway` is a single-binary Go HTTP gateway. It accepts Anthropic-compatible and OpenAI-compatible client requests, normalizes them into an internal protocol model, resolves the requested public model alias through a registry, and forwards the request to a selected provider adapter.

Core components:

- `cmd/llm-gateway` loads configuration and wires the server.
- `internal/httpapi` handles auth, request parsing, routing, retry/failover, and response encoding.
- `internal/models` stores static aliases, dynamic discovery results, and weighted/sticky routing state.
- `internal/provider` adapts the internal protocol to Anthropic-compatible or OpenAI-compatible upstream APIs.
- `internal/health` tracks provider readiness for `/readyz` and routing decisions.

Request flow:

```text
client request
  → HTTP API handler
  → gateway auth and body limits
  → protocol normalization
  → model registry resolution
  → provider adapter
  → upstream LLM backend
  → protocol-specific response encoder
```

Streaming follows the same path, but provider adapters return internal stream events that the HTTP layer flushes as Anthropic or OpenAI SSE frames.

See [`docs/architecture.md`](docs/architecture.md) for the full component map and request flow. See [`docs/reference/configuration.md`](docs/reference/configuration.md) for configuration fields, including provider setup, routing aliases, health checks, and transport options.

## Testing without gateway authentication

Client-to-gateway API key authentication is enabled by default because `auth.disable` defaults to `false`. You can disable it for local testing only:

```json
{
  "auth": {
    "disable": true
  }
}
```

When this is enabled, the gateway logs a warning at startup. Do not use this in production: anyone who can reach the gateway can call your configured provider backends and spend your provider credentials.

## Claude Code-style usage

Point an Anthropic-compatible client at the gateway base URL:

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:8080/v1
ANTHROPIC_AUTH_TOKEN=$LLM_GATEWAY_API_KEY
```

For Claude Code, you can also configure `~/.claude/settings.json` to call this gateway instead of a provider directly:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8080/v1",
    "ANTHROPIC_AUTH_TOKEN": "${LLM_GATEWAY_API_KEY}",
    "ANTHROPIC_MODEL": "code-main",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "code-fast",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "code-main",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "code-large"
  }
}
```

Use any configured gateway model alias for `ANTHROPIC_MODEL`, such as `code-main`, `code-fast`, or `code-large`.

## OpenAI/Codex-style usage

Point an OpenAI-compatible client at the gateway base URL:

```bash
OPENAI_BASE_URL=http://127.0.0.1:8080/v1
OPENAI_API_KEY=$LLM_GATEWAY_API_KEY
```

## Curl examples

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY"
```

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-main","messages":[{"role":"user","content":"hello"}]}'
```

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}'
```

Streaming:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $LLM_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-main","stream":true,"messages":[{"role":"user","content":"hello"}]}'
```

## Performance notes

- One long-lived HTTP client is created per provider.
- Provider transports are configured with high idle connection limits for concurrent gateway traffic; tune per provider with `transport.max_idle_conns`, `transport.max_idle_conns_per_host`, and timeout fields.
- Model routing is O(1) through a precomputed registry snapshot.
- Streaming requests flush SSE events and cancel upstream calls when the client disconnects.
- Prompt bodies are not logged by default.
