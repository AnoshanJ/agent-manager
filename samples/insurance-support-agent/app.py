import os

# Set before importing Strands: without it, tool input/output never reach span attributes.
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")

import logging
import threading
import uuid
from collections import OrderedDict
from typing import Any

from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from strands import Agent

load_dotenv()

from agent import build_agent
from config import Config

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("insurance-support")

CONFIG = Config.from_env()

# Demo-grade conversation store. Sessions are keyed on the caller-supplied
# session_id alone, so anyone who knows an id can resume that conversation —
# a real deployment must key on the authenticated subject instead.
MAX_SESSIONS = 500
SESSIONS: "OrderedDict[str, Agent]" = OrderedDict()
_sessions_lock = threading.Lock()

app = FastAPI(title="Insurance Support Agent", version="1.0.0")

# Off unless set: deployed agents get CORS from the gateway, and a second set of
# headers from here would make browsers reject the response.
_cors_origins = [o for o in os.environ.get("CORS_ALLOW_ORIGINS", "").split(",") if o]
if _cors_origins:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=_cors_origins,
        allow_methods=["POST", "GET", "OPTIONS"],
        allow_headers=["Authorization", "Content-Type"],
    )


class ChatRequest(BaseModel):
    message: str
    session_id: str | None = None
    context: dict[str, Any] | None = None


class ChatResponse(BaseModel):
    response: str
    session_id: str


def _agent_for(session_id: str) -> Agent:
    # FastAPI runs this sync endpoint on a threadpool, so the lookup, the insert
    # and the eviction all have to happen under one lock.
    with _sessions_lock:
        agent = SESSIONS.get(session_id)
        if agent is None:
            agent = build_agent(CONFIG)
            SESSIONS[session_id] = agent
            while len(SESSIONS) > MAX_SESSIONS:
                SESSIONS.popitem(last=False)
        else:
            SESSIONS.move_to_end(session_id)
        return agent


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "company": CONFIG.company_name}


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="message must not be empty")

    session_id = req.session_id or str(uuid.uuid4())

    try:
        result = _agent_for(session_id)(req.message)
    except Exception as exc:
        # Upstream errors can carry gateway URLs and provider payloads; log them,
        # do not return them.
        log.exception("agent invocation failed")
        raise HTTPException(status_code=500, detail="agent invocation failed") from exc

    return ChatResponse(response=str(result), session_id=session_id)
