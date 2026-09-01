#!/usr/bin/env python3
from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest


REPO = Path(__file__).resolve().parents[1]
EXAMPLE = REPO / "examples/read-only-retrieval/answer.py"


class Handler(BaseHTTPRequestHandler):
    request_body: dict | None = None

    def do_POST(self) -> None:
        length = int(self.headers["Content-Length"])
        type(self).request_body = json.loads(self.rfile.read(length))
        body = json.dumps({"choices": [{"message": {"content": "Answer from allowed context."}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_: object) -> None:
        pass


class ReadOnlyRetrievalTests(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()
        self.temp = tempfile.TemporaryDirectory()
        self.documents = Path(self.temp.name) / "documents.json"
        self.documents.write_text(json.dumps([
            {"id": "public-policy", "text": "Travel policy allows rail bookings."},
            {"id": "payroll", "text": "Payroll adjustment is confidential."},
        ]))

    def tearDown(self) -> None:
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()
        self.temp.cleanup()

    def test_never_sends_a_document_outside_the_explicit_allowlist(self) -> None:
        endpoint = f"http://127.0.0.1:{self.server.server_port}/v1"
        result = subprocess.run(
            [str(EXAMPLE), "--endpoint", endpoint, "--documents", str(self.documents),
             "--allow-id", "public-policy", "--question", "What does travel policy allow?"],
            text=True, capture_output=True, check=False,
        )

        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("Answer from allowed context.\n", result.stdout)
        prompt = Handler.request_body["messages"][0]["content"]
        self.assertIn("Travel policy", prompt)
        self.assertNotIn("Payroll adjustment", prompt)


if __name__ == "__main__":
    unittest.main(verbosity=2)
