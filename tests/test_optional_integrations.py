"""Isolated installed-helper contracts; no live endpoint or client invocation."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[1]


class OptionalIntegrationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.home = Path(self.temp.name)
        self.bin = self.home / "bin"
        self.bin.mkdir()
        self.env = {"HOME": str(self.home), "PATH": f"{self.bin}:/usr/bin:/bin", "SOVKIT_ENDPOINT_URL": "fixture-endpoint"}
        self.command("python3", f'#!/bin/sh\nif [ "${{2:-}}" = fixture-endpoint ]; then exit 0; fi\nexec "{sys.executable}" "$@"\n')
        for source, name in [("sovkit", "sovkit-doctor"), ("pi-sovereign", "pi-sovereign"), ("opencode-sovereign", "opencode-sovereign")]:
            shutil.copy2(REPO / "bin" / source, self.bin / name)

    def tearDown(self):
        self.temp.cleanup()

    def command(self, name, body):
        path = self.bin / name
        path.write_text(body)
        path.chmod(0o700)

    def doctor(self):
        return subprocess.run([str(self.bin / "sovkit-doctor"), "doctor"], env=self.env, text=True, capture_output=True)

    def pi_profile(self, path=None):
        path = path or self.home / ".pi/profiles/sovereign/agent"
        path.mkdir(parents=True)
        (path / "settings.json").write_text(json.dumps({"defaultProvider": "sovereign-qwen", "defaultModel": "actual/model", "subagents": {"modelScope": {"enforce": True, "strict": True, "allow": ["sovereign-qwen/*"]}}}))
        (path / "models.json").write_text(json.dumps({"providers": {"sovereign-qwen": {"baseUrl": "http://127.0.0.1:30000/v1", "models": [{"id": "actual/model"}]}}}))
        (path / "npm/node_modules/pi-subagents").mkdir(parents=True)
        extension = path / "npm/node_modules/oh-my-pi/dist/extension.js"
        extension.parent.mkdir(parents=True)
        extension.write_text("fixture")
        self.command("pi", '#!/bin/sh\nprintf "%s" "$PI_CODING_AGENT_DIR"\n')
        return path

    def opencode_profile(self):
        path = self.home / ".config/opencode/sovereign.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps({"model": "sovereign-qwen/actual/model", "enabled_providers": ["sovereign-qwen"], "provider": {"sovereign-qwen": {"options": {"baseURL": "http://127.0.0.1:30000/v1"}, "models": {"actual/model": {}}}}}))
        self.command("opencode", "#!/bin/sh\necho fixture-version\n")

    def test_generic_only_endpoint_is_healthy_without_optional_clients(self):
        result = self.doctor()
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertIn("Pi integration not configured", result.stdout)
        self.assertIn("OpenCode integration not configured", result.stdout)
        self.assertNotIn("FAIL", result.stdout)

    def test_pi_only_does_not_require_opencode(self):
        self.pi_profile()
        result = self.doctor()
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertIn("PASS  Pi provider lock", result.stdout)
        self.assertIn("OpenCode integration not configured", result.stdout)

    def test_opencode_only_does_not_require_pi(self):
        self.opencode_profile()
        result = self.doctor()
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertIn("PASS  OpenCode provider lock", result.stdout)

    def test_malformed_existing_profile_still_fails(self):
        path = self.pi_profile()
        (path / "settings.json").write_text("invalid JSON")
        result = self.doctor()
        self.assertEqual(1, result.returncode)
        self.assertIn("FAIL  Pi provider lock", result.stdout)

    def test_pi_wrapper_and_doctor_use_sovereign_override_first(self):
        selected = self.pi_profile(self.home / "chosen profile")
        self.env.update(PI_SOVEREIGN_DIR=str(selected), PI_CODING_AGENT_DIR=str(self.home / "other profile"))
        result = subprocess.run([str(self.bin / "pi-sovereign")], env=self.env, text=True, capture_output=True)
        self.assertEqual(str(selected), result.stdout)
        result = self.doctor()
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.assertIn(f"Profile: {selected}\n", result.stdout)

    def test_missing_opencode_profile_points_to_connected_setup(self):
        result = subprocess.run([str(self.bin / "opencode-sovereign")], env=self.env, text=True, capture_output=True)
        self.assertEqual(66, result.returncode)
        self.assertIn("sovkit start", result.stderr)
        self.assertNotIn("install-macos.sh", result.stderr)
