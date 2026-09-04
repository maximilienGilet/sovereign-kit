"""Build-time runtime check. No GPU, model download, credentials or server bind."""
import pathlib
import shutil
import subprocess

ROOT = pathlib.Path('/opt/sovereign-kit/llama')
REVISION = '86b351fd64d5ebbf1ba795ffd60c8f4a8c958613'


def check(root=ROOT):
    if (root / 'source-revision').read_text().strip() != REVISION:
        raise RuntimeError('Unexpected llama.cpp source revision')
    for tool in ('python3', 'curl', 'flock', 'sshd'):
        if not shutil.which(tool):
            raise RuntimeError('Missing runtime tool: ' + tool)
    result = subprocess.run([str(root / 'llama-server'), '--help'],
                            check=True, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, timeout=30)
    for option in ('draft-mtp', '--spec-draft-n-max', '--spec-draft-p-min', '--jinja'):
        if option not in result.stdout:
            raise RuntimeError('Missing runtime option: ' + option)
    print('PASS: pinned llama.cpp, embedded MTP CLI and runtime tools')


if __name__ == '__main__':
    check()
