"""Conversational HTTP server on :3000 backed by Ollama. Keeps a
per-session message history so the simulator's multi-turn conversations
have continuity. No tool calls here — this example focuses purely on
conversation quality under different user personas.

POST /chat {"message": str, "session_id": str} → {"response": str}

Configuration:
  OLLAMA_URL    (default http://localhost:11434)
  OLLAMA_MODEL  (default qwen2)
  SYSTEM_PROMPT optional system message seed
"""

from __future__ import annotations

import os
import threading

import requests
from flask import Flask, jsonify, request

OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://localhost:11434")
OLLAMA_MODEL = os.environ.get("OLLAMA_MODEL", "qwen2")
SYSTEM_PROMPT = os.environ.get(
    "SYSTEM_PROMPT",
    "You are a concise customer-support agent. Keep answers short and actionable.",
)

app = Flask(__name__)
_sessions: dict[str, list[dict]] = {}
_lock = threading.Lock()


def history_for(session_id: str) -> list[dict]:
    with _lock:
        if session_id not in _sessions:
            _sessions[session_id] = [{"role": "system", "content": SYSTEM_PROMPT}]
        return _sessions[session_id]


@app.post("/chat")
def chat():
    body = request.get_json(force=True) or {}
    session_id = body.get("session_id", "default")
    user_msg = body.get("message", "")
    history = history_for(session_id)

    history.append({"role": "user", "content": user_msg})
    r = requests.post(
        f"{OLLAMA_URL}/api/chat",
        json={"model": OLLAMA_MODEL, "messages": history, "stream": False},
        timeout=120,
    )
    r.raise_for_status()
    msg = r.json().get("message", {})
    assistant = msg.get("content", "")
    history.append({"role": "assistant", "content": assistant})
    return jsonify({"response": assistant, "session_id": session_id})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=3000)
