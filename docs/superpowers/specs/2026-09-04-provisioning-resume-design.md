# Persistent provisioning resume

Status: implemented and reviewed (2026-09-04). Local fake-provider validation only; no live instance operations.

Legacy import requires exact provider image metadata matching the pinned recipe. Remote processes without a matching deployment fingerprint, ambiguous worker inventories, or an uncertain prior launch are refused rather than automatically relaunched.

## Goal

Resume an interrupted Vast deployment without creating another paid instance.
Cover both future deployments with a durable checkpoint and the user's existing
instance 49834278, created before checkpoints existed.

## User experience

- On startup, an unfinished deployment takes precedence over starting setup.
  Show instance ID, recipe, last confirmed operation and the billing warning.
- Offer Continue, Destroy (with the existing separate destructive confirmation),
  or Quit. Continuing is explicit; startup never launches a server automatically.
- Existing-instance recovery accepts an explicit instance ID. For 49834278,
  discover provider metadata and present the recipe and SSH identity for
  confirmation; do not infer an exact recipe revision from a label alone.
- Reuse the cube, live provider activity, normal keyboard controls and accessible
  text equivalents. An offline instance shows an actionable error, not a loader
  with fabricated progress.
- Never fall back from a failed resume to offer search or paid creation.

## Durable state

Store one versioned checkpoint beside the configured config file. Include the
instance ID, an exact recipe snapshot (including image digest and model revision),
SSH identity path, approved known-hosts reference, and last confirmed phase.
Do not store API tokens, private key bytes or environment credentials.

Use owner-only permissions, atomic replacement, and a single-writer lock.
Persist creation intent before the paid API call; replace it with the returned
instance ID immediately after successful creation, before polling or SSH.
If the create result is lost or persistence fails, retain the uncertain state,
show recovery instructions and prohibit automatic duplicate creation.
Malformed or unsupported checkpoint versions fail visibly and block new setup;
they are never silently ignored or overwritten.

## Resume execution

1. Read and validate the checkpoint; request the API token through the existing
   credential path. Check the current authenticated account can access the exact
   instance. An absent instance is not permission to create a replacement.
2. Re-fetch provider state and connection details. Wait through transient states
   using existing activity reporting; retain checkpoint on timeout or failure.
3. Reuse approved host trust only when the current endpoint and trusted key
   reference match. A changed endpoint/key requires explicit verification,
   never StrictHostKeyChecking=no or silent trust replacement.
4. Reconcile remote server state before launching. A healthy service matching
   the saved deployment is reused. An in-progress matching launch is observed.
   A different or ambiguous process/model is reported rather than killed or
   started alongside a duplicate.
5. Only launch the pinned recipe when absence of its server is established.
   Persist launch intent so interruption after dispatch is reconciled on retry.
6. Save the normal route configuration. Retire the checkpoint only after this
   succeeds; endpoint verification remains distinct from configuration saved.

Local last-phase markers are hints, never evidence that the remote instance is
still running, trusted, or serving the requested model.

## Recovery and destruction

Rebuild the existing exact-ID destroy capability after credentials and ownership
checks. Keep the checkpoint until absence has been verified following destruction.
Errors and cancellation retain recovery information. Quitting stops local work
only and does not imply that provider billing stopped.

## Tests

- Restart after create, during image download, after trust approval, during
  launch, and between launch success and config save.
- Resume never calls SearchOffers or CreateInstance.
- Changed host key, foreign/missing/offline instance, missing token, invalid
  checkpoint and stale completion cannot proceed or destroy the wrong instance.
- Interrupted or already-running server does not cause duplicate launch.
- Atomic persistence failures, uncertain creation and concurrent invocations
  cannot silently produce another paid instance.
- Sensitive values never appear in checkpoints, activity or error output.
- Small-terminal and accessible resume/confirmation controls stay visible.

## Scope

Implement and validate against local fixtures. Do not connect to, start services
on, or destroy the user's live instance as part of implementation testing.
Do not commit unrelated dirty-worktree changes.
