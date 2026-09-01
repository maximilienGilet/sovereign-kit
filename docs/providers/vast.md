# Vast.ai: optional GPU-host worksheet

Vast.ai can be one way to obtain a GPU host. It is not required by Sovereign Kit and it is not the server contract: the contract remains one loopback-bound Qwen/SGLang endpoint reached through a verified SSH tunnel.

**Status:** this is an operator worksheet, not a published ready-to-run Vast template. The current `server/run-sglang.sh` recipe has not been live-validated inside a Vast template/container. Do not represent this as a one-click deployment.

## What this adapter does not do

It does not use a Vast API key, create an instance, select an offer, create a public inference port, store SSH material, manage billing, or destroy an instance. Those decisions stay with the operator.

## Create a private template

In the Vast template UI, create a **private** template intended only to speed up repeated host selection.

| Field | Value / rule |
|---|---|
| Name | `Sovereign Kit host v1` or a client-private name |
| Visibility | Private. Do not publish a template until its image, network, and launch flow have been tested. |
| Launch mode | SSH, so the operator can inspect the host and obtain SSH forwarding. |
| Image | Use a versioned SSH-capable Vast base image accepted by the UI. Do **not** claim that the OCI digest in `server/image.lock` is accepted by Vast template fields; Vast documents image tags, not digest references. |
| Disk | Choose enough persistent disk for the model cache, runtime, and logs after checking the selected model’s current size. Disk cannot be resized after instance creation. |
| Public ports | Leave application port mappings empty. Do not map port `30000`. |
| Instance Portal | Do not configure a portal or Cloudflare tunnel for the inference service. |
| On-start script | Leave empty in V1. Do not fetch boot scripts from a mutable URL. |
| Secrets | Never put SSH keys, API keys, Hugging Face tokens, registry credentials, hostnames, or client data in the template. |

Save the template. Its Vast configuration revision is identified by a template hash, but that does not freeze the bytes behind an image tag.

## Choose an instance

For each launch, choose the offer explicitly. Record the GPU model and VRAM, region/provider approval, disk, interruptibility, hourly price, and deletion procedure.

The published Studio benchmarks used an RTX PRO 6000 S with 96 GB VRAM. They do not prove that another offer supports 262K context or five simultaneous requests. See [Route profiles](../profiles.md) and the [benchmark contract](../benchmark-contract.md).

Before renting, decide:

- maximum hourly and total spend;
- whether an interruptible offer is acceptable;
- whether the provider, host location, storage, and retention terms have been approved for the intended material;
- who stops or destroys the instance.

## Prepare the host manually

After the instance starts, obtain its current SSH host, port, and user from Vast. These values are deployment-specific: never commit or reuse them as documentation defaults.

1. Verify the SSH host key through an approved independent channel.
2. Save that verified key in a dedicated known-hosts file on the Mac.
3. Connect to the host and check whether the existing Linux host-Docker prerequisites are present:

   ```bash
   docker --version
   nvidia-smi
   ```

4. If Docker and NVIDIA Container Toolkit are available, use a reviewed checkout or release of Sovereign Kit and run:

   ```bash
   ./server/run-sglang.sh
   ```

The script uses host Docker and `--network host`, then binds SGLang itself to `127.0.0.1:30000`. A Vast direct-container template cannot simply paste this script into `onstart`: it invokes Docker and assumes host networking. That direct-container path is deferred until separately tested.

## Connect the Mac

With SGLang running on the remote host, open the existing strict tunnel on the Mac:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
  <identity-file> <known-hosts-file>
```

Then run:

```bash
sovkit doctor
```

Continue only after it passes. The endpoint must remain local on both sides of the tunnel. Do not work around a failure by adding a Vast public port, an Instance Portal route, or a `0.0.0.0` SGLang bind.

## Stop and destroy

Follow [Operations](../operations.md), then explicitly stop or destroy the Vast instance. Closing the tunnel does not stop the remote process or billing. Check persistent disk charges as part of the shutdown procedure.

## Known limits

- Vast template image tags are mutable; the repository’s digest lock applies to the host-Docker server recipe, not a documented Vast template-image mechanism.
- A private Vast template is not a provider, region, retention, DPA, disk deletion, or legal-sovereignty guarantee.
- `--trust-remote-code` remains a model/server supply-chain decision.
- The shared Pi + OpenCode V1 route is keyless at the SGLang API layer; do not add server API authentication without a separately tested Pi-compatible profile.

## Sources

- [Vast template settings](https://docs.vast.ai/guides/templates/template-settings)
- [Vast Docker environment and networking](https://docs.vast.ai/guides/instances/docker-environment)
- [Vast SSH local port forwarding](https://docs.vast.ai/guides/instances/connect/ssh#ssh-local-port-forwarding)
- [Vast template API reference](https://docs.vast.ai/api-reference/creating-and-using-templates-with-api)
