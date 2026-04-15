"""Defensive all-faults agent. Calls /search then /llm/complete
through TOOL_BASE_URL (the ruptor proxy). Handles every failure mode
defensively: timeouts, 5xx, malformed JSON, empty bodies, rate limits.
Always exits 0 so the orchestrator records the experiment cleanly.
"""

from __future__ import annotations

import json
import os
import sys
import time

import requests

TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "http://localhost:9191")
SEARCH_TIMEOUT = 2.5    # shorter than the 3s tool_timeout / llm_timeout delay
LLM_TIMEOUT = 2.5


def log(msg: str) -> None:
    print(f"[agent] {msg}", flush=True)


def call_search(query: str, retry_on_rate_limit: bool = True) -> None:
    url = f"{TOOL_BASE_URL}/search"
    started = time.monotonic()
    try:
        resp = requests.get(url, params={"q": query}, timeout=SEARCH_TIMEOUT)
    except requests.Timeout:
        log(f"search timed out after {SEARCH_TIMEOUT}s — agent gave up cleanly")
        return
    except requests.ConnectionError as e:
        log(f"search connection error: {e}")
        return

    elapsed = time.monotonic() - started

    if resp.status_code == 429:
        wait = int(resp.headers.get("Retry-After", "1") or "1")
        log(f"search rate-limited (429), retry-after={wait}s")
        if retry_on_rate_limit:
            time.sleep(min(wait, 3))
            log("retrying search once after backoff")
            call_search(query, retry_on_rate_limit=False)
        return

    if resp.status_code >= 500:
        log(f"search 5xx ({resp.status_code}) — fallback: returning empty result set")
        return

    if not resp.content:
        log("search returned 200 with empty body — treating as no-results")
        return

    try:
        data = resp.json()
    except ValueError:
        log(f"search returned non-JSON body ({len(resp.content)} bytes) — fallback engaged")
        log(f"  raw: {resp.text[:120]!r}")
        return

    log(f"search ok in {elapsed*1000:.0f}ms: {json.dumps(data)[:160]}")


def call_llm(prompt: str) -> None:
    url = f"{TOOL_BASE_URL}/llm/complete"
    started = time.monotonic()
    try:
        resp = requests.post(url, json={"prompt": prompt}, timeout=LLM_TIMEOUT)
    except requests.Timeout:
        log(f"llm call timed out after {LLM_TIMEOUT}s — falling back to canned answer")
        return
    except requests.ConnectionError as e:
        log(f"llm connection error: {e}")
        return

    elapsed = time.monotonic() - started

    if resp.status_code >= 500:
        log(f"llm 5xx ({resp.status_code}) — falling back to canned answer")
        return

    if not resp.content:
        log("llm returned 200 with empty body — falling back to canned answer")
        return

    try:
        data = resp.json()
    except ValueError:
        log(f"llm returned non-JSON body — falling back to canned answer")
        return

    log(f"llm ok in {elapsed*1000:.0f}ms: {data.get('completion','')[:80]!r}")


if __name__ == "__main__":
    query = sys.argv[1] if len(sys.argv) > 1 else "hello"
    call_search(query)
    call_llm(f"summarize: {query}")
    log("done")
