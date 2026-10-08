"""Instance configuration, read from env at startup."""

from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import urlparse


def _env(name: str, default: str | None = None) -> str:
    val = os.environ.get(name) or default
    if val is None:
        raise RuntimeError(f"Missing required env var: {name}")
    return val


@dataclass(frozen=True)
class Config:
    openai_api_key: str
    openai_base_url: str
    openai_model: str
    company_name: str
    model_provider: str = "openai"
    bedrock_region: str = "us-east-1"
    bedrock_model_id: str = ""
    llm_gateway_url: str = ""
    llm_gateway_api_key: str = ""

    @classmethod
    def from_env(cls) -> "Config":
        provider = _env("MODEL_PROVIDER", "openai")
        if provider not in {"openai", "bedrock-gateway"}:
            raise RuntimeError("MODEL_PROVIDER must be openai or bedrock-gateway")
        gateway_url = _env("LLM_GATEWAY_URL") if provider == "bedrock-gateway" else ""
        if gateway_url:
            parsed = urlparse(gateway_url)
            if (
                parsed.scheme not in {"http", "https"}
                or not parsed.hostname
                or parsed.username
                or parsed.password
                or parsed.query
                or parsed.fragment
            ):
                raise RuntimeError(
                    "LLM_GATEWAY_URL must be an HTTP(S) base URL without credentials, query, or fragment"
                )
            if (
                parsed.scheme != "https"
                and parsed.hostname
                not in {"localhost", "127.0.0.1", "host.docker.internal"}
                and not parsed.hostname.endswith(".localhost")
            ):
                raise RuntimeError("Use HTTPS for a non-local LLM gateway")
        return cls(
            openai_api_key=_env("OPENAI_API_KEY") if provider == "openai" else "",
            openai_base_url=_env("OPENAI_BASE_URL", ""),
            openai_model=_env("OPENAI_MODEL", "gpt-4o-mini"),
            company_name=_env("COMPANY_NAME", "O2 Insurance"),
            model_provider=provider,
            bedrock_region=_env("BEDROCK_REGION", "us-east-1"),
            bedrock_model_id=_env("BEDROCK_MODEL_ID")
            if provider == "bedrock-gateway"
            else "",
            llm_gateway_url=gateway_url.rstrip("/"),
            llm_gateway_api_key=_env("LLM_GATEWAY_API_KEY")
            if provider == "bedrock-gateway"
            else "",
        )
