# Prebuilt llama.cpp Implementation Plan

**Goal:** Publish a public, model-free GHCR image and eliminate build/package commands from new Solo provisioning.

**Architecture:** Build the already-verified CUDA13.3.1/llama.cpp source pins on GitHub x86. Image includes Python, curl, SSH prerequisites, native MTP binary and source provenance. New recipes opt into a precompiled runtime only after a public digest is verified; old snapshots retain the source-build route.

**Tech stack:** Docker multi-stage build, GitHub Actions, Go/Python launcher.

## Constraints

- No changes to current Vast instances; no secret/model files in build context.
- Publish only server/llama build files and the dedicated workflow on a new feat branch based on remote main.
- Check runtime tools/MTP flags before publication; verify anonymous registry access and immutable digest before changing recipes.
- Native CUDA build is not a GPU model-load benchmark.

## Steps

- [ ] Add bounded image smoke check and Dockerfile for existing pins; build on GitHub Actions using package-scoped credentials.
- [x] Add failing no-build supervisor test, implement explicit precompiled mode with source verification, and retain legacy recipe fingerprints via omitted JSON defaults.
- [ ] Verify image publication/public pull; pin both Solo recipes to the resulting digest and precompiled mode.
- [ ] Run complete tests/build and document public artifact plus legacy resume behavior.

## Verification in progress

- Local Go suite: 516 passing tests; `go build ./...` and Docker build checks pass.
- Scoped code review found generated SSH host keys in the image; fixed by
  same-layer removal, an image smoke assertion, and per-container startup generation.
  Review of the correction passed; CI verifies two distinct public fingerprints.
- Public build branch: `feat/prebuilt-llama-runtime`, source commit
  `785066949876103519a16b8be7dc3f80b0ef13a9`.
- Build: https://github.com/maximilienGilet/sovereign-kit/actions/runs/33863201655
- Recipe activation remains gated on successful build and anonymous digest access.

## Publication repair in progress

- The original run compiled llama-server successfully, then failed its CPU-only
  help check with exit 127; the check hid the loader diagnostic.
- Exact pinned CUDA CMake source links `CUDA::cuda_driver`. CPU CI has no injected
  GPU driver. The repaired check scopes the toolkit stub to one temporary child
  environment and reports the actual diagnostic; serving is unchanged.
- Two regression tests passed after failing against the previous smoke check.
  Independent review found no blocker (minor: empty inherited loader path adds a
  current-directory segment in controlled CI only).
- Publication source: `d2df722e7c4c3f1195ed9c60d894d0f79ea78769`, dedicated branch
  `feat/prebuilt-llama-runtime`, temporary checkout
  `/private/tmp/sovkit-prebuilt-publish.F42p2t/source`.
- Active build: https://github.com/maximilienGilet/sovereign-kit/actions/runs/33875422503
- Both new bundled-supervisor regression cases currently fail as expected while
  recipes still select the legacy source-build mode. Do not turn them green by
  activating an unpublished or non-public image.
- Current Vast instances are explicitly outside mutation scope.

## Publication and activation completed

- Run 33875422503 succeeded in 24m14s: build, CPU help/MTP check,
  separate SSH identities, and GHCR push all passed.
- Published immutable digest:
  `ghcr.io/maximiliengilet/sovereign-kit-llama@sha256:b5079e645adf7390cd00119719aa93a2566e0f9d53177bfdc506418ad311f631`.
- Anonymous registry verification passed: manifest hash and digest header agree,
  config is amd64 and labels source d2df722, expected entrypoint, all 21 layer
  download HEAD requests succeeded. No CI stub directory appears in runtime
  LD_LIBRARY_PATH.
- Both Solo recipes now pin that digest with `precompiled = true`.
  Both actual-recipe supervisor regression tests now pass; all 537 Go tests
  passed (15 packages), and the two smoke regression tests passed.
- CLI rebuilt and installed with install-macos.sh; installed binary matches the
  independently built verification binary. Profiles and Vast instances untouched.
- Full GPU/model loading remains untested; no paid instance was created.
- All 13 existing Python tests passed with SOVKIT_ENDPOINT_URL pointed to
  `http://127.0.0.1:0/v1/models`: the missing-tunnel test otherwise sees the user's
  real running endpoint and correctly gets a healthy response. This is test
  environment isolation only; no endpoint or test source was changed.
