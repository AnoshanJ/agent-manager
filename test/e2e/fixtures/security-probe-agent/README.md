# AMP Security Probe Agent

This is a deterministic E2E security fixture, not a production agent or a
general-purpose network utility. It uses no LLM and accepts no arbitrary URL,
command, token, or credential input.

The probe exposes only fixed operations used by `security/runtime`:

- report runtime-hardening booleans;
- attempt the named in-cluster Kubernetes API network path and report only a
  controlled evidence category;
- mint an AgentID token and return its non-secret requested scopes, plus
  best-effort granted-scope diagnostics when the token is a JWT; and
- invoke the fixed `echo` and `add` MCP tools with a fresh AgentID token.

No endpoint returns access tokens, client credentials, environment-variable
values, remote response bodies, or exception messages.

Run the probe's unit tests with:

```bash
python -m unittest discover -s tests
```

Run locally with:

```bash
python -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
python main.py
```
