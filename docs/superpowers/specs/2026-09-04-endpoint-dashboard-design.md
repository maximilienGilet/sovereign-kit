# Endpoint dashboard and explicit client setup

## Approved product direction

Sovereign Kit exposes a private inference endpoint, not a coding-agent launcher.
The terminal dashboard remains open to own the SSH tunnel. Coding integrations
are optional and never launch a client process automatically.

## Main dashboard

Show the configured OpenAI-compatible base URL, the exact model identifier
reported by the server, local tunnel state, and endpoint availability.
Show the Vast instance identifier when applicable. Do not claim LIVE from
configuration alone, and do not confuse an active tunnel with a responding model.

The endpoint is constructed from the saved local route; the example is
`http://127.0.0.1:30000/v1`. Explain that it is reachable from this Mac while
the tunnel remains open. Do not expose a remote public endpoint or imply the
base URL is a browser page or an OpenAPI specification.

Keyboard navigation selects the endpoint, model identifier, or an integration.
Enter copies the selected endpoint or model identifier with success/failure
feedback. Copy raw values, without ANSI styles, wrapping, or trailing newlines.

Read model identity from a bounded request to `/v1/models`. Never substitute a
hard-coded Studio alias for Solo or a custom model. Missing, ambiguous, or failed
model discovery is visible and blocks model-specific profile installation.
Provide a retry action. Preserve the existing tunnel-loss cleanup behavior.

## Connect an application

Offer generic OpenAI-compatible connection instructions first: base URL,
model identifier, and the non-secret placeholder `local-qwen-tunnel` for clients
that require a key. Explain that the loopback endpoint itself is keyless and
protected by the SSH route; this is not a Vast API key.

Pi / Oh My Pi and OpenCode are optional integrations. Their flow is:

1. Inspect the selected integration's profile without writing files or installing
   packages. Show its exact destination path and inspection result.
2. If already compatible, show launch instructions immediately.
3. If absent, incomplete, or different, explain the differences and show an
   explicit installation/update confirmation. Entering this screen does not
   authorize any write. Default selection is cancel.
4. After explicit confirmation, install/update only this integration's profile.
   Show progress and keep the TUI responsive. On failure, retain the error and
   offer a retry; do not unlock launch instructions or claim readiness.
5. Reinspect installed files and dependencies. Only a successful verification
   unlocks the launch instructions. Enter then copies the command; it never
   launches Pi, Oh My Pi, or OpenCode.

Escape returns to the main dashboard without closing the tunnel. Existing
disconnect and quit actions retain their explicit lifecycle semantics.

## Profile compatibility and writes

A directory alone does not establish an installed profile. Inspect valid
configuration, the expected endpoint, model identity, default model selection,
and required integration packages. Report absent, ready, incomplete, different,
or unreadable states distinctly. An unreadable or unsafe path must not trigger
an overwrite fallback.

For Pi / Oh My Pi, use the existing isolated Sovereign profile mechanism and
its pinned integration packages. For OpenCode, use its isolated Sovereign config.
Resolve supported path overrides consistently between inspection, installation,
and the generated launch command. Do not silently replace the global user profile.

The profile must use the actual served model. Model limits must come from
verified deployment metadata or endpoint metadata; do not silently reuse the
hard-coded Studio limits. When the required metadata is unavailable, explain
what is missing and block installation rather than inventing model capabilities.

Before changing an existing configuration, retain a recoverable backup and show
the affected paths in the confirmation screen. Preserve unrelated settings,
credentials, sessions, and packages. Refuse symlink-based escapes from the
confirmed target. Failed installation must not erase the previous profile.
Recheck relevant state at execution time rather than trusting an old inspection.

All profile work is tied to the active application context. Cancellation or
disconnect must not leave an unowned installer worker. Late results from an old
connection cannot change the new dashboard's state.

## Layout

Reuse Bubble Tea, Bubbles and Lip Gloss. On wide terminals, put connection facts
and copyable values above an integration list/details area. On narrow terminals,
stack the same information with scrolling. The endpoint remains the primary
content; no empty metrics, decorative comparisons, or duplicated model cards.
The footer advertises the action for the current selection and current step.

## Acceptance checks

- Opening the dashboard or selecting an integration performs no installation.
- Compatible profiles skip installation and expose a correct launch command.
- Missing, incomplete and different profiles require explicit confirmation.
- Cancel, inspection failure and installation failure never expose a false ready state.
- Newly installed profiles select the actual served model and private endpoint.
- Existing user data survives an update and failures are recoverable.
- Copy success and failure are visible; no client executable is launched.
- Disconnect cancels workers; stale results do not revive a closed connection.
- Endpoint discovery failures and multiple model identifiers fail closed.
- Small terminals retain navigation, copy actions and access to errors.

## Development boundaries

Implementation and automated tests use temporary profiles and simulated
installation/network boundaries. No live profile installation, Vast instance
creation/deletion, paid deployment, or remote runtime migration is authorized by
this design approval. Public image publication remains a separate unfinished task.
