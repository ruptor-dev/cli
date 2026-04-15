"""Scripted agent that exercises the three mock tools in sequence:
search → db lookup → notify. Prints what happens at each step so
ruptor's observations map captures hits and errors against each tool
independently. Uses short per-call timeouts so injected faults surface
as real client-side failures rather than hanging the run.
"""

import json
import os

import requests

TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "http://localhost:9090")


def call(method: str, path: str, **kwargs) -> None:
    url = f"{TOOL_BASE_URL}{path}"
    try:
        resp = requests.request(method, url, timeout=5, **kwargs)
    except requests.RequestException as e:
        print(f"[{method} {path}] error: {e}")
        return
    print(f"[{method} {path}] {resp.status_code}")
    try:
        print(json.dumps(resp.json(), indent=2))
    except ValueError:
        print(f"  non-JSON body: {resp.text[:200]!r}")


if __name__ == "__main__":
    call("GET", "/search", params={"q": "hello"})
    call("GET", "/db/user", params={"id": "42"})
    call("POST", "/notify", json={"to": "user_42", "message": "hi"})
