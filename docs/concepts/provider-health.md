# Provider Health and Protection

Provider health and protection features help the gateway stay responsive when an upstream provider is slow, unhealthy, saturated, or repeatedly failing.

## Health Checks

Global health checks are disabled by default:

```json
"health": {
  "enabled": false,
  "interval": "30s",
  "timeout": "5s",
  "failure_threshold": 2,
  "success_threshold": 1
}
```

Provider-specific probes default to `GET /models` and expected status `200`:

```json
"providers": [
  {
    "name": "openai",
    "health": {
      "enabled": true,
      "probe_path": "/models",
      "probe_method": "GET",
      "expected_status": [200]
    }
  }
]
```

For Anthropic-compatible providers, probe paths use the same endpoint construction rules as other provider calls. If the base URL does not end in `/v1`, the gateway inserts `/v1` before the probe path.

## Readiness Endpoint

- `GET /healthz` — liveness only, always checks the gateway process.
- `GET /readyz` — includes provider health details.

`/readyz` returns `503` when health checks are enabled and an enabled provider is unhealthy.

## Passive Health Updates

Provider call results are recorded into the health manager. Local gateway admission errors, such as concurrency-limit and circuit-open rejections, are ignored so they do not mark the provider unhealthy.

## Concurrency Limits

Per-provider concurrency limits are disabled by default:

```json
"concurrency": {
  "max_in_flight": 0
}
```

A positive value limits concurrent `Complete` and `Stream` calls for that provider. Streaming calls hold a slot until the stream ends. `HealthCheck` and `ListModels` bypass concurrency limits.

If the limit is full, the gateway returns a local `429 rate_limit_error`. With retry enabled and 429 in `retry_on_status`, the gateway can fail over to another provider target.

## Circuit Breaker

Circuit breakers are disabled by default:

```json
"circuit_breaker": {
  "enabled": false,
  "failure_threshold": 5,
  "success_threshold": 1,
  "open_timeout": "30s"
}
```

States:

- `closed` — calls are allowed; failures are counted.
- `open` — calls fail fast with local `503 provider_error`.
- `half_open` — one trial call is allowed after `open_timeout`.

Circuit breaker failure signals include retryable provider HTTP errors, network errors, and timeouts. Client cancellation and local admission errors do not open the circuit.

`HealthCheck` and `ListModels` bypass circuit breakers so the gateway can probe recovery and refresh model discovery independently of user traffic.

## Operational Guidance

Start with conservative settings:

```json
"routing": {
  "retry": {
    "enabled": true,
    "max_attempts": 2
  }
},
"providers": [
  {
    "name": "provider-a",
    "concurrency": { "max_in_flight": 64 },
    "circuit_breaker": { "enabled": true }
  }
]
```

Increase `max_in_flight` based on provider limits, gateway CPU/memory, and observed streaming duration. Use `/readyz` to inspect provider state while testing.
