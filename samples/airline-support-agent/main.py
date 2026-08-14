"""Entrypoint for the airline support agent. AM run command: ``python main.py``."""

from __future__ import annotations

import uvicorn

from app import app


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=8000)


if __name__ == "__main__":
    main()
