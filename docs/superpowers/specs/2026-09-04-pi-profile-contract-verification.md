# Local Pi profile contract verification

Verified against installed `@earendil-works/pi-coding-agent` version 0.84.2,
using its own local source and documentation. No package update was performed.

## Configuration paths and values

- `dist/config.js`: `PI_CODING_AGENT_DIR` selects the agent directory;
  `models.json` is resolved inside it.
- `dist/core/settings-manager.js`: user defaults still use `settings.json`,
  including `defaultProvider` and `defaultModel`.
- `docs/models.md`: a keyless local provider needs a non-empty dummy `apiKey`
  to appear as available. The literal `local-qwen-tunnel` is compatible.
- `api: openai-completions` and provider-level compatibility flags
  `supportsDeveloperRole: false`, `supportsReasoningEffort: false` are supported.
- `dist/core/package-manager.js`: `pi install npm:...` uses the user-scope
  package root `<agentDir>/npm`, with packages under `node_modules/<name>`.

## Independent runtime check

Executed the installed `ModelRuntime.create` with a temporary `models.json`,
in-memory authentication and model store, `allowModelNetwork: false`, and
`PI_OFFLINE=1`. The fixture supplied a single `sovereign-qwen` model with an
explicit ID, context/output limits, loopback base URL and dummy key.

Assertions passed:

1. Runtime reported no configuration error.
2. The exact custom model was loaded.
3. The model appeared in `getAvailableSnapshot()` without login.
4. Its resolved request authentication used the non-secret placeholder.

The same assertions also passed against the actual output of
`clientprofile.Service.Install`, using a temporary home and a simulated package
runner. The generated file, not just the hand-written fixture, was loaded by the
installed Pi runtime. The package runner created inert package metadata in the
temporary staging directory; no package was downloaded or executed.

This verifies the profile/auth contract behind the user's “No models available”
error. It does not verify package installation, extension compatibility, remote
inference, or that any live user profile is installed. No live profile, saved
credentials or remote server was touched.
