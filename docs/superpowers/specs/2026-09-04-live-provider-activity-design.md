# Live provider activity and region filtering

User-approved scope:

- Region filter: World, North America, South America, Europe, Asia, Africa,
  Oceania. Optional country refinement. Expand regions into the existing
  server-side country query before loading offers, never only filter the page.
- Display the provider's actual status and status_msg during provisioning.
  Retain the last four distinct observed messages; emphasize the newest.
  Show age of the last successful check even if the message is unchanged.
  No fake percentage or invented progress. A layer's Pull complete is not
  instance readiness. Offline remains an error with explicit destroy recovery.
- Improve the existing fixed isometric cube with smoother lighting and material
  shading, not rotating geometry; preserve terminal state freeze and NO_COLOR.

Implementation boundaries:

Activity is an optional setup observer, separate from milestone progress.
The GET instance adapter decodes nullable status_msg into an empty string.
Each successful poll publishes a timestamped, identity-scoped, token-redacted
and terminal-control-stripped snapshot. Existing five-second polling remains;
this is sampled activity, not a push stream or complete Docker log.
Root-owned event generation fences and timer remain unchanged.
Compact mode prioritizes the latest message and billing identity over decoration.

Region membership uses installed CLDR M49 data, not an asserted replica of
Vast's proprietary country groupings. North America uses 003, including
Central America and the Caribbean. Region/country mismatches fail closed.

Verified: 459 tests across 14 packages; 321 race-enabled tests for CLI/setup/Vast;
go vet and diff whitespace checks. Fake-provider PTY preview includes the user's
example layer messages. No real provider resource was created or destroyed.
