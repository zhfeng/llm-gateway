# Troubleshooting

## Claude Code reports the selected model does not exist

Check which model alias the gateway exposes:

```bash
curl http://127.0.0.1:11434/v1/models
```

The requested model must match a key under `models` or a dynamically discovered model ID.

For Claude Code with this gateway, use the gateway root as the base URL:

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:11434
```

Do not include `/v1` if your Claude Code version appends `/v1/messages` itself, or requests may hit `/v1/v1/messages`.

## Provider returns 401 or invalid API key

Check the provider API key environment variable used by the provider config:

```bash
echo "$VOLCENGINE_API_KEY"
echo "$ANTHROPIC_API_KEY"
echo "$OPENAI_API_KEY"
```

The gateway process must inherit those variables when it starts.

## Gateway returns gateway API key errors

If `auth.disable` is false, clients must send either:

```text
Authorization: Bearer <gateway key>
x-api-key: <gateway key>
```

The gateway key comes from `auth.api_keys` or `auth.api_keys_env`.

For local-only testing, you can disable gateway auth:

```json
"auth": { "disable": true }
```

Do not use this in production.

## Check provider health

Gateway liveness:

```bash
curl http://127.0.0.1:11434/healthz
```

Provider readiness details:

```bash
curl http://127.0.0.1:11434/readyz
```

Provider health polling only runs when:

```json
"health": { "enabled": true }
```

## Debug message flow

Enable local-only message logging:

```json
"debug": { "log_messages": true }
```

Logs include incoming client bodies, outgoing provider payloads, provider responses, and upstream stream events. These logs may contain prompts and completions.

## Weighted route does not distribute as expected

Check whether sticky routing is enabled. It is enabled by default for coding-agent stability:

```json
"routing": {
  "sticky_weighted": {
    "enabled": true
  }
}
```

With stickiness, the same sticky key keeps using the same provider target until TTL expiry. To observe raw weighted distribution, disable stickiness or vary the sticky key.

## Retry/failover does not happen

Retry is disabled by default. Enable it and set `max_attempts` above `1`:

```json
"routing": {
  "retry": {
    "enabled": true,
    "max_attempts": 2
  }
}
```

Only configured statuses/network errors/timeouts retry. Non-retryable client/provider auth errors should fail directly.

Streaming failover only happens before response headers/SSE output are sent. Mid-stream failover is intentionally not supported.

## Provider is skipped or fails fast

Check per-provider protection config:

```json
"concurrency": { "max_in_flight": 32 },
"circuit_breaker": { "enabled": true }
```

Concurrency saturation returns local `429`. An open circuit breaker returns local `503`. With retry enabled, the gateway can fail over to another target for those statuses.

## Dynamic models are missing

Dynamic discovery only runs for providers with:

```json
"discover_models": true
```

Filters are applied to provider model IDs before prefixing:

```json
"include_models": ["doubao-*"],
"exclude_models": ["*-embedding-*"]
```

Static `models` entries always take precedence over dynamic discoveries.
