# Vast balance and SSH trust design

## Goal

Keep the operator informed about Vast credit without interrupting setup, and remove an SSH confirmation that cannot be meaningfully verified from the TUI alone.

## Global Vast balance

- Read the authenticated Vast account balance through a narrow Vast client method.
- Show `VAST  $12.34` right-aligned in the global application header on every full-size screen, including provisioning and errors.
- Fetch once when the application starts, then refresh every 60 seconds without blocking input or rendering.
- Retain the latest successful value during transient refresh failures.
- Show no balance when no Vast credential exists or no successful value has been read. Never render zero as a fallback and never expose API errors in the header.
- Truncate the left-hand screen title first when the terminal is narrow. The balance must never wrap or displace the body.

## SSH trust

- For a Vast instance created by Sovereign Kit, accept and persist the first complete host-key set automatically. This is trust on first use, not certificate verification.
- On resume or reconnect, require the scanned keys to match the saved digest. A mismatch remains a hard failure and cannot be approved interactively.
- Keep the manual SSH flow unchanged: it requires a user-supplied, already verified `known_hosts` file and never performs first-use trust itself.
- Remove the Vast fingerprint prompt from both the interactive and accessible setup flows. Provisioning moves directly from key collection to the launch step.

## Boundaries and failure handling

- Balance reads are read-only, authenticated, cancellable, time-bounded, and never delay setup.
- A balance response is accepted only when its amount is finite and non-negative.
- Empty or incomplete SSH key material still fails before anything is trusted.
- Existing saved checkpoints and their host-key digests remain compatible.

## Verification

- Vast client tests cover valid, malformed, unauthorized, and missing balance responses.
- Application tests cover initial fetch, refresh, stale-value retention, global placement, narrow layouts, credential absence, and cancellation.
- Orchestrator tests prove Vast-created instances need no confirmation, first-use keys are saved, and changed keys are rejected. Existing manual-route tests continue to require a readable verified `known_hosts` file.
