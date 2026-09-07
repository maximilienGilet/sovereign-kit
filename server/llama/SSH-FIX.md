# SSH startup repair (2026-09-04)

Vast instance 49874528 failed with `sshd: no hostkeys available -- exiting.`
Vast bypasses the image entrypoint, so its SSH launch missed our key generation.

The image now diverts the packaged sshd executable and initializes missing host
keys at the daemon boundary. Existing identities are preserved. The SysV service
matches the diverted executable but starts through the wrapper. No host private
keys are included in either image. Package-owned originals use dpkg diversions.

`Dockerfile.ssh-fix` adds only this repair to the pinned published runtime; it
does not rebuild llama.cpp or change CUDA. The full Dockerfile includes the same
installer for future builds.

## Verification and release state

- Old image: entrypoint-bypassed test reproduced the missing-hostkeys failure.
- Patched local image: authenticated SSH with strict host checking passes;
  identities survive daemon restart and differ between disposable containers.
- Precompiled runtime CPU smoke passes; this is not a GPU inference test.
- Service lifecycle assertions pass in native amd64 CI. They are skipped under local Rosetta,
  whose `/proc/PID/exe` reports the translator instead of the daemon. Native
  amd64 CI passed these assertions before publishing.
- GitHub workflow dispatch input `ssh_patch=true` selects the lightweight image
  repair and runs tests before pushing. Each test container has no external
  network and is removed after execution.
- Published from isolated branch `feat/ssh-startup-fix-20260904`, commit
  `ffc1988df9a40b8c72c57cfb7997bef0ace5678b`.
- CI: https://github.com/maximilienGilet/sovereign-kit/actions/runs/33897034100
- Verified anonymous manifest digest:
  `sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239`.
- Both local Qwen Solo recipes now use the published digest. The existing Vast
  instance is unchanged; it still uses the old image.

Existing containers do not pick up a new image automatically. No paid Vast
instance or GPU inference test was launched as part of this repair.
