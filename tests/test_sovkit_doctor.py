#!/usr/bin/env python3
"""Behaviour tests for the public `sovkit doctor` command."""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
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
        rendered = Path(self.temp.name) / "rendered"
        subprocess.run(
            ["python3", str(REPO / "scripts/render-profile.py"), "--profile", str(REPO / "profiles/studio-qwen-pro6000/profile.json"), "--output", str(rendered)],
            check=True,
            capture_output=True,
            text=True,
        )
        profile = self.home / ".pi/profiles/sovereign/agent"
        profile.mkdir(parents=True)
        shutil.copy(rendered / "pi/settings.json", profile / "settings.json")
        shutil.copy(rendered / "pi/models.json", profile / "models.json")
        (profile / "npm/node_modules/pi-subagents").mkdir(parents=True)
        extension = profile / "npm/node_modules/oh-my-pi/dist"
        extension.mkdir(parents=True)
        (extension / "extension.js").touch()
        config = self.home / ".config/opencode"
        config.mkdir(parents=True)
        shutil.copy(rendered / "opencode/sovereign.json", config / "sovereign.json")

    def tearDown(self) -> None:
        self.temp.cleanup()

    def _fake_command(self, name: str, content: str) -> None:
        command = self.bin_dir / name
        command.write_text(content)
        command.chmod(0o755)

    def run_doctor(self) -> subprocess.CompletedProcess[str]:
        env = os.environ | {
            "HOME": str(self.home),
            "PATH": f"{self.bin_dir}:{os.environ['PATH']}",
        }
        return subprocess.run(
            [str(DOCTOR), "doctor"],
            text=True,
            capture_output=True,
            env=env,
            check=False,
        )

    def test_reports_a_healthy_local_install_even_without_a_tunnel(self) -> None:
        result = self.run_doctor()

        self.assertEqual(1, result.returncode, result.stderr)
        self.assertIn("PASS  Pi profile", result.stdout)
        self.assertIn("PASS  Pi provider lock", result.stdout)
        self.assertIn("PASS  Pi extensions", result.stdout)
        self.assertIn("PASS  OpenCode provider lock", result.stdout)
        self.assertIn("FAIL  Local endpoint", result.stdout)
        self.assertIn("sovkit-tunnel", result.stdout)

    def test_installer_installs_the_sovkit_command(self) -> None:
        install_home = Path(self.temp.name) / "install-home"
        install_home.mkdir()
        result = subprocess.run(
            [str(REPO / "install-macos.sh")],
            text=True,
            capture_output=True,
            env=os.environ | {"HOME": str(install_home), "PATH": f"{self.bin_dir}:{os.environ['PATH']}"},
            check=False,
        )

        self.assertEqual(0, result.returncode, result.stderr)
        command = install_home / ".local/bin/sovkit"
        self.assertTrue(command.is_file())
        self.assertTrue(os.access(command, os.X_OK))

    def test_studio_profile_has_a_fail_closed_route_and_evidence(self) -> None:
        profile = json.loads((REPO / "profiles/studio-qwen-pro6000/profile.json").read_text())
        self.assertEqual("studio-qwen-pro6000", profile["id"])
        self.assertEqual("ssh-loopback", profile["route"]["transport"])
        self.assertEqual("deny", profile["route"]["fallback"])
        self.assertEqual("none", profile["provider"]["authentication"]["mode"])
        self.assertEqual(262144, profile["limits"]["contextWindow"])
        self.assertEqual(5, profile["limits"]["maxConcurrentRequests"])
        self.assertTrue(profile["evidence"]["benchmarks"])

    def test_profile_checker_accepts_the_published_studio_profile(self) -> None:
        result = subprocess.run(
            ["python3", "scripts/check-profiles.py"],
            cwd=REPO,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("Profile validation passed", result.stdout)

    def test_server_recipe_uses_a_digest_locked_image(self) -> None:
        image = (REPO / "server/image.lock").read_text().strip()
        self.assertRegex(image, r"^lmsysorg/sglang@sha256:[0-9a-f]{64}$")
        launch = (REPO / "server/run-sglang.sh").read_text()
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
