# llm-gateway Agent Guide

This file is the short navigation map for agents and contributors. Keep durable knowledge in `docs/`; keep this file focused on where to look and what rules to follow before changing code.

`AGENTS.md` is a static navigation aid for whoever opens the repository. If project-specific runtime prompt files are added later, keep runtime context there and durable documentation in `docs/`.

## Start Here

- Product overview: `README.md`
- Documentation index: `docs/index.md`
- Detailed architecture: `docs/architecture.md`
- Routing model: `docs/concepts/routing.md`
- Provider health and protection: `docs/concepts/provider-health.md`
- Configuration reference: `docs/reference/configuration.md`
- Development workflow: `docs/operations/development.md`
- Troubleshooting: `docs/operations/troubleshooting.md`

## Repository Shape

- `cmd/llm-gateway`: CLI entrypoint, config loading, provider construction, registry/health/server startup.
- `internal/config`: JSON config schema, defaults, validation, and runtime config values.
- `internal/httpapi`: HTTP handlers, gateway auth, request parsing, retry/failover, streaming response encoding.
- `internal/models`: model registry, static/dynamic routes, weighted and sticky target selection.
- `internal/provider`: provider interface, Anthropic/OpenAI-compatible adapters, transport setup, hooks, limits, circuit breakers.
- `internal/health`: provider health manager, periodic probes, readiness state.
- `internal/protocol`: normalized request/response/message/tool/stream types shared between handlers and providers.
- `internal/stream`: SSE parser/writer helpers.
- `internal/gwerror`: gateway error type and Anthropic/OpenAI error response writers.
- `docs`: durable architecture, concepts, operations, and references.
- `bin`: local build/test artifacts; keep provider-specific local test configs here if they should not be committed.

## Rules

Before editing routing, provider, health, or config behavior, read:

- `docs/architecture.md` — package responsibilities and request flow.
- `docs/concepts/routing.md` — model alias, weighted, sticky, retry, and failover semantics.
- `docs/concepts/provider-health.md` — health checks, `/readyz`, concurrency limits, and circuit breaker behavior.
- `docs/reference/configuration.md` — config field defaults and validation expectations.

Update those files when behavior or config changes. Do not duplicate full reference material here.

Do not add AI co-author or generated-by trailers to commit messages. This includes `Co-Authored-By` or similar attribution for Claude, Codex, or any other AI coding tool.

## Common Commands

See `docs/operations/development.md` for build and test commands. Most changes should pass:

```bash
go test ./...
```

For concurrency, routing, or health changes, also run the relevant race tests described in `docs/operations/development.md`.

## Documentation Rules

- Add or update docs in the same change as architecture, config, routing, provider, or operations changes.
- Keep `README.md` concise; put detailed explanations in `docs/`.
- Configuration changes belong in `docs/reference/configuration.md` and examples when appropriate.
- Routing behavior changes belong in `docs/concepts/routing.md`.
- Health, retry, circuit breaker, or concurrency behavior changes belong in `docs/concepts/provider-health.md`.
- Operational troubleshooting belongs in `docs/operations/troubleshooting.md`.

## Local Files

Do not commit local provider credentials, generated binaries, swap files, or local test configs. Provider-specific test configs can live under `bin/` and remain untracked.
