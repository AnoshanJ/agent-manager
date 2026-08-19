"""Named network probes for destinations that must be sandbox-blocked."""

from __future__ import annotations

import os

import httpx


async def probe_kubernetes_api() -> dict[str, object]:
    """Attempt the in-cluster API without credentials and report reachability only.

    Any HTTP response proves the network path was reachable, even a 401 or 403.
    Only a connect timeout is strong evidence that the NetworkPolicy dropped it.
    Error messages and destination values are deliberately never returned.
    """

    host = os.environ.get("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
    port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
    try:
        timeout = httpx.Timeout(connect=3.0, read=3.0, write=3.0, pool=3.0)
        async with httpx.AsyncClient(
            timeout=timeout,
            verify=False,
            trust_env=False,
        ) as client:
            response = await client.get(f"https://{host}:{port}/version")
        return {
            "target": "kubernetes-api",
            "outcome": "reachable",
            "http_status": response.status_code,
        }
    except (httpx.ConnectTimeout, httpx.PoolTimeout):
        return {
            "target": "kubernetes-api",
            "outcome": "blocked",
            "http_status": None,
        }
    except httpx.HTTPError:
        # DNS errors, connection refusal, resets, and read timeouts do not prove
        # that NetworkPolicy blocked a live target. Fail closed as indeterminate.
        return {
            "target": "kubernetes-api",
            "outcome": "indeterminate",
            "http_status": None,
        }
