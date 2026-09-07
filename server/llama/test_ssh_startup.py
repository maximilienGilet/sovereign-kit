"""Run inside a disposable image with its ENTRYPOINT bypassed (as Vast does)."""
import pathlib
import socket
import sys
import subprocess
import tempfile
import time


def run(*args):
    return subprocess.check_output(args, stderr=subprocess.STDOUT, text=True, timeout=15)


def check():
    assert not list(pathlib.Path('/etc/ssh').glob('ssh_host_*_key')), 'Image contains shared host keys'
    with tempfile.TemporaryDirectory(dir='/root') as directory:
        root = pathlib.Path(directory)
        key = str(root / 'client')
        run('ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', key)
        fingerprint = None
        for _ in range(2):
            with tempfile.TemporaryFile(mode='w+') as logs:
                server = subprocess.Popen([
                    '/usr/sbin/sshd', '-D', '-e', '-p', '2222',
                    '-o', 'ListenAddress=127.0.0.1',
                    '-o', 'PermitRootLogin=yes', '-o', 'PasswordAuthentication=no',
                    '-o', 'UsePAM=no', '-o', f'AuthorizedKeysFile={key}.pub',
                ], stdout=logs, stderr=logs)
                try:
                    for attempt in range(50):
                        if server.poll() is not None:
                            logs.seek(0)
                            raise AssertionError('sshd exited: ' + logs.read())
                        try:
                            scanned = run('ssh-keyscan', '-T', '1', '-t', 'ed25519', '-p', '2222', '127.0.0.1')
                            if 'ssh-ed25519 ' in scanned:
                                break
                        except subprocess.CalledProcessError:
                            pass
                        time.sleep(0.1)
                    else:
                        raise AssertionError('SSH did not become ready')
                    known = root / 'known_hosts'
                    known.write_text(scanned)
                    result = run('ssh', '-F', '/dev/null', '-p', '2222', '-i', key,
                                 '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes',
                                 '-o', 'StrictHostKeyChecking=yes', '-o', f'UserKnownHostsFile={known}',
                                 'root@127.0.0.1', 'printf ssh-startup-ok')
                    assert result == 'ssh-startup-ok', result
                    current = run('ssh-keygen', '-lf', '/etc/ssh/ssh_host_ed25519_key.pub').split()[1]
                    assert fingerprint is None or current == fingerprint, 'Restart changed host identity'
                    fingerprint = current
                except subprocess.CalledProcessError as error:
                    logs.seek(0)
                    raise AssertionError(error.output + '\nServer: ' + logs.read()) from error
                finally:
                    if server.poll() is None:
                        server.terminate()
                    server.wait(timeout=5)
        # Vast may use the distribution service rather than invoke sshd directly.
        run('service', 'ssh', 'start')
        with socket.create_connection(('127.0.0.1', 22), timeout=2) as connection:
            assert connection.recv(128).startswith(b'SSH-')
        pid = pathlib.Path('/run/sshd.pid').read_text()
        # Rosetta reports its translator as /proc/PID/exe, so Debian's --exec
        # matching cannot work there even for an unmodified sshd. Native amd64
        # CI must exercise the lifecycle assertions below.
        executable = str(pathlib.Path(f'/proc/{pid.strip()}/exe').readlink())
        if 'rosetta' in executable.lower():
            print('SKIP service lifecycle: Rosetta executable matching; required in native CI', file=sys.stderr)
            print(fingerprint)
            return
        run('service', 'ssh', 'restart')
        assert pathlib.Path('/run/sshd.pid').read_text() != pid, 'Service restart did not replace daemon'
        run('service', 'ssh', 'stop')
        for _ in range(30):
            try:
                with socket.create_connection(('127.0.0.1', 22), timeout=1):
                    pass
            except ConnectionRefusedError:
                break
            time.sleep(0.1)
        else:
            raise AssertionError('Service stop left SSH listening')
        print(fingerprint)


if __name__ == '__main__':
    check()
