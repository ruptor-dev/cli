"""Ollama-backed agent that exposes a /chat endpoint on :3000 and
wires up a single `search` tool. When the LLM decides to call the
tool, the agent issues an HTTP GET against TOOL_BASE_URL/search, so
pointing TOOL_BASE_URL at the ruptor proxy is all it takes to subject
the tool call to fault injection.

Configuration:
  OLLAMA_URL    Ollama API root (default http://localhost:11434)
  OLLAMA_MODEL  model to use    (default qwen2)
  TOOL_BASE_URL tool endpoint   (default http://localhost:9090)
"""

from __future__ import annotations

import json
import os

import requests
from flask import Flask, jsonify, request

OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://localhost:11434")
OLLAMA_MODEL = os.environ.get("OLLAMA_MODEL", "qwen2")
TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "http://localhost:9090")

app = Flask(__name__)

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "search",
            "description": "Search the knowledge base.",
            "parameters": {
                "type": "object",
                "properties": {"q": {"type": "string"}},
                "required": ["q"],
            },
        },
    }
]


def call_tool(name: str, args: dict) -> str:
    if name != "search":
        return json.dumps({"error": f"unknown tool {name}"})
    try:
        resp = requests.get(
            f"{TOOL_BASE_URL}/search",
            params={"q": args.get("q", "")},
            timeout=10,
        )
        return resp.text
    except requests.RequestException as e:
        return json.dumps({"error": str(e)})


def chat(messages: list[dict]) -> dict:
    r = requests.post(
        f"{OLLAMA_URL}/api/chat",
        json={"model": OLLAMA_MODEL, "messages": messages, "tools": TOOLS, "stream": False},
        timeout=120,
    )
    r.raise_for_status()
    return r.json()


@app.post("/chat")
def chat_endpoint():
    body = request.get_json(force=True) or {}
    user_msg = body.get("message", "")
    messages = [{"role": "user", "content": user_msg}]
    reply = chat(messages)
    msg = reply.get("message", {})

    for call in msg.get("tool_calls", []) or []:
        fn = call.get("function", {})
        tool_output = call_tool(fn.get("name", ""), fn.get("arguments", {}) or {})
        messages.append(msg)
        messages.append({"role": "tool", "content": tool_output})
        reply = chat(messages)
        msg = reply.get("message", {})

    return jsonify({"response": msg.get("content", "")})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=3000)
