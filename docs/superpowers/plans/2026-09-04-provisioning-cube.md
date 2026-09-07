# Provisioning Cube Implementation Plan

**Goal:** Realize the approved README-inspired cube and centered loading layout.

**Architecture:** Pure cube renderer, existing root-owned timer, Lip Gloss
composition. Provider and recovery behavior remain unchanged.

**Tech Stack:** Go, Bubble Tea, Bubbles, Lip Gloss, ANSI terminal cells.

## Constraints

Preserve the dirty worktree. No real provider actions or commits.
No new dependencies. Keep compact mode, NO_COLOR and paid warnings.

## Execution

- [x] Add failing composition test in internal/cli/provisioning_cube_test.go:
  require centered status, common active/future column and inset instance ID.
  Run rtk go test ./internal/cli -run TestProvisioningCube -count=1.
  Observed failure: old warning at column zero, mismatched step alignment.
- [x] Extract stationary cube geometry to internal/cli/provisioning_cube.go;
  draw double outer contours and inner cube using 2×4 Braille subcells.
- [x] Replace orbital rendering in internal/cli/provisioning_loader.go;
  compose cube and fixed-width timeline using Lip Gloss; compact fallback.
- [x] Center the bounded body and paid warning in application_view.go.
- [x] Test color-only animation, frozen error/success markers and dimensions.
- [x] Run complete test suite: 450 tests passed across 14 packages.
- [x] Inspect truecolor fake-provider PTY preview; no external resources used.
