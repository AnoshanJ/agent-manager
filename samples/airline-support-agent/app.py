import os

# Set before importing Strands: without it, tool input/output never reach span attributes.
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")

import logging
import uuid
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from strands import Agent

from agent import build_agent
from config import Config

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("airline-support")

CONFIG = Config.from_env()
SESSIONS: dict[str, Agent] = {}

app = FastAPI(title="Airline Support Agent", version="1.0.0")


class ChatRequest(BaseModel):
    message: str
    session_id: str | None = None
    context: dict[str, Any] | None = None


class ChatResponse(BaseModel):
    response: str
    session_id: str


def _agent_for(session_id: str) -> Agent:
    if session_id not in SESSIONS:
        SESSIONS[session_id] = build_agent(CONFIG)
    return SESSIONS[session_id]


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "airline": CONFIG.airline_name}


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="message must not be empty")

    session_id = req.session_id or str(uuid.uuid4())

    try:
        result = _agent_for(session_id)(req.message)
    except Exception as exc:
        log.exception("agent invocation failed")
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    return ChatResponse(response=str(result), session_id=session_id)
