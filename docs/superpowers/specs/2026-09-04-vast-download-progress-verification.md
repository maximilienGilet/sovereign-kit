# Vast download progress — verification

Checked 2026-09-04. Scope: public Vast documentation and official `vast-ai/vast-python` client (now redirected to `vast-ai/vast-cli`), with Docker documentation for comparison. No rental, authenticated log request, or instance mutation performed.

## Verified findings

**MEDIUM confidence: no documented structured image-pull percentage was found in the inspected Vast interfaces.** Instance details expose `actual_status`, `intended_status`, `cur_state`, `next_state`, and nullable `status_msg`; the published response does not define Docker layer IDs or downloaded/total-byte fields. This is a bounded documentation finding, not proof that private/internal interfaces cannot provide more. [Show instance API](https://docs.vast.ai/api-reference/instances/show-instance).

**MEDIUM confidence: additional logs are an available API capability.** Send `PUT https://console.vast.ai/api/v0/instances/request_logs/{id}` with Bearer authorization. Optional body fields: `tail` (string line count), `filter` (grep string), and `daemon_logs: "true"` to select daemon system logs instead of container logs. The documented response is `{success, result_url, msg}`; retrieve the log text from `result_url` after the asynchronous upload. It is not documented as inline logs or a structured progress stream. [Show logs API](https://docs.vast.ai/api-reference/instances/show-logs).

The inspected official legacy CLI implementation uses the same endpoint with a trailing slash, then polls `result_url` up to 30 times, waiting 0.3 seconds between attempts, and prints the first HTTP-200 text result. It contains no `websocket` or `progressDetail` match. This implementation corroborates snapshot retrieval, but its retry timing is client behavior, not an API SLA. The repository now identifies `vast.py` as deprecated. [Official CLI implementation](https://raw.githubusercontent.com/vast-ai/vast-python/master/vast.py), [current repository](https://github.com/vast-ai/vast-cli).

## Separate the stages

| Stage | Evidence / usable signal | Percentage assessment |
| --- | --- | --- |
| Initial Vast image pull, before instance SSH | Vast defines instance `Loading` as Docker-image download; `Connecting` means Docker is running but connection is not verified. Poll instance state and optionally request provider logs. | Unknown total: keep indeterminate unless real log evidence supplies complete, interpretable byte counters. |
| Model download inside the running container | This is separate from the host image pull. Vast serverless explicitly distinguishes `LOADING` (Docker pull/run) from `MODEL_LOADING`. Application download instrumentation or application logs are the appropriate source. | A downloader we control can report bytes and use a valid known total; this is not a Vast instance-progress field. |
| Docker pull on a machine whose Docker daemon we control | Docker Engine `POST /images/create?fromImage=…` emits JSON with `status`, `id`, and `progressDetail.current/total` during downloads. | Per-layer download progress is possible. It is not automatically total provisioning progress. |

Sources: [Vast instance states](https://docs.vast.ai/guides/instances/manage-instances), [serverless worker states](https://docs.vast.ai/guides/serverless/worker-states), [Docker Engine pull example](https://docs.docker.com/reference/api/engine/sdk/examples/#pull-an-image).

## Practical implications (engineering judgment)

- Preserve the honest indeterminate Docker-pull stage and elapsed time. Supplement `status_msg` with bounded, sanitized log snapshots if authorized; log unavailability should not fail provisioning.
- Do not promise that `daemon_logs` always contains pull byte counts: the reference specifies log categories, not their contents or availability during initial loading. Verify this with a real loading instance before designing a parser.
- Do not infer access to the provider's Docker socket from SSH into the rented container. The reviewed Vast interfaces do not expose the Docker Engine pull stream above.
- Keep model-download progress separate. With an application-controlled downloader, report received bytes; show a percentage only when the transfer's total is established and matches what is being counted. Missing totals should remain indeterminate.
- If layer logs become available, retain phase/status separately from counters; do not equate download completion with extraction, container startup, SSH readiness, or model readiness.

## Confidence, source evaluation, and limitations

Overall **MEDIUM**: official API reference and first-party CLI agree on the log retrieval protocol; official state documentation separates the stages. More than three primary pages were checked, but they are mostly one vendor, not three independent authorities. Docker independently verifies its own interface only.

No public WebSocket contract for initial instance image-pull progress was found in the documentation index, targeted official-doc searches, or inspected legacy CLI. Undocumented console behavior and the newer packaged CLI were not exhaustively inspected. Exact log contents, refresh cadence, access during `loading`, byte-total completeness, and parser reliability remain unverified. Application-specific Hugging Face download behavior was not researched here.
