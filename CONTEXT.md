# Ubiquitous language

## Consultant team

The primary user of Sovereign Kit: a technical consultant or delivery team that deploys coding agents for client work with heightened confidentiality, egress-control or data-location requirements.

## Sovereign Kit

The project under design: a provider-neutral setup kit that turns an approved inference route into a ready-to-use private AI environment for a Consultant Team. Its V1 promise is a **verified private execution path**: it configures and verifies that the parent agent, subagents and inference use an approved route without model-provider fallback, then emits a human-readable diagnostic. It explicitly does not claim that private networking alone establishes legal or contractual sovereignty.

## Dashboard

An optional local user interface over the same policy, verification report and lifecycle state as the CLI. It must not become a second configuration surface or a required dependency for the verified private execution path.

## Inference Target

A provider-neutral description of an approved inference destination: private endpoint, access method (SSH/VPN/Tailscale), expected model and verification policy.

## Provider Setup Kit

The project’s central product concept: a reproducible kit that turns an approved Compute Provider or External SSH Target into a ready-to-use private AI environment for supported coding-agent harnesses. It handles the setup path from provider to model endpoint to local developer tool; it is not merely an auditor or a generic local-model manager.

## Compute Provider Adapter

An optional adapter that manages lifecycle/cost state for an Inference Target. V1 ships a Vast.ai adapter with lifecycle support and an External SSH Target adapter with verification/tunneling but no provisioning.

## Core License

The provider-neutral core is Apache-2.0. Commercial value comes from consulting, integration, support, training, client-specific policies, specialized adapters and potential future enterprise extensions—not from restricting use of the core.

## Commercial Model

The open-source project is proof, acquisition and a delivery accelerator. The initial paid offer is a fixed-scope implementation of a verified private agent environment: approved inference route, policy, provider adapter, private access, verification report, operating runbook and team handover. A later operating contract can cover upgrades, security reviews, FinOps optimization, provider/model changes and incident support.

## Initial Buyer

The initial buyer is a CTO, DSI, engineering lead or innovation lead in a PME/ETI that already wants to use coding agents, but cannot send a repository or sensitive data to a public model provider without an acceptable security/client framework. The buying trigger is an agent project blocked by confidentiality, customer or security requirements—not an abstract desire for sovereignty.

## Delivery Boundary

The paid offer provides a bounded technical guarantee: an approved and verified inference route, versioned policy, configured egress/fallback controls, audit/status report, operating runbook and explicit residual risks. It does not certify legal sovereignty, GDPR/cybersecurity compliance, provider contractual compliance, or unlimited performance/availability.

## V1 Verification Interface

V1 exposes a single human-readable `sovkit doctor` diagnostic. It verifies the configurable local path and clearly states what it cannot verify. Machine-readable reports, CI gates, signed attestations and retained audit history are deferred until a concrete customer workflow requires them.

## V1 Agent Harnesses

V1 supports Pi (including Oh-My-Pi and pi-subagents) and OpenCode. Both must resolve the same private Qwen Inference Target. OpenCode is configured through a provider allowlist injected at runtime with OpenCode’s highest-precedence inline configuration, so a repository-local config cannot re-enable an external provider.
