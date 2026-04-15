"""Two-endpoint mock service for the all-faults example.
  GET  /search?q=        → {"results": [...]}
  POST /llm/complete     → {"completion": "..."}
Listens on :9191 so it does not collide with the other examples.
"""

from flask import Flask, jsonify, request

app = Flask(__name__)


@app.get("/search")
def search():
    return jsonify({"query": request.args.get("q", ""), "results": ["r1", "r2"]})


@app.post("/llm/complete")
def llm_complete():
    body = request.get_json(silent=True) or {}
    prompt = body.get("prompt", "")
    return jsonify({"completion": f"response to: {prompt[:40]}"})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=9191)
