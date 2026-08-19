"""Tests for redacted AgentID token evidence."""

import base64
import json
import unittest

from probe.identity import TokenResult, _jwt_scopes, _normalized_scopes


def _jwt(payload: dict[str, object]) -> str:
    encoded = base64.urlsafe_b64encode(json.dumps(payload).encode()).decode()
    return f"header.{encoded.rstrip('=')}.signature"


class IdentityEvidenceTests(unittest.TestCase):
    def test_requested_scopes_are_normalized(self) -> None:
        self.assertEqual(
            _normalized_scopes("proxy:write proxy:read proxy:write"),
            ["proxy:read", "proxy:write"],
        )

    def test_jwt_scope_claim_is_available_as_diagnostic_evidence(self) -> None:
        self.assertEqual(
            _jwt_scopes(_jwt({"scope": "proxy:write proxy:read"})),
            ["proxy:read", "proxy:write"],
        )

    def test_opaque_token_does_not_invent_granted_scopes(self) -> None:
        self.assertEqual(_jwt_scopes("opaque-access-token"), [])

    def test_public_result_never_contains_access_token(self) -> None:
        result = TokenResult(
            configured=True,
            token_minted=True,
            status_code=200,
            requested_scopes=["proxy:read"],
            granted_scopes=[],
            oauth_error="",
            access_token="secret-token-value",
        ).public()

        self.assertNotIn("access_token", result)
        self.assertNotIn("secret-token-value", str(result))


if __name__ == "__main__":
    unittest.main()
