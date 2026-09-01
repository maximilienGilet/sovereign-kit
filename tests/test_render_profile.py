#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path
import subprocess
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[1]
RENDERER = REPO / "scripts/render-profile.py"
PROFILE = REPO / "profiles/studio-qwen-pro6000/profile.json"


class RenderProfileTests(unittest.TestCase):
    def test_renders_the_studio_route_into_both_client_adapters(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp) / "output"
            result = subprocess.run(
                ["python3", str(RENDERER), "--profile", str(PROFILE), "--output", str(output)],
                text=True, capture_output=True, check=False,
            )
            self.assertEqual(0, result.returncode, result.stderr)
            pi_models = json.loads((output / "pi/models.json").read_text())
            opencode = json.loads((output / "opencode/sovereign.json").read_text())
            self.assertEqual("http://127.0.0.1:30000/v1", pi_models["providers"]["sovereign-qwen"]["baseUrl"])
            self.assertEqual("qwen3.8-27b-nvfp4", opencode["provider"]["sovereign-qwen"]["models"].keys().__iter__().__next__())
            self.assertEqual(262144, opencode["provider"]["sovereign-qwen"]["models"]["qwen3.8-27b-nvfp4"]["limit"]["context"])

    def test_rejects_a_profile_the_shared_pi_adapter_cannot_authenticate(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            profile = json.loads(PROFILE.read_text())
            profile["provider"]["authentication"] = {"mode": "environment"}
            path = Path(temp) / "profile.json"
            path.write_text(json.dumps(profile))
            result = subprocess.run(
                ["python3", str(RENDERER), "--profile", str(path), "--output", str(Path(temp) / "output")],
                text=True, capture_output=True, check=False,
            )
            self.assertEqual(1, result.returncode)
            self.assertIn("keyless", result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
