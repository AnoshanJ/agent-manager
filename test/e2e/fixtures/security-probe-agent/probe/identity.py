"""AgentID token minting with strictly redacted public results."""

from __future__ import annotations

import base64
import json
import os
from dataclasses import dataclass

import httpx


SAFE_OAUTH_ERRORS = {
    "invalid_client",
    "invalid_request",
    "invalid_scope",
    "server_error",
    "temporarily_unavailable",
    "unauthorized_client",
}


@dataclass(slots=True)
class TokenResult:
    configured: bool
    token_minted: bool
    status_code: int | None
    granted_scopes: list[str]
    oauth_error: str
    access_token: str

    def public(self) -> dict[str, object]:
        """Return only non-credential evidence safe for test output and logs."""

        return {
            "configured": self.configured,
            "token_minted": self.token_minted,
            "status_code": self.status_code,
            "granted_scopes": self.granted_scopes,
            "oauth_error": self.oauth_error,
        }


def _jwt_scopes(token: str) -> list[str]:
    try:
        payload_segment = token.split(".")[1]
        payload_segment += "=" * (-len(payload_segment) % 4)
        payload = json.loads(base64.urlsafe_b64decode(payload_segment))
    except (IndexError, ValueError, json.JSONDecodeError):
        return []

    value = payload.get("scope", payload.get("scp", []))
    if isinstance(value, str):
        return sorted(set(value.split()))
    if isinstance(value, list):
        return sorted({scope for scope in value if isinstance(scope, str)})
    return []


async def mint_agent_token() -> TokenResult:
    client_id = os.environ.get("AMP_AGENTID_CLIENT_ID", "")
    client_secret = os.environ.get("AMP_AGENTID_CLIENT_SECRET", "")
    token_endpoint = os.environ.get("AMP_AGENTID_TOKEN_ENDPOINT", "")
    requested_scopes = os.environ.get("AMP_AGENTID_SCOPES", "")
    configured = bool(client_id and client_secret and token_endpoint)
    if not configured:
        return TokenResult(False, False, None, [], "not_configured", "")

    try:
        async with httpx.AsyncClient(
            timeout=10.0,
            trust_env=False,
        ) as client:
            response = await client.post(
                token_endpoint,
                data={
                    "grant_type": "client_credentials",
                    "scope": requested_scopes,
                },
                auth=httpx.BasicAuth(client_id, client_secret),
            )
    except httpx.HTTPError:
        return TokenResult(True, False, None, [], "request_failed", "")

    oauth_error = ""
    token = ""
    try:
        body = response.json()
    except ValueError:
        body = {}
    if response.status_code == 200 and isinstance(body, dict):
        candidate = body.get("access_token", "")
        if isinstance(candidate, str):
            token = candidate
    elif isinstance(body, dict):
        candidate = body.get("error", "")
        if isinstance(candidate, str):
            oauth_error = (
                candidate if candidate in SAFE_OAUTH_ERRORS else "token_rejected"
            )

    return TokenResult(
        configured=True,
        token_minted=bool(token),
        status_code=response.status_code,
        granted_scopes=_jwt_scopes(token),
        oauth_error=oauth_error,
        access_token=token,
    )
