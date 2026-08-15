"""Strands agent construction."""

from __future__ import annotations

from strands import Agent
from strands.models.openai import OpenAIModel

from config import Config
from tools import (
    change_seat,
    get_flight_status,
    list_bookings,
    list_flights,
    lookup_booking,
)

SYSTEM_PROMPT = """You are a customer support agent for {airline}.

You can do five things, and nothing else:

1. List the passenger's bookings (list_bookings)
2. Show the flight schedule (list_flights)
3. Look up one booking by its reference (lookup_booking)
4. Check the status, gate and timings of a flight (get_flight_status)
5. Move a booking to a different seat (change_seat)

When asked what you can do, describe those capabilities in your own words and
suggest a concrete next step. You cannot book new flights, cancel bookings,
process refunds or take payments — say so plainly and point to what you can do
instead.

Always use the tools for facts — never guess a flight status, seat, route or
passenger name. When the passenger asks about "my bookings" or "my flights",
call list_bookings rather than asking for a reference; only ask for the
six-character reference when you need one specific booking and cannot tell which
from the conversation. Confirm the seat number back after a successful change.
If a tool returns an error, explain it plainly and offer the next step.

Format lists as short bullets. Keep replies brief and friendly.
"""


def build_agent(cfg: Config) -> Agent:
    client_args: dict[str, str] = {"api_key": cfg.openai_api_key}
    if cfg.openai_base_url:
        client_args["base_url"] = cfg.openai_base_url

    model = OpenAIModel(client_args=client_args, model_id=cfg.openai_model)

    return Agent(
        model=model,
        tools=[
            list_bookings,
            list_flights,
            lookup_booking,
            get_flight_status,
            change_seat,
        ],
        system_prompt=SYSTEM_PROMPT.format(airline=cfg.airline_name),
        callback_handler=None,
    )
