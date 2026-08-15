"""Serves the demo UI and injects its config from environment variables.

Stdlib only — no npm, no build step. Run it from anywhere:

    AGENT_URL=http://localhost:10150/chat python frontend/serve.py

Leaving OIDC_CLIENT_ID unset serves the UI in no-login mode, so the sample can
be exercised before OAuth is configured on the agent.
"""

from __future__ import annotations

import json
import os
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

HERE = Path(__file__).parent


def build_config() -> dict[str, str]:
    agent_url = os.environ.get("AGENT_URL", "http://localhost:10150/chat")
    return {
        "agentUrl": agent_url,
        "issuer": os.environ.get("OIDC_ISSUER", "").rstrip("/"),
        "clientId": os.environ.get("OIDC_CLIENT_ID", ""),
        "scopes": os.environ.get("OIDC_SCOPES", "openid profile email"),
        "companyName": os.environ.get("COMPANY_NAME", "O2 Insurance"),
    }


class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):  # noqa: N802
        path = self.path.split("?", 1)[0]

        if path == "/config.js":
            body = f"window.APP_CONFIG = {json.dumps(build_config())};".encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/javascript")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(body)
            return

        # "/?code=..." is the OAuth redirect back, so "/" must render the page.
        if path in ("/", ""):
            self.path = "/index.html"
        else:
            self.path = path

        super().do_GET()

    def log_message(self, fmt, *args):
        print(f"  {fmt % args}")


def main() -> None:
    # UI_PORT, not PORT: the agent uses PORT, and both often run from one shell.
    port = int(os.environ.get("UI_PORT", "13000"))
    config = build_config()

    handler = partial(Handler, directory=str(HERE))
    server = ThreadingHTTPServer(("127.0.0.1", port), handler)

    mode = "OAuth login" if config["clientId"] else "no-login (OIDC_CLIENT_ID unset)"
    print(f"Insurance support demo UI on http://localhost:{port}")
    print(f"  mode      : {mode}")
    print(f"  agent     : {config['agentUrl']}")
    if config["clientId"]:
        print(f"  issuer    : {config['issuer']}")
        print(f"  client id : {config['clientId']}")
        print(f"  redirect  : http://localhost:{port}/  (register this with your provider)")
    server.serve_forever()


if __name__ == "__main__":
    main()
