"""Minimal real tool: a Flask server that the agent hits when no
fault injection is in play. Listening on 0.0.0.0:9090 so the ruptor
proxy running on :8080 can reach it as the passthrough target.
"""

from flask import Flask, jsonify, request

app = Flask(__name__)


@app.get("/search")
def search():
    q = request.args.get("q", "")
    return jsonify({"query": q, "results": ["result1", "result2"]})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=9090)
