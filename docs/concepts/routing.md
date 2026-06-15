# Routing

Routing maps a client-facing model name to one concrete provider target for a request.

## Public Model Aliases

Clients request public aliases:

```json
{
  "model": "code-main",
  "messages": [{ "role": "user", "content": "hello" }]
}
```

The gateway resolves that alias to a provider and provider-specific model/deployment.

Legacy single-target route:

```json
"models": {
  "code-main": {
    "provider": "volcengine-ark",
    "provider_model": "ark-code-latest"
  }
}
```

## Weighted Multi-Provider Routes

A public alias can route across several provider targets:

```json
"models": {
  "code-main": {
    "policy": { "type": "weighted" },
    "targets": [
      { "provider": "volcengine-ark", "provider_model": "ark-code-latest", "weight": 80 },
      { "provider": "minimax", "provider_model": "MiniMax-M2", "weight": 20 }
    ]
  }
}
```

Weights are relative. `80/20` is equivalent to `4/1`.

## Sticky Weighted Routing

Weighted routes are sticky by default. This keeps coding-agent conversations on the same selected provider for consistency.

Sticky key order:

1. Configured header, default `X-LLM-Gateway-Sticky-Key`.
2. Gateway auth key hash when `routing.sticky_weighted.fallback` is `auth_key`.
3. No sticky key, fallback to normal weighted random selection.

Config:

```json
"routing": {
  "sticky_weighted": {
    "enabled": true,
    "header": "X-LLM-Gateway-Sticky-Key",
    "fallback": "auth_key",
    "ttl": "24h",
    "max_entries": 10000
  }
}
```

Sticky bindings are in-memory. If you run multiple gateway replicas, use load-balancer stickiness, an explicit stable header routed to the same replica, or add shared sticky state in a future change.

## Retry and Failover

Retry/failover is opt-in:

```json
"routing": {
  "retry": {
    "enabled": true,
    "max_attempts": 2,
    "retry_on_status": [408, 429, 500, 502, 503, 504],
    "retry_on_network_error": true,
    "retry_on_timeout": true
  }
}
```

Attempt order:

1. Primary route selected by registry, including sticky target when present.
2. Other targets on the same model alias.
3. Healthy/unknown providers are preferred over unhealthy providers.
4. If all targets are unhealthy, the gateway fails open and still attempts the original ordering.

Non-streaming retry can fail over transparently before a response is written.

Streaming failover is only attempted before downstream headers/SSE output are written. After streaming output begins, the gateway preserves the current stream and emits an error event if the provider stream fails.

## Interaction with Sticky Routing

Sticky routing is a preference, not a hard pin. If the sticky provider is unhealthy or locally unavailable and retry/failover is enabled, the gateway can temporarily try another target. The sticky binding is not rewritten by failover in the current implementation.
