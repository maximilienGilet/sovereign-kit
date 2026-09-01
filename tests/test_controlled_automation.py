#!/usr/bin/env python3
from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading
import unittest

REPO = Path(__file__).resolve().parents[1]
EXAMPLE = REPO / "examples/controlled-automation/propose.py"


class Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        body = json.dumps({"choices": [{"message": {"content": json.dumps({
            "action": "draft_reply", "requires_approval": True, "draft": "Proposed reply."
        })}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_: object) -> None:
        pass


class ControlledAutomationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()

    def tearDown(self) -> None:
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()

    def test_prints_a_validated_proposal_and_never_executes_it(self) -> None:
        result = subprocess.run(
            [str(EXAMPLE), "--endpoint", f"http://127.0.0.1:{self.server.server_port}/v1",
             "--allowed-action", "draft_reply", "--input", "Customer asks for a status update."],
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(0, result.returncode, result.stderr)
        proposal = json.loads(result.stdout)
        self.assertEqual("draft_reply", proposal["action"])
        self.assertTrue(proposal["requires_approval"])
        self.assertEqual("Proposed reply.", proposal["draft"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
