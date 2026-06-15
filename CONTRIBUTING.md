# Contributing to llm-gateway

Thanks for your interest in contributing! This guide will help you get started.

## Quick Start

```bash
git clone <repo-url>
cd llm-gateway
go build -o bin/llm-gateway ./cmd/llm-gateway
./bin/llm-gateway -config config.example.json
```

## Prerequisites

- Go 1.25+
- At least one provider API key for local end-to-end testing

## Project Structure

```text
cmd/llm-gateway      # CLI entrypoint and startup wiring
internal/config      # JSON config schema, defaults, validation
internal/httpapi     # HTTP handlers, auth, parsing, retry/failover, streaming
internal/models      # model registry, dynamic discovery, weighted/sticky routing
internal/provider    # provider adapters, hooks, limits, circuit breakers
internal/health      # provider health manager and readiness state
internal/protocol    # normalized gateway request/response types
internal/stream      # SSE helpers
internal/gwerror     # normalized error responses
docs/                # durable architecture, concepts, operations, reference
```

See [`docs/architecture.md`](docs/architecture.md) for the system overview and
[`AGENTS.md`](AGENTS.md) for the short repository navigation map.

## Development

### Build

```bash
go build -o bin/llm-gateway ./cmd/llm-gateway
```

### Test

```bash
go test ./...
```

For concurrency, routing, health, or provider protection changes, also run:

```bash
go test -race ./internal/provider ./internal/httpapi ./internal/health ./internal/models
```

### Format

```bash
gofmt -w cmd internal
```

## Local Provider Testing

Keep provider-specific local configs and generated binaries untracked under
`bin/` when they should not be committed:

```bash
./bin/llm-gateway -config bin/config.volcengine.example.json
```

Never commit real provider API keys, local `.env` files, swap files, or generated
binaries.

## Documentation Expectations

Update docs in the same change as behavior changes:

- Architecture or package responsibility changes: `docs/architecture.md`
- Routing, sticky, retry, or failover changes: `docs/concepts/routing.md`
- Health, concurrency, circuit breaker changes: `docs/concepts/provider-health.md`
- Config field changes: `docs/reference/configuration.md`
- Local workflow or operations changes: `docs/operations/`

Keep `README.md` concise and put durable explanations in `docs/`.

## How to Contribute

### Report Bugs

Open an issue with:

- Gateway version or commit
- Config shape with secrets removed
- Request endpoint and model alias
- Expected behavior
- Actual response/logs
- Reproduction steps

For security vulnerabilities, follow [`SECURITY.md`](SECURITY.md) instead of
opening a public issue.

### Suggest Features

Open an issue or discussion describing:

- Use case
- Desired config/API shape
- Operational constraints
- Backward compatibility concerns

### Contributor Responsibility

You are responsible for the code you commit, even when you used an AI coding
tool or other automation to help write it. Review changes yourself before
committing: read the diff, check for secrets or local-only files, verify tests,
and make sure the behavior matches the intent.

### Submit Code

1. Fork or branch from `main`.
2. Make focused changes.
3. Add or update tests.
4. Update docs when behavior/config changes.
5. Run `gofmt` and `go test ./...`.
6. Open a PR with a short summary and test plan.

### Commit Messages

Use concise, imperative messages. Conventional prefixes are welcome but not
required:

```text
feat: add provider failover policy
fix: preserve Anthropic stream lifecycle events
docs: document routing configuration
test: cover circuit breaker half-open state
```

## Areas for Contribution

| Area | Description |
| --- | --- |
| Providers | Add provider compatibility improvements and new backends. |
| Routing | Improve policies, failover, sticky behavior, and observability. |
| Health/HA | Improve probes, circuit breakers, concurrency protection, metrics. |
| Protocol | Improve OpenAI/Anthropic conversion edge cases. |
| Docs | Keep architecture, config, and operations docs current. |
| Tests | Increase coverage for streaming, failover, and provider edge cases. |

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
All participants are expected to uphold these standards.

## Security

Found a security vulnerability? Please do **not** open a public issue. See
[`SECURITY.md`](SECURITY.md) for private reporting guidance.
