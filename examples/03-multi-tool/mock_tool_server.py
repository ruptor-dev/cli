"""Three-endpoint mock service for multi-tool chaos.
  GET  /search?q=     → {"results": [...]}
  GET  /db/user?id=   → {"user": {...}}
  POST /notify        → {"sent": true}
"""

from flask import Flask, jsonify, request

app = Flask(__name__)


@app.get("/search")
def search():
    return jsonify({"query": request.args.get("q", ""), "results": ["r1", "r2", "r3"]})


@app.get("/db/user")
def db_user():
    uid = request.args.get("id", "0")
    return jsonify({"user": {"id": uid, "name": f"user_{uid}", "plan": "pro"}})


@app.post("/notify")
def notify():
    return jsonify({"sent": True})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=9090)
