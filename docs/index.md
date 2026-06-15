# Documentation Index

This directory is the durable knowledge base for `llm-gateway`. Keep `README.md` concise for product overview and quick usage; put long-lived architecture, configuration, and operations details here.

## Primary Entrypoints

- `../README.md` — overview, quick start, and common examples.
- `architecture.md` — system-level architecture and request flow.
- `reference/configuration.md` — full configuration reference.
- `concepts/routing.md` — model aliases, weighted routing, sticky routing, retry, and failover.
- `concepts/provider-health.md` — provider health checks, readiness, circuit breakers, and concurrency limits.
- `operations/development.md` — local build/test workflow.
- `operations/troubleshooting.md` — common operational issues.

## By Reader Goal

### Understand the system

- `architecture.md` — components, package responsibilities, request lifecycle.
- `concepts/routing.md` — how client model aliases map to provider targets.
- `concepts/provider-health.md` — how the gateway protects itself and routes around provider issues.

### Configure the gateway

- `reference/configuration.md` — config file structure and defaults.
- `../config.example.json` — generic example config.

### Operate locally

- `operations/development.md` — build, test, and run commands.
- `operations/troubleshooting.md` — debugging Claude Code, provider auth, `/readyz`, and routing failures.

## Update Policy

When code changes alter request flow, provider behavior, routing semantics, health handling, or config fields, update the relevant page here in the same change.
