# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `llm-gateway`, please report it
privately. Do **not** open a public issue for security vulnerabilities.

Preferred reporting options:

1. Use GitHub's private security advisory reporting if available for the
   repository.
2. Otherwise email the maintainer at `zh.feng@gmail.com` with a clear subject
   such as `llm-gateway security report`.

We make a best effort to acknowledge reports within a few business days and to
follow up with an initial assessment shortly after. Response times depend on
maintainer availability.

## What to Include

Please include:

- A detailed description of the vulnerability
- Steps to reproduce, ideally with a minimal proof of concept
- Affected version or commit range
- Relevant config snippets with secrets removed
- Whether the issue affects gateway clients, provider credentials, upstream
  provider requests, logs, routing, health endpoints, or local files
- Any known mitigations

## Disclosure

We follow coordinated disclosure:

- Reports are investigated privately.
- Fixes are prepared before public disclosure when possible.
- Credits are given to the reporter unless anonymity is requested.
- A CVE or GitHub Security Advisory may be requested when warranted.

## Supported Versions

Only the latest mainline version receives security fixes. We recommend always
using the most recent version.

## Security Model

`llm-gateway` is an HTTP gateway that:

- Accepts Anthropic-compatible and OpenAI-compatible client requests
- Sends prompts, tool schemas, messages, and conversation context to configured
  LLM providers
- Holds provider API credentials in memory after reading config/env vars
- Can optionally log prompt and completion bodies when debug logging is enabled
- Can route traffic across multiple providers according to configured policies

## Sensitive Configuration

Do not commit:

- Provider API keys
- Gateway API keys
- `.env` files
- Local provider-specific test configs containing secrets
- Generated binaries or swap files

Provider-specific local test configs can live under `bin/` and remain untracked.

## Operational Guidance

- Keep gateway authentication enabled outside local testing.
- Bind local/dev instances to `127.0.0.1` unless intentionally exposing them.
- Treat `debug.log_messages` as sensitive because it logs prompts and
  completions.
- Review `routing.retry`, provider health checks, and failover settings before
  using with paid providers, because retries can increase provider usage.
- Use least-privilege provider API keys where available.
- Be careful when adding providers or custom headers; outbound headers may carry
  secrets.

## Known High-Impact Areas

Please report privately if you find issues involving:

- Exposure of provider or gateway API keys
- Bypassing gateway authentication
- Logging secrets or prompt data unexpectedly
- SSRF or arbitrary outbound request construction through config/client input
- Cross-tenant sticky routing or provider selection leakage
- Retry/failover behavior that duplicates sensitive actions unexpectedly
- Incorrect handling of tool call results that could confuse clients or tools
