# Provisioning Resume Implementation Plan

**Goal:** Resume existing paid instances without duplicate creation or launch.
**Architecture:** Optional durable checkpoint at the setup boundary, enabled in
production; the TUI/accessibility layer routes to resume before normal setup.
**Tech Stack:** Go, Bubble Tea, filesystem atomic writes and OS file locks.

## Global Constraints

Preserve dirty worktree; no commits or real provider operations. Never silently
replace host trust. No credentials in checkpoint. Resume never creates an offer.
Current legacy instance is recovered by explicit ID and recipe selection.

### Task 1: Persistent setup engine

Ownership: internal/setup checkpoint/resume implementation, orchestrator.go,
system.go resume reconciliation and their tests; optional Vast metadata fields.
Interfaces:
- Options.CheckpointPath string: opt-in journal path (production enables).
- Options.Resume bool: resume journal, no offer selection/creation.
- Options.ResumeInstanceID int: explicit legacy import if no known-ID journal.
- Checkpoint contains Version, InstanceID, Recipe recipe.Recipe,
  IdentityFile, KnownHostsDir, Phase, plus approved endpoint/trust as needed.
- ReadCheckpoint(path string) (Checkpoint,error); os.IsNotExist means absent.
- CheckpointPath(configPath string) string returns configPath+".pending.json".
- RunVast retains signature and handles both create and resume via Options.

- [ ] Write failing tests: journal intent precedes create, returned ID survives
  failure; retry makes zero CreateInstance/SearchOffers calls; corrupt journal
  blocks creation; concurrent operations block; no credential serialization.
- [ ] Atomic owner-only checkpoint writes and single-writer lock; failure keeps
  uncertainty rather than permitting another paid creation.
- [ ] Resume re-fetches exact instance and retains recovery on offline/error.
  Legacy import must verify provider image compatibility and require explicit
  selected recipe/identity; do not infer pinned recipe from labels.
- [ ] Trust reuse only for unchanged approved endpoint and key. Reconciliation
  requires matching deployed recipe fingerprint; ambiguous processes refuse.
  Preserve explicit destroy; journal cleared only after config saved or absence
  verified under lock. Interrupted launch must not produce duplicate processes.
- [ ] Run setup/Vast tests, report exact API and residual constraints.

### Task 2: Application integration

Ownership: internal/cli and cmd/sovkit, tests, README.
- [ ] Add resume entry and explicit ID parsing; home/setup/start detects pending
  state before launching another setup or using an old route.
- [ ] Read checkpoint safely, show exact-ID Continue/Destroy/Quit choice,
  existing token prompt; no new paid confirmation or recipe selection on a
  valid journal. Legacy resume ID asks recipe and identity explicitly.
- [ ] Route the existing worker/event pipeline to RunVast resume options.
  Preserve live activity, recovery actions, cancellation and redaction.
- [ ] Accessible equivalent and CLI usage; fake-runner tests for automatic
  pending detection, explicit consent, malformed state, and legacy ID path.

### Task 3: Integration review

- [ ] Review new snapshot against spec and safety requirements.
- [ ] Full Go tests and race tests for touched concurrency boundaries.
- [ ] Report how to resume 49834278; do not run that operation on real provider.
