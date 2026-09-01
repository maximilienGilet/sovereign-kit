#!/usr/bin/env python3
from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading
import unittest


REPO = Path(__file__).resolve().parents[1]
EXAMPLE = REPO / "examples/internal-text-assistant/assistant.py"


class Handler(BaseHTTPRequestHandler):
    request_body: dict | None = None

    def do_POST(self) -> None:
        length = int(self.headers["Content-Length"])
        type(self).request_body = json.loads(self.rfile.read(length))
        body = json.dumps({"choices": [{"message": {"content": "Approved answer."}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_: object) -> None:
        pass


class InternalAssistantExampleTests(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()

    def tearDown(self) -> None:
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()

    def test_sends_only_the_supplied_text_and_prints_the_answer(self) -> None:
        endpoint = f"http://127.0.0.1:{self.server.server_port}/v1"
        result = subprocess.run(
            [str(EXAMPLE), "--endpoint", endpoint, "--text", "Approved source text."],
            text=True,
            capture_output=True,
            check=False,
        )

        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("Approved answer.\n", result.stdout)
        self.assertEqual("qwen3.8-27b-nvfp4", Handler.request_body["model"])
        self.assertEqual("Approved source text.", Handler.request_body["messages"][0]["content"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
