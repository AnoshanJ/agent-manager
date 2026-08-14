"""Tools exposed to the agent. Strands derives each schema from the signature
and the Args section of the docstring."""

from __future__ import annotations

import json

from strands import tool

from data import BOOKINGS, FLIGHTS, OCCUPIED_SEATS


@tool
def lookup_booking(reference: str) -> str:
    """Look up a passenger booking by its reference.

    Args:
        reference: Six-character booking reference, e.g. SKY7QT.
    """
    booking = BOOKINGS.get(reference.strip().upper())
    if booking is None:
        return json.dumps({"error": f"No booking found for reference {reference}."})
    return json.dumps(booking)


@tool
def get_flight_status(flight_number: str) -> str:
    """Get the current status, gate and timings of a flight.

    Args:
        flight_number: Flight number, e.g. SK412.
    """
    flight = FLIGHTS.get(flight_number.strip().upper())
    if flight is None:
        return json.dumps({"error": f"No flight found with number {flight_number}."})
    return json.dumps(flight)


@tool
def change_seat(reference: str, seat: str) -> str:
    """Move a booking to a different seat.

    Args:
        reference: Six-character booking reference, e.g. SKY7QT.
        seat: Requested seat, e.g. 12C.
    """
    ref = reference.strip().upper()
    requested = seat.strip().upper()

    booking = BOOKINGS.get(ref)
    if booking is None:
        return json.dumps({"error": f"No booking found for reference {reference}."})

    taken = OCCUPIED_SEATS.setdefault(booking["flight_number"], set())
    if requested in taken and requested != booking["seat"]:
        return json.dumps({"error": f"Seat {requested} is already taken."})

    taken.discard(booking["seat"])
    taken.add(requested)
    booking["seat"] = requested
    return json.dumps({"reference": ref, "seat": requested, "status": "seat updated"})
