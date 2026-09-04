"""Build-time runtime check. No GPU, model download, credentials or server bind."""
import argparse
import os
import pathlib
import shutil
import subprocess
import tempfile

ROOT = pathlib.Path('/opt/sovereign-kit/llama')
REVISION = '86b351fd64d5ebbf1ba795ffd60c8f4a8c958613'


def check(root=ROOT, cpu_only=False, cuda_stub=pathlib.Path('/usr/local/cuda/lib64/stubs/libcuda.so')):
    if list(pathlib.Path('/etc/ssh').glob('ssh_host_*')):
        raise RuntimeError('Image must not contain SSH host keys')
    if (root / 'source-revision').read_text().strip() != REVISION:
        raise RuntimeError('Unexpected llama.cpp source revision')
    for tool in ('python3', 'curl', 'flock', 'sshd'):
        if not shutil.which(tool):
            raise RuntimeError('Missing runtime tool: ' + tool)
    # CI has no injected GPU driver. Expose the toolkit stub to --help only,
    # never in the image's global loader path or the serving environment.
    with tempfile.TemporaryDirectory(prefix='llama-smoke-driver-') as temporary:
        env = os.environ.copy()
        if cpu_only:
            if not cuda_stub.is_file():
                raise RuntimeError('Missing CUDA driver stub: ' + str(cuda_stub))
            pathlib.Path(temporary, 'libcuda.so.1').symlink_to(cuda_stub)
            env['LD_LIBRARY_PATH'] = temporary + ':' + env.get('LD_LIBRARY_PATH', '')
        result = subprocess.run([str(root / 'llama-server'), '--help'],
                                check=False, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=30, env=env)
    if result.returncode:
        raise RuntimeError(f'llama-server --help exited {result.returncode}:\n{result.stdout}')
    for option in ('draft-mtp', '--spec-draft-n-max', '--spec-draft-p-min', '--jinja'):
        if option not in result.stdout:
            raise RuntimeError('Missing runtime option: ' + option)
    print('PASS: pinned llama.cpp, embedded MTP CLI and runtime tools')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cpu-only', action='store_true', help='Isolated toolkit driver stub for the CI help check only')
    check(cpu_only=parser.parse_args().cpu_only)
