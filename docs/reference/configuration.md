# Configuration Reference

The gateway loads one JSON config file via:

```bash
llm-gateway -config config.example.json
```

## Top-Level Shape

```json
{
  "server": {},
  "auth": {},
  "debug": {},
  "health": {},
  "routing": {},
  "model_discovery": {},
  "providers": [],
  "models": {}
}
```

## `server`

| Field | Default | Meaning |
| --- | --- | --- |
| `addr` | `127.0.0.1:8080` | Listen address. |
| `read_header_timeout` | `10s` | Max time to read request headers. |
| `read_timeout` | `30s` | Max time to read entire request. |
| `write_timeout` | `0s` | Max response write time; `0s` disables. |
| `idle_timeout` | `120s` | Keep-alive idle timeout. |
| `max_body_bytes` | `10485760` | Max client request body size. |

## `auth`

| Field | Default | Meaning |
| --- | --- | --- |
| `disable` | `false` | Disable client-to-gateway auth. Local testing only. |
| `api_keys` | `[]` | Literal gateway API keys. |
| `api_keys_env` | `[]` | Environment variables containing gateway API keys. |

Clients can authenticate with either:

```text
Authorization: Bearer <key>
x-api-key: <key>
```

## `debug`

| Field | Default | Meaning |
| --- | --- | --- |
| `log_messages` | `false` | Log prompt/response bodies and stream events. Sensitive; local debugging only. |

## `health`

Global provider health polling:

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | Enable periodic provider health checks. |
| `interval` | `30s` | Health polling interval. |
| `timeout` | `5s` | Timeout for each provider probe. |
| `failure_threshold` | `2` | Consecutive failures before unhealthy. |
| `success_threshold` | `1` | Consecutive successes before healthy. |

## `routing.sticky_weighted`

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | `true` | Reuse a selected weighted target for the same sticky key. |
| `header` | `X-LLM-Gateway-Sticky-Key` | Header used as explicit sticky key. |
| `fallback` | `auth_key` | `auth_key` or `none`. |
| `ttl` | `24h` | Sticky binding lifetime. |
| `max_entries` | `10000` | Max in-memory sticky bindings. |

## `routing.retry`

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | Enable retry/failover. |
| `max_attempts` | `1` | Total attempts across target providers. |
| `backoff` | `200ms` | Initial backoff between attempts. |
| `max_backoff` | `1s` | Backoff cap. |
| `retry_on_status` | `[408,429,500,502,503,504]` | Provider statuses that can retry. |
| `retry_on_network_error` | `true` | Retry network errors. |
| `retry_on_timeout` | `true` | Retry timeout errors. |

## `model_discovery`

| Field | Default | Meaning |
| --- | --- | --- |
| `ttl` | `10m` | Dynamic model cache TTL. |
| `stale_while_revalidate` | `true` | Keep stale discovered routes while refreshing. |

## `providers[]`

Required fields:

| Field | Meaning |
| --- | --- |
| `name` | Unique provider name. |
| `type` | `anthropic_compatible` or `openai_compatible`. |
| `base_url` | Provider API base URL. |

Common optional fields:

| Field | Meaning |
| --- | --- |
| `api_key` / `api_key_env` | Provider credential. |
| `headers` | Static outbound provider headers. |
| `api_key_header` | Override auth header name. |
| `api_key_scheme` | Override auth scheme, e.g. `Bearer`. |
| `timeout` | Total provider request timeout; default `120s`. |
| `discover_models` | Enable dynamic `/models` discovery. |
| `model_prefix` | Prefix dynamically discovered exposed model IDs. |
| `include_models` / `exclude_models` | Glob filters for discovered models. |

### `providers[].transport`

Outbound HTTP transport pooling:

| Field | Default |
| --- | --- |
| `max_idle_conns` | `1024` |
| `max_idle_conns_per_host` | `256` |
| `idle_conn_timeout` | `90s` |
| `dial_timeout` | `10s` |
| `dial_keep_alive` | `30s` |
| `tls_handshake_timeout` | `10s` |
| `expect_continue_timeout` | `1s` |
| `force_attempt_http2` | `true` |

### `providers[].health`

Per-provider health and probe options:

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | global `health.enabled` | Provider health polling enabled. |
| `probe_path` | `/models` | Provider API-relative probe path. |
| `probe_method` | `GET` | `GET` or `HEAD`. |
| `expected_status` | `[200]` | Status codes considered healthy. |

### `providers[].concurrency`

| Field | Default | Meaning |
| --- | --- | --- |
| `max_in_flight` | `0` | Concurrent `Complete`/`Stream` calls; `0` means unlimited. |

### `providers[].circuit_breaker`

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | Enable circuit breaker. |
| `failure_threshold` | `5` | Consecutive failures before open. |
| `success_threshold` | `1` | Half-open successes before close. |
| `open_timeout` | `30s` | Time before half-open trial. |

## `models`

Legacy single-target alias:

```json
"models": {
  "code-main": {
    "provider": "provider-a",
    "provider_model": "model-large"
  }
}
```

Weighted multi-target alias:

```json
"models": {
  "code-main": {
    "policy": { "type": "weighted" },
    "targets": [
      { "provider": "provider-a", "provider_model": "model-large", "weight": 80 },
      { "provider": "provider-b", "provider_model": "model-balanced", "weight": 20 }
    ]
  }
}
```

Do not mix `provider` / `provider_model` with `targets` in the same model route.
