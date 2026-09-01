# Controlled automation

This runbook uses the private route to create a **proposal**, then leaves validation, approval, and execution to a client-owned workflow. The model never receives credentials and never invokes a tool itself.

## Architecture

```text
approved input → private route → structured proposal
               → client schema/business-rule validation
               → human approval when required
               → client-owned worker executes the approved action
```

Keep the worker and inference route separate. A private model endpoint does not authorize a business action.

## Prerequisites

- working [server setup](../server.md), SSH tunnel, and passing `sovkit doctor`;
- a client-owned workflow engine or application;
- a defined action allowlist and JSON schema;
- named human approvers for sensitive actions;
- an audit location for proposal, validation, approval, and execution outcome.

Start with drafts, classifications, or extractions. Do not start with money movement, access changes, deletion, external communications, or irreversible writes.

## Run the included proposal-only example

```bash
python3 examples/controlled-automation/propose.py \
  --allowed-action draft_reply \
  --input 'Customer asks for a status update.'
```

The output is JSON only if all of these conditions hold:

- the model returned parseable JSON;
- `action` is in the explicit `--allowed-action` list;
- `requires_approval` is exactly `true`.

The program does **not** send email, call an API, write a file, run shell code, browse the web, or execute the proposal. It is a tested boundary example, not a workflow engine.

## Build the production path

1. Authenticate the initiating user in the client application.
2. Construct a minimal, approved task input.
3. Call the private route with an explicit output schema and action allowlist.
4. Validate response shape, fields, allowed action, target identifiers, and business rules in application code.
5. Store the proposal with immutable request references permitted by the retention policy.
6. Require approval where the action can affect a customer, system, money, access, or irreversible state.
7. Execute only through a scoped client-owned worker credential.
8. Record execution result and expose a retry/rollback procedure.

A model result is untrusted input. Validate it exactly as you would validate a browser request or third-party webhook.

## Approval and policy

Require explicit human approval for at least:

- sending an external communication;
- changing permissions or account state;
- creating financial commitments;
- deleting or overwriting records;
- calling an external system with broad credentials.

Make approval bind to the exact validated proposal, not merely the original request. If the proposal is regenerated, require a new approval.

## Operations and failure handling

Before enabling a workflow, test with a non-sensitive fixture and a non-production worker target. Confirm the audit record shows input reference, model proposal, validation decision, approver, action ID, and result.

- **Endpoint failure:** stop; inspect `sovkit doctor`, tunnel, and server. Never silently fall back to a public model.
- **Invalid JSON or unknown action:** reject the proposal. Do not attempt natural-language recovery and execute it.
- **Approval missing:** persist or display the proposal as pending; do not enqueue execution.
- **Worker failure:** record the outcome; use the client system’s idempotency and rollback design. Do not ask the model to retry blindly.
- **Unexpected external effect:** disable the workflow and investigate the client’s validation/authorization path.

Stopping the tunnel or GPU host does not undo client-side writes. The workflow owner needs its own idempotency, retry, and rollback controls.

## Data, secrets, and egress

Keep business-system credentials only in the client worker’s secret store. Do not put them in model prompts, templates, repository files, Pi/OpenCode profiles, or environment dumps.

The inference endpoint needs no browser, shell, MCP, or business-system egress for this pattern. Any such capability is a separate opt-in product decision with identity, allowlists, credentials, audit, and approval controls.

## Out of scope

Sovereign Kit does not ship a scheduler, queue, policy engine, credentials vault, approval UI, webhook receiver, browser, shell runner, MCP server, or business-system connector.
