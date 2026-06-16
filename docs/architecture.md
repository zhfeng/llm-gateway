# Architecture

`llm-gateway` is a single-binary Go HTTP gateway for LLM clients. It exposes Anthropic-compatible and OpenAI-compatible endpoints, normalizes incoming requests into an internal protocol model, resolves a public model alias to a provider target, and forwards the request to Anthropic-compatible or OpenAI-compatible backends.

## Components

| Component | Package | Responsibility |
| --- | --- | --- |
| CLI entrypoint | `cmd/llm-gateway` | Load config, construct providers, registry, health manager, and HTTP server. |
| Config | `internal/config` | Parse JSON config, apply defaults, validate providers/routes, produce runtime settings. |
| HTTP API | `internal/httpapi` | Auth, request parsing, model resolution, retry/failover, response encoding. |
| Registry | `internal/models` | Store exposed model aliases, dynamic discovery results, weighted/sticky target selection. |
| Provider adapters | `internal/provider` | Convert internal protocol to provider wire format, execute outbound HTTP, stream SSE. |
| Health | `internal/health` | Track provider readiness state and expose routability decisions. |
| Protocol | `internal/protocol` | Internal request/response/message/tool/usage structs. |
| Stream | `internal/stream` | SSE read/write helpers. |
| Gateway errors | `internal/gwerror` | Normalize errors into Anthropic/OpenAI response envelopes. |

## Request Flow

Non-streaming request flow:

```text
client
  │
  ▼
/v1/messages or /v1/chat/completions
  │
  ▼
httpapi handler
  ├─ authenticate gateway API key
  ├─ read and size-limit body
  ├─ parse client protocol shape
  ├─ normalize into protocol.Request
  ├─ resolve model alias through models.Registry
  ├─ retry/failover across provider targets when enabled
  ▼
provider adapter
  ├─ convert protocol.Request to provider wire request
  ├─ send outbound HTTP with provider auth/transport
  ├─ decode provider response
  ▼
httpapi response writer
  └─ encode response in the client-requested protocol shape
```

Streaming request flow is similar, but provider adapters return a channel of `protocol.StreamEvent` values. The HTTP handlers convert those events into OpenAI or Anthropic SSE frames and flush them downstream.

## Model Alias and Provider Target Split

`protocol.Request` keeps the client-facing model and provider-facing model separate:

```go
Model         string // public alias requested by client
ProviderModel string // selected backend model/deployment
```

Handlers preserve `Model` and set `ProviderModel` after registry resolution. Provider adapters send `ProviderModel` upstream.

## Registry Snapshot Model

`internal/models.Registry` uses an `atomic.Value` snapshot for lock-free reads:

- `Resolve` / `ResolveWithOptions` read from the current snapshot.
- Dynamic model discovery rebuilds and swaps a new snapshot.
- Sticky routing state is stored separately under its own mutex.

Static config routes override dynamic discovery conflicts.

## HTTP Middleware Chain

The server composes request handling with discrete middleware layers. Each middleware owns one concern and returns an `http.Handler`:

```go
type Middleware func(http.Handler) http.Handler
```

`internal/server.Chain` applies middleware in the listed order, so the first middleware is the outermost layer. The API routes are grouped behind the auth middleware, while health routes remain outside that chain:

```text
request metrics → /v1 API mux → auth → httpapi handler
request metrics → /healthz or /readyz
```

This keeps authentication, request metrics, routing, and health handling separate. Request metrics currently preserve the timing hook without emitting per-request INFO logs.

## Provider Protection Layers

Provider instances are created once at startup and then wrapped with forwarding hooks and optional protection decorators:

```text
circuit breaker → concurrency limiter → forwarding hooks → HTTP provider
```

Forwarding hooks bracket actual provider calls. `BeforeProviderCall` runs immediately before an upstream `Complete` or `Stream` call, and `AfterProviderCall` runs after the call completes, fails, or a stream finishes. Admission-control rejections from the circuit breaker or concurrency limiter do not fire forwarding hooks because no upstream forwarding happened.

Health checks and model discovery bypass these decorators for now, so the gateway can still probe recovery and refresh model lists while user traffic is limited or circuits are open.

## Health Endpoints

- `/healthz` — static process liveness.
- `/readyz` — provider readiness detail from `internal/health.Manager`.

`/readyz` returns `503` only when health checks are enabled and at least one enabled provider is unhealthy.

## Change Rule

When adding a new routing policy, provider adapter, config field, or health/failover behavior, update:

- `reference/configuration.md` for config fields.
- `concepts/routing.md` for routing semantics.
- `concepts/provider-health.md` for health/protection behavior.
