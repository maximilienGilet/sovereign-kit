import importlib.util
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('smoke', pathlib.Path(__file__).with_name('smoke.py'))
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class SmokeTests(unittest.TestCase):
    def fixture(self, directory, body):
        root = pathlib.Path(directory)
        (root / 'source-revision').write_text(smoke.REVISION)
        binary = root / 'llama-server'
        binary.write_text('#!/bin/sh\n' + body)
        binary.chmod(0o700)
        return root

    def test_failed_binary_reports_loader_diagnostic(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.fixture(directory, 'echo "libmissing.so: cannot open shared object file" >&2\nexit 127\n')
            with patch.object(smoke.shutil, 'which', return_value='/fixture/tool'), patch.object(pathlib.Path, 'glob', return_value=[]):
                with self.assertRaisesRegex(RuntimeError, 'libmissing.so'):
                    smoke.check(root)

    def test_cpu_only_driver_is_scoped_to_smoke_subprocess(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.fixture(directory, 'test -L "${LD_LIBRARY_PATH%%:*}/libcuda.so.1" || exit 127\necho "draft-mtp --spec-draft-n-max --spec-draft-p-min --jinja"\n')
            stub = root / 'libcuda.so'
            stub.write_bytes(b'fixture, not a CUDA library')
            before = os.environ.get('LD_LIBRARY_PATH')
            with patch.object(smoke.shutil, 'which', return_value='/fixture/tool'), patch.object(pathlib.Path, 'glob', return_value=[]):
                smoke.check(root, cpu_only=True, cuda_stub=stub)
            self.assertEqual(before, os.environ.get('LD_LIBRARY_PATH'))


if __name__ == '__main__':
    unittest.main()
