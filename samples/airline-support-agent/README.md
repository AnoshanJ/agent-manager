# Airline Support Agent

An end-to-end sample: a Strands agent deployed on WSO2 Agent Manager, plus a
browser UI that logs a passenger in with OAuth2 and chats with the agent through
the gateway.

The agent works on its own — deploy it and chat from **Try It**. The login flow
is an optional second step, and the UI runs with or without it.

## What is in here

| Path                 | Purpose                                                            |
| -------------------- | ------------------------------------------------------------------ |
| `main.py` / `app.py` | FastAPI service exposing `POST /chat`, port 8000 (override `PORT`) |
| `agent.py`           | Strands agent and OpenAI model wiring                              |
| `tools.py`           | Three tools: booking lookup, flight status, seat change            |
| `data.py`            | In-memory bookings and flights — no database to set up             |
| `frontend/`          | The invoking service: a single-page UI that calls the agent        |

Sample data to try: bookings `SKY7QT`, `SKY3MN`, `SKY9XB`; flights `SK412`,
`SK779`, `SK203`.

## Prerequisites

- An OpenAI API key
- Python 3.11+ to run the agent or the UI locally

## Run locally

```bash
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export OPENAI_API_KEY=<your-key>
PORT=10150 python main.py
```

If `python` still resolves outside the venv (pyenv shims are a common cause),
call the interpreter directly: `./.venv/bin/python main.py`.

```bash
curl -X POST http://localhost:10150/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"s1","message":"What is the status of flight SK412?"}'
```

Then run the UI against it — no login, since no provider is configured:

```bash
AGENT_URL=http://localhost:10150/chat python frontend/serve.py
```

Open <http://localhost:13000>.

## Environment variables

### Agent

| Variable             | Required | Default           | Purpose                                                                         |
| -------------------- | -------- | ----------------- | ------------------------------------------------------------------------------- |
| `OPENAI_API_KEY`     | yes      | —                 | Model credential                                                                |
| `PORT`               | no       | `8000`            | Local override only; deployed agents are routed to 8000                         |
| `CORS_ALLOW_ORIGINS` | no       | —                 | Comma-separated origins. Only for local browser use — leave unset when deployed |
| `OPENAI_BASE_URL`    | no       | OpenAI's default  | Any OpenAI-compatible endpoint — an AM LLM provider, a proxy                    |
| `OPENAI_MODEL`       | no       | `gpt-4o-mini`     | Model id                                                                        |
| `AIRLINE_NAME`       | no       | `Skyline Airways` | Used in the system prompt and the UI header                                     |

Set `OPENAI_BASE_URL` when routing model calls through a gateway; the key you
supply in `OPENAI_API_KEY` is then the gateway's key.

### Frontend (`frontend/serve.py`)

| Variable         | Required | Default                       | Purpose                                           |
| ---------------- | -------- | ----------------------------- | ------------------------------------------------- |
| `AGENT_URL`      | yes      | `http://localhost:10150/chat` | Full chat URL — local or the deployed gateway URL |
| `OIDC_ISSUER`    | no       | —                             | Issuer base URL; discovery is appended            |
| `OIDC_CLIENT_ID` | no       | —                             | **Unset means no login** — the UI chats directly  |
| `OIDC_SCOPES`    | no       | `openid profile email`        | Requested scopes                                  |
| `UI_PORT`        | no       | `13000`                       | UI port                                           |

## 1. Deploy in Agent Manager

### Step 1: Create the agent

1. Navigate to the **Default** project
2. Select the **Platform-Hosted Agent** card
3. Pick **Source Code** as the source type

### Step 2: Configure agent details

| Field                 | Value                                              |
| --------------------- | -------------------------------------------------- |
| **Display Name**      | `Airline Support Agent`                            |
| **Description**       | `Passenger support agent for bookings and flights` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`            |
| **Branch**            | `main`                                             |
| **App Path**          | `/samples/airline-support-agent`                   |
| **Language**          | `Python`                                           |
| **Language Version**  | `3.11`                                             |
| **Start Command**     | `python main.py`                                   |

### Step 3: Select the agent interface

Choose **Chat Agent**.

### Step 4: Environment variables

```env
OPENAI_API_KEY=<your-openai-api-key>
```

### Step 5: Deploy

Review and click **Deploy**. The build takes roughly 6-10 minutes.

## 2. Invoke the agent

Use **Try It** in the left navigation, or point the UI at the deployed agent:

```bash
AGENT_URL=https://<your-agent-url>/chat python frontend/serve.py
```

## 3. Optional: secure the agent with OAuth2

The gateway validates the caller's JWT; the agent itself does no auth work. The
UI performs an OpenID Connect authorization-code flow with PKCE as a public
client, then sends the access token as `Authorization: Bearer`.

### With the bundled identity provider (Thunder)

Every installation ships ThunderID, already registered with the gateway as
`ThunderKeyManager`, so there is no identity provider to add.

1. **Create an OAuth application in Thunder.** It must be a public client with
   PKCE — grant type `authorization_code`, token endpoint auth method `none`,
   and callback URL `http://localhost:13000/`. Note the client ID and the Thunder
   issuer URL.
2. **Enable OAuth on the agent.** Open the agent, click **Deploy** →
   **Configure & Deploy**, and under **Endpoint Authentication** select
   **OAuth**, then choose `ThunderKeyManager`.
3. **Run the UI with the provider configured:**

   ```bash
   export AGENT_URL=https://<your-agent-url>/chat
   export OIDC_ISSUER=<thunder-issuer-url>
   export OIDC_CLIENT_ID=<client-id-from-step-1>
   python frontend/serve.py
   ```

Open <http://localhost:13000> and sign in. The UI greets you using the `given_name`
claim from the ID token, then chats through the gateway.

To confirm the gateway is enforcing the token, call the agent without one:

```bash
curl -i -X POST https://<your-agent-url>/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"s1","message":"hello"}'
```

This should return `401`.

### With another provider (Asgardeo, Okta, ...)

The UI is provider-agnostic — it reads the issuer's
`.well-known/openid-configuration` and uses whatever endpoints it advertises.

1. Create the equivalent application with your provider — a **single-page
   application** / public client with PKCE and callback `http://localhost:13000/`.
2. Register the provider once in the console under **Gateways → Identity
   Providers**: paste the issuer URL and let discovery populate the issuer and
   JWKS URI.
3. Select that provider in step 2 above instead of `ThunderKeyManager`, and set
   `OIDC_ISSUER` and `OIDC_CLIENT_ID` to its values.

Nothing else changes.

## Observability

Leave **auto instrumentation** enabled (it is on by default). The platform's
instrumentation installs a global OpenTelemetry tracer provider, and the Strands
agent emits its agent, event-loop and tool spans into it — so this sample needs
no tracing code and does not install `strands-agents[otel]`.

Two details matter if you adapt this sample:

- `app.py` sets `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`
  before importing Strands. Without it, tool input and output are not recorded
  as span attributes and the trace view shows tool spans without their data.
- `requirements.txt` pins `wrapt==1.17.3`. Strands otherwise resolves wrapt 2.x,
  which removed an argument that the platform's auto-instrumentation still uses.

Disabling auto instrumentation removes the OpenTelemetry SDK from the image
entirely, so the agent would then need to install and configure its own
exporter.
