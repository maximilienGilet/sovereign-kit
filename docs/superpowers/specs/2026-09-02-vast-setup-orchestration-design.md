# Vast Setup Orchestration Design

## Goal

Make the primary private-inference journey:

```text
export VAST_API_KEY='***'
sovkit setup
sovkit start
```

`setup` provisions one explicitly selected Vast instance or configures the existing manual SSH route. `start` owns the strict loopback tunnel, waits for the local OpenAI-compatible endpoint, runs the dashboard, and stops the tunnel when the dashboard exits. No command falls back to a public inference provider.

## Scope

This change implements Vast and manual SSH setup. The provider selector remains small and can add provider-specific workflows later; it is not a generic plugin framework.

Included:

- provider selection with Huh;
- a binary-bundled `qwen-studio` recipe;
- corrected Vast offer and instance-creation contracts;
- offer shortlist, cost scenarios, and explicit paid-resource confirmation;
- bounded instance polling;
- host-key scan, fingerprint display, explicit trust, and isolated known-hosts persistence;
- controlled remote SGLang startup after host trust;
- Vast configuration persistence;
- foreground `sovkit start` lifecycle;
- injected boundaries and deterministic tests.

Excluded:

- additional managed providers;
- OS keychain storage;
- arbitrary Docker, shell, or SGLang flags;
- automatic instance destruction;
- detached tunnel/process management;
- throughput claims or automatic benchmarks;
- legacy directory deletion.

## Architecture

### Command layer

`internal/cli` owns Huh presentation and provider selection. Manual SSH remains an explicit provider choice and retains its strict credential-file validation.

The command entrypoint owns an application dependency set so tests can provide fake setup, process, health, and dashboard implementations. Production dependencies are constructed only at the executable boundary.

### Built-in recipe

The existing `recipes/qwen-studio.toml` becomes accessible through a binary-bundled recipe loader. Setup validates the recipe before making provider calls. The recipe remains the only source of runtime image, model revision, serving limits, minimum VRAM, and disk requirement. The built-in recipe requests 120 GB.

The recipe runtime image and model revision remain immutable. Setup never resolves a mutable model or image reference.

### Vast adapter corrections

Vast REST offer search uses:

- `type: "ondemand"`;
- verified, rentable, and not-rented filters;
- `gpu_ram >= minimum_vram_gb * 1024`, because REST uses MB.

Returned `gpu_ram` is normalized from MB to GB before reaching planner/UI code. Instance creation uses `runtype: "ssh_direct"`. The adapter sends no `onstart` command.

### Vast orchestrator

A focused orchestrator depends on narrow interfaces for:

- `SearchOffers`;
- `CreateInstance`;
- `GetInstance`;
- operator interaction;
- polling time;
- host-key scan/fingerprint;
- strict SSH remote execution;
- known-hosts/config persistence.

The interfaces expose provider operations and observable decisions, not Huh, `exec.Cmd`, or filesystem implementation details.

### Start lifecycle

`sovkit start` loads validated config, constructs the existing strict route command, starts it, polls local `/v1/models` every five seconds for at most thirty minutes, runs the dashboard after health succeeds, and always terminates and reaps the SSH process when the dashboard returns or startup fails.

`start` stays foreground-owned. `tunnel` and `doctor` remain available as separate diagnostic commands.

## Setup Data Flow

1. Show provider choice: Vast or Manual SSH.
2. For Vast, require non-empty `VAST_API_KEY` from the environment.
3. Load and validate `qwen-studio`.
4. Prompt for an existing private-key path, defaulting to `~/.ssh/id_ed25519`; require a readable regular file.
5. Search at most five eligible offers using the recipe VRAM floor.
6. Filter defensively and sort by visible hourly compute price.
7. Show GPU, normalized VRAM, hourly price, 730-hour monthly scenario, 8,760-hour annual scenario, location, and reliability. Label performance `unknown/unmeasured`. Label costs as compute scenarios; do not imply inclusion of storage, egress, tax, or future price changes.
8. Let the operator select one offer.
9. Show offer ID, GPU, location, fixed disk allocation, hourly rate, scenario costs, and the fact that billing continues after CLI exit.
10. Require an explicit confirmation. Cancellation performs no create or persistence operation.
11. Create one SSH-direct instance from the recipe’s digest-pinned runtime image, with no public inference-port mapping and no `onstart` command.
12. Poll every five seconds for at most ten minutes. Success requires `actual_status == "running"`, non-empty SSH host, and a valid SSH port. `exited`, `unknown`, and `offline` fail immediately.
13. Scan the instance SSH endpoint and derive SHA-256 fingerprints from the returned host keys.
14. Display every fingerprint as first-use trust material and require explicit operator confirmation.
15. Write raw scanned keys to a per-instance isolated known-hosts file with mode `0600`. If that path already exists with different content, fail without overwriting it.
16. Use strict SSH with the selected identity and isolated known-hosts file to launch the reviewed SGLang command. No remote execution occurs before trust confirmation.
17. Save mode-`0600` TOML with `provider.kind = "vast"`, instance ID, SSH host/port/user, identity path, known-hosts path, and the fixed loopback route.
18. Print the instance ID, config path, ongoing billing warning, and `sovkit start` as the next action.

The Vast SSH user is `root`. The matching public key must already be registered with Vast before instance creation.

## Controlled Server Command

The remote command is generated only from a validated typed recipe. The production runner safely quotes each argument and starts the service without keeping the bootstrap SSH session attached.

For `qwen-studio`, it invokes `sglang serve` with:

- the pinned Hugging Face repository and revision;
- recipe context window and request limits;
- reviewed fixed cache/attention/parser values already represented by the historical server configuration;
- `--host 127.0.0.1`;
- `--port 30000`.

Output goes to a fixed remote log path. The user cannot inject environment variables, shell fragments, image names, model references, ports, or additional server flags.

## Failure Semantics

Before instance creation, every failure is side-effect free except terminal output.

After instance creation, any polling, host-key, remote-start, or config error includes the instance ID and states that billing may still be active. The tool does not destroy the instance automatically because deletion is consequential and may discard paid state.

Specific stop conditions:

- missing API key;
- no eligible offers;
- offer-selection cancellation;
- cost-confirmation rejection;
- provider terminal instance status;
- readiness timeout;
- missing SSH endpoint;
- empty or malformed host-key scan;
- host-key rejection;
- changed pre-existing isolated host key;
- remote server launch failure;
- config persistence failure;
- tunnel startup or health timeout.

No failure path selects another provider or endpoint.

## Testing

Development is test-first.

Adapter tests assert:

- `ondemand` request type;
- GB-to-MB request conversion;
- MB-to-GB response normalization;
- bearer authentication;
- `ssh_direct` creation without `onstart`.

Orchestrator tests assert:

- missing-key rejection;
- deterministic eligible-offer ordering and labeled scenario costs;
- no creation before explicit cost confirmation;
- exact controlled create request;
- bounded polling and terminal-status handling;
- no remote execution or persistence before host-key confirmation;
- changed-key refusal;
- controlled server arguments and loopback binding;
- successful `0600` known-hosts and config persistence;
- instance ID and billing warning on every post-create failure.

Command tests assert:

- Vast/manual provider selection;
- existing manual secure-route behavior;
- `start` health-before-dashboard ordering;
- tunnel cleanup on success and failure;
- no real Vast, SSH, health endpoint, process, or TTY use.

Verification runs focused tests during development, then:

```text
go test ./...
go vet ./...
git diff --check
```

A CLI smoke check verifies help output and provider-aware setup/start command routing without provisioning a real instance.
