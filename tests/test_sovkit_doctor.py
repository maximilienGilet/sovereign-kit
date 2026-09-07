#!/usr/bin/env python3
"""Behaviour tests for the public `sovkit doctor` command and installer."""
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[1]
DOCTOR = REPO / "bin" / "sovkit"


class SovkitDoctorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.home = Path(self.temp.name) / "home"
        self.home.mkdir()
        self.bin_dir = Path(self.temp.name) / "bin"
        self.bin_dir.mkdir()
        self._fake_command(
            "pi",
            "#!/usr/bin/env bash\n"
            "if [[ ${1:-} == install ]]; then\n"
            "  package=${2:-}\n"
            "  if [[ $package == *pi-subagents* ]]; then mkdir -p \"$PI_CODING_AGENT_DIR/npm/node_modules/pi-subagents\"; fi\n"
            "  if [[ $package == *oh-my-pi* ]]; then mkdir -p \"$PI_CODING_AGENT_DIR/npm/node_modules/oh-my-pi/dist\"; touch \"$PI_CODING_AGENT_DIR/npm/node_modules/oh-my-pi/dist/extension.js\"; fi\n"
            "fi\n"
            "echo 'pi 0.84.2'\n",
        )
        self._fake_command("opencode", "#!/usr/bin/env bash\necho '1.18.25'\n")
        # Doctor fixtures are explicit existing profiles, not installer side effects.
        profile = self.home / ".pi/profiles/sovereign/agent"
        profile.mkdir(parents=True)
        settings = {"defaultProvider": "sovereign-qwen", "defaultModel": "owner/served-model", "subagents": {"modelScope": {"enforce": True, "strict": True, "allow": ["sovereign-qwen/*"]}}}
        models = {"providers": {"sovereign-qwen": {"baseUrl": "http://127.0.0.1:30000/v1", "models": [{"id": "owner/served-model"}]}}}
        (profile / "settings.json").write_text(json.dumps(settings))
        (profile / "models.json").write_text(json.dumps(models))
        (profile / "npm/node_modules/pi-subagents").mkdir(parents=True)
        extension = profile / "npm/node_modules/oh-my-pi/dist/extension.js"
        extension.parent.mkdir(parents=True)
        extension.write_text("// fixture")
        config = self.home / ".config/opencode/sovereign.json"
        config.parent.mkdir(parents=True)
        config.write_text(json.dumps({"model": "sovereign-qwen/owner/served-model", "enabled_providers": ["sovereign-qwen"], "provider": {"sovereign-qwen": {"options": {"baseURL": "http://127.0.0.1:30000/v1"}, "models": {"owner/served-model": {}}}}}))
        self.existing_settings = (profile / "settings.json").read_bytes()
        result = subprocess.run(
            [str(REPO / "install-macos.sh")],
            text=True,
            capture_output=True,
            env=os.environ | {"HOME": str(self.home), "PATH": f"{self.bin_dir}:{os.environ['PATH']}"},
            check=False,
        )
        self.assertEqual(0, result.returncode, result.stderr)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def _fake_command(self, name: str, content: str) -> None:
        command = self.bin_dir / name
        command.write_text(content)
        command.chmod(0o755)

    @property
    def sovkit(self) -> Path:
        return self.home / ".local/bin/sovkit"

    def run_doctor(self) -> subprocess.CompletedProcess[str]:
        env = os.environ | {"HOME": str(self.home), "PATH": f"{self.home / '.local/bin'}:{self.bin_dir}:{os.environ['PATH']}"}
        return subprocess.run([str(self.sovkit), "doctor"], text=True, capture_output=True, env=env, check=False)

    def test_reports_local_configuration_and_missing_tunnel(self) -> None:
        result = self.run_doctor()
        self.assertEqual(1, result.returncode, result.stderr)
        self.assertIn("PASS  Pi profile", result.stdout)
        self.assertIn("PASS  Pi provider lock", result.stdout)
        self.assertIn("PASS  Pi extensions", result.stdout)
        self.assertIn("PASS  OpenCode provider lock", result.stdout)
        self.assertIn("FAIL  Local endpoint", result.stdout)
        self.assertIn("sovkit-tunnel", result.stdout)

    def test_installer_preserves_an_existing_fail_closed_route(self) -> None:
        profile = self.home / ".pi/profiles/sovereign/agent"
        settings = json.loads((profile / "settings.json").read_text())
        self.assertEqual(self.existing_settings, (profile / "settings.json").read_bytes())
        models = json.loads((profile / "models.json").read_text())
        opencode = json.loads((self.home / ".config/opencode/sovereign.json").read_text())
        self.assertEqual("sovereign-qwen", settings["defaultProvider"])
        self.assertEqual(["sovereign-qwen/*"], settings["subagents"]["modelScope"]["allow"])
        self.assertEqual(["sovereign-qwen"], list(models["providers"]))
        self.assertEqual("http://127.0.0.1:30000/v1", models["providers"]["sovereign-qwen"]["baseUrl"])
        self.assertEqual(["sovereign-qwen"], opencode["enabled_providers"])
        self.assertEqual("sovereign-qwen/owner/served-model", opencode["model"])

    def test_installer_installs_the_go_cli_and_doctor_helper(self) -> None:
        self.assertTrue(self.sovkit.is_file())
        self.assertTrue(os.access(self.sovkit, os.X_OK))
        helper = self.home / ".local/bin/sovkit-doctor"
        self.assertTrue(helper.is_file())
        self.assertTrue(os.access(helper, os.X_OK))
        result = subprocess.run(
            [str(self.sovkit), "help"],
            text=True,
            capture_output=True,
            env=os.environ | {"HOME": str(self.home), "PATH": f"{self.home / '.local/bin'}:{self.bin_dir}:{os.environ['PATH']}"},
            check=False,
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("start", result.stdout)
        self.assertIn("tunnel", result.stdout)
        self.assertIn("doctor", result.stdout)
        self.assertNotIn("dashboard", result.stdout)

    def test_server_recipe_uses_a_digest_locked_image(self) -> None:
        image = (REPO / "server/image.lock").read_text().strip()
        launch = (REPO / "server/run-sglang.sh").read_text()
        self.assertRegex(image, r"^lmsysorg/sglang@sha256:[0-9a-f]{64}$")
        self.assertIn('image="$(<"$repo_dir/server/image.lock")"', launch)
        self.assertIn('--host 127.0.0.1', launch)

    def test_fails_when_the_pi_provider_allowlist_is_relaxed(self) -> None:
        settings_path = self.home / ".pi/profiles/sovereign/agent/settings.json"
        settings = json.loads(settings_path.read_text())
        settings["subagents"]["modelScope"]["allow"] = ["*"]
        settings_path.write_text(json.dumps(settings))
        result = self.run_doctor()
        self.assertEqual(1, result.returncode)
        self.assertIn("FAIL  Pi provider lock", result.stdout)


if __name__ == "__main__":
    unittest.main(verbosity=2)
