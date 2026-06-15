# llm-gateway

A Go LLM gateway that exposes Anthropic-compatible and OpenAI-compatible endpoints for Claude Code, Codex-style agents, SDK clients, and other tools.

## Endpoints

- `GET /healthz`
- `GET /v1/models`
- `POST /v1/messages`
- `POST /v1/chat/completions`

The gateway accepts Anthropic-style and OpenAI-style client requests, resolves the requested model through a unified model registry, and forwards the request to an `anthropic_compatible` or `openai_compatible` provider backend.

## Run

```bash
go run ./cmd/llm-gateway -config config.example.json
```

Optional address override:

```bash
go run ./cmd/llm-gateway -config config.example.json -addr 127.0.0.1:8080
```

## Config

Providers use a `base_url` that already includes the API prefix, usually `/v1`. The gateway appends endpoint paths such as `/messages`, `/chat/completions`, and `/models`.

```json
{
  "server": {
    "addr": "127.0.0.1:8080",
    "read_header_timeout": "10s",
    "read_timeout": "30s",
    "write_timeout": "0s",
    "idle_timeout": "120s",
    "max_body_bytes": 10485760
  },
  "auth": {
    "disable": false,
    "api_keys_env": ["LLM_GATEWAY_API_KEY"]
  },
  "debug": {
    "log_messages": false
  },
  "model_discovery": {
    "ttl": "10m",
    "stale_while_revalidate": true
  },
  "providers": [
    {
      "name": "anthropic",
      "type": "anthropic_compatible",
      "base_url": "https://api.anthropic.com/v1",
      "api_key_env": "ANTHROPIC_API_KEY",
      "transport": {
        "max_idle_conns": 1024,
        "max_idle_conns_per_host": 256,
        "idle_conn_timeout": "90s",
        "dial_timeout": "10s",
        "dial_keep_alive": "30s",
        "tls_handshake_timeout": "10s",
        "expect_continue_timeout": "1s",
        "force_attempt_http2": true
      },
      "discover_models": true,
      "model_prefix": "anthropic/",
      "include_models": ["*"],
      "exclude_models": []
    },
    {
      "name": "openai",
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "api_key_env": "OPENAI_API_KEY",
      "discover_models": true,
      "model_prefix": "openai/",
      "include_models": ["*"],
      "exclude_models": []
    }
  ],
  "models": {
    "claude-sonnet": {
      "provider": "anthropic",
      "provider_model": "claude-sonnet-4-6"
    },
    "gpt-main": {
      "provider": "openai",
      "provider_model": "gpt-4.1"
    }
  }
}
```

Static `models` entries are exposed exactly as configured and take precedence over dynamically discovered models. Dynamic models discovered from provider `/models` use `model_prefix` when configured.

A static model alias can route across multiple provider targets with weighted selection:

```json
{
  "models": {
    "code-main": {
      "policy": { "type": "weighted" },
      "targets": [
        { "provider": "anthropic", "provider_model": "claude-sonnet-4-6", "weight": 80 },
        { "provider": "openai", "provider_model": "gpt-4.1", "weight": 20 }
      ]
    }
  }
}
```

Weighted routing is sticky by default so coding-agent conversations keep using the same selected provider. The gateway first looks for `X-LLM-Gateway-Sticky-Key`; if absent, it falls back to a hash of the gateway auth key when `routing.sticky_weighted.fallback` is `auth_key`:

```json
{
  "routing": {
    "sticky_weighted": {
      "enabled": true,
      "header": "X-LLM-Gateway-Sticky-Key",
      "fallback": "auth_key",
      "ttl": "24h",
      "max_entries": 10000
    }
  }
}
```

Provider health checks and retry/failover are opt-in. `/healthz` remains static liveness; `/readyz` includes provider health details when enabled:

```json
{
  "health": {
    "enabled": false,
    "interval": "30s",
    "timeout": "5s",
    "failure_threshold": 2,
    "success_threshold": 1
  },
  "routing": {
    "retry": {
      "enabled": false,
      "max_attempts": 1,
      "backoff": "200ms",
      "max_backoff": "1s",
      "retry_on_status": [408, 429, 500, 502, 503, 504],
      "retry_on_network_error": true,
      "retry_on_timeout": true
    }
  }
}
```

Providers can also opt into per-provider probes, concurrency limits, and circuit breakers:

```json
{
  "health": {
    "enabled": true,
    "probe_path": "/models",
    "probe_method": "GET",
    "expected_status": [200]
  },
  "concurrency": {
    "max_in_flight": 32
  },
  "circuit_breaker": {
    "enabled": true,
    "failure_threshold": 5,
    "success_threshold": 1,
    "open_timeout": "30s"
  }
}
```

Concurrency-limit errors return `429`; open circuit breaker errors return `503`. When retry is enabled and those statuses are retryable, the gateway can fail over to another target.

Dynamic discovery can be filtered per provider with shell-style glob patterns:

```json
{
  "include_models": ["doubao-*", "deepseek-*"],
  "exclude_models": ["*-embedding-*", "*-t2i-*"]
}
```

`include_models` is applied first. If it is empty, all discovered models are included by default. `exclude_models` is applied after includes. Static `models` aliases are not affected by these filters.

Set `debug.log_messages` to `true` to log incoming client bodies, outgoing provider bodies, and upstream streaming events. These logs may contain prompt and completion content, so keep it disabled outside local debugging.

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
    "ANTHROPIC_MODEL": "ark-code-latest",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "ark-code-latest",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "ark-code-latest",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "ark-code-latest"
  }
}
```

This mirrors a direct Volcengine Ark setup where Claude Code points at `https://ark.cn-beijing.volces.com/api/coding`, but moves that provider connection behind the gateway.

## Volcengine Ark Anthropic-compatible provider

Volcengine Ark exposes an Anthropic-compatible endpoint that can be configured as a gateway provider.

The relevant provider config is:

```json
{
  "name": "volcengine-ark",
  "type": "anthropic_compatible",
  "base_url": "https://ark.cn-beijing.volces.com/api/coding",
  "api_key_env": "VOLCENGINE_API_KEY",
  "api_key_header": "Authorization",
  "api_key_scheme": "Bearer",
  "discover_models": true,
  "model_prefix": "volcengine/",
  "include_models": ["*"],
  "exclude_models": ["*-embedding-*", "*-t2i-*", "*-i2i-*", "*-t2v-*", "*-i2v-*"]
}
```

Then point Claude Code at the gateway:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8080/v1",
    "ANTHROPIC_AUTH_TOKEN": "local-gateway-key",
    "ANTHROPIC_MODEL": "ark-code-latest"
  }
}
```

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
