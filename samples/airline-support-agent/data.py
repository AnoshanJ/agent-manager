"""In-memory airline fixtures. Mutated in place by change_seat."""

from __future__ import annotations

FLIGHTS: dict[str, dict] = {
    "SK412": {
        "flight_number": "SK412",
        "route": "London Heathrow (LHR) -> New York JFK",
        "departure": "2026-09-14T09:40:00Z",
        "arrival": "2026-09-14T12:55:00Z",
        "status": "on time",
        "gate": "B12",
        "aircraft": "Airbus A350",
    },
    "SK779": {
        "flight_number": "SK779",
        "route": "New York JFK -> Tokyo Haneda",
        "departure": "2026-09-18T17:10:00Z",
        "arrival": "2026-09-19T21:30:00Z",
        "status": "delayed",
        "delay_minutes": 45,
        "gate": "C7",
        "aircraft": "Boeing 787-9",
    },
    "SK203": {
        "flight_number": "SK203",
        "route": "Dubai (DXB) -> London Heathrow (LHR)",
        "departure": "2026-09-21T02:15:00Z",
        "arrival": "2026-09-21T06:50:00Z",
        "status": "on time",
        "gate": "A3",
        "aircraft": "Airbus A380",
    },
}

BOOKINGS: dict[str, dict] = {
    "SKY7QT": {
        "reference": "SKY7QT",
        "passenger": "Ada Lovelace",
        "flight_number": "SK412",
        "seat": "14A",
        "cabin": "economy",
        "checked_bags": 1,
        "status": "confirmed",
    },
    "SKY3MN": {
        "reference": "SKY3MN",
        "passenger": "Grace Hopper",
        "flight_number": "SK779",
        "seat": "2C",
        "cabin": "business",
        "checked_bags": 2,
        "status": "confirmed",
    },
    "SKY9XB": {
        "reference": "SKY9XB",
        "passenger": "Alan Turing",
        "flight_number": "SK203",
        "seat": "31F",
        "cabin": "economy",
        "checked_bags": 0,
        "status": "checked-in",
    },
}

OCCUPIED_SEATS: dict[str, set[str]] = {
    "SK412": {"14A", "14B", "1A", "22C"},
    "SK779": {"2C", "2D", "9A"},
    "SK203": {"31F", "31E", "12B"},
}
