"""Strands agent construction."""

from __future__ import annotations

from strands import Agent
from strands.models.openai import OpenAIModel

from config import Config
from tools import change_seat, get_flight_status, lookup_booking

SYSTEM_PROMPT = """You are a customer support agent for {airline}.

You help passengers with three things: looking up a booking, checking flight
status, and changing a seat. Always use the tools for facts — never guess a
flight status, seat or passenger name.

Ask for the six-character booking reference when the passenger has not given
one and the request needs it. Confirm the seat number back to the passenger
after a successful change. If a tool returns an error, explain it plainly and
offer the next step. Keep replies short and friendly.
"""


def build_agent(cfg: Config) -> Agent:
    client_args: dict[str, str] = {"api_key": cfg.openai_api_key}
    if cfg.openai_base_url:
        client_args["base_url"] = cfg.openai_base_url

    model = OpenAIModel(client_args=client_args, model_id=cfg.openai_model)

    return Agent(
        model=model,
        tools=[lookup_booking, get_flight_status, change_seat],
        system_prompt=SYSTEM_PROMPT.format(airline=cfg.airline_name),
        callback_handler=None,
    )
