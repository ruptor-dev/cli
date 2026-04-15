"""Toy agent for the quickstart. Issues a single search against the
tool URL configured via TOOL_BASE_URL (or the real server on :9090 when
that env var is unset). Designed to surface whatever response shape the
ruptor proxy returned, including injected faults."""

import os
import sys
import json

import requests

TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "http://localhost:9090")


def run(query: str) -> None:
    try:
        resp = requests.get(f"{TOOL_BASE_URL}/search", params={"q": query}, timeout=35)
    except requests.Timeout:
        print(f"agent: tool call timed out against {TOOL_BASE_URL}")
        return
    except requests.ConnectionError as e:
        print(f"agent: tool unreachable at {TOOL_BASE_URL}: {e}")
        return

    print(f"agent: {resp.status_code} {resp.reason}")
    try:
        print(json.dumps(resp.json(), indent=2))
    except ValueError:
        print(f"agent: non-JSON response: {resp.text[:200]!r}")


if __name__ == "__main__":
    query = sys.argv[1] if len(sys.argv) > 1 else "hello"
    run(query)
