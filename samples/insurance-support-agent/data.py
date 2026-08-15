"""In-memory airline fixtures. Mutated in place by change_seat."""

from __future__ import annotations

FLIGHTS: dict[str, dict] = {
    "O2412": {
        "flight_number": "O2412",
        "route": "London Heathrow (LHR) -> New York JFK",
        "departure": "2026-09-14T09:40:00Z",
        "arrival": "2026-09-14T12:55:00Z",
        "status": "on time",
        "gate": "B12",
        "aircraft": "Airbus A350",
    },
    "O2779": {
        "flight_number": "O2779",
        "route": "New York JFK -> Tokyo Haneda",
        "departure": "2026-09-18T17:10:00Z",
        "arrival": "2026-09-19T21:30:00Z",
        "status": "delayed",
        "delay_minutes": 45,
        "gate": "C7",
        "aircraft": "Boeing 787-9",
    },
    "O2203": {
        "flight_number": "O2203",
        "route": "Dubai (DXB) -> London Heathrow (LHR)",
        "departure": "2026-09-21T02:15:00Z",
        "arrival": "2026-09-21T06:50:00Z",
        "status": "on time",
        "gate": "A3",
        "aircraft": "Airbus A380",
    },
    "O2118": {
        "flight_number": "O2118",
        "route": "London Heathrow (LHR) -> Singapore Changi",
        "departure": "2026-09-25T21:05:00Z",
        "arrival": "2026-09-26T17:20:00Z",
        "status": "boarding",
        "gate": "D9",
        "aircraft": "Boeing 777-300ER",
    },
}

BOOKINGS: dict[str, dict] = {
    "O2K7QT": {
        "reference": "O2K7QT",
        "passenger": "Ada Lovelace",
        "flight_number": "O2412",
        "seat": "14A",
        "cabin": "economy",
        "checked_bags": 1,
        "status": "confirmed",
    },
    "O2M3MN": {
        "reference": "O2M3MN",
        "passenger": "Ada Lovelace",
        "flight_number": "O2779",
        "seat": "2C",
        "cabin": "business",
        "checked_bags": 2,
        "status": "confirmed",
    },
    "O2X9XB": {
        "reference": "O2X9XB",
        "passenger": "Ada Lovelace",
        "flight_number": "O2203",
        "seat": "31F",
        "cabin": "economy",
        "checked_bags": 0,
        "status": "checked-in",
    },
}

OCCUPIED_SEATS: dict[str, set[str]] = {
    "O2412": {"14A", "14B", "1A", "22C"},
    "O2779": {"2C", "2D", "9A"},
    "O2203": {"31F", "31E", "12B"},
    "O2118": {"7A", "7B"},
}
