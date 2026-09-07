# Live throughput dashboard design

## Approved direction

The connected endpoint dashboard uses the **Focus + Inspector** layout. Its
first glance answers whether the private model is healthy and working; its
second glance explains current and recent performance. The ten-minute
generation graph is the visual focus. This work does not redesign the separate
provisioning ASCII animation.

## Information hierarchy

On wide terminals, render:

1. a status strip containing tunnel/model health, active requests, and queued
   requests;
2. a large generation-velocity chart on the left;
3. a compact inspector on the right containing prompt throughput, KV usage,
   configured context, configured output, generation average, and generation
   peak;
4. session and instance facts below the performance area;
5. the copyable endpoint, model identifier, and integration actions last.

Each fact has one authoritative location. In particular, current generation
throughput, active/queued requests, context/output limits, and KV usage must not
be repeated in multiple cards. The existing connection and instance actions
remain available.

On medium or narrow terminals, stack the same sections in that order. Reduce
the chart before removing essential status or endpoint information. When there
is insufficient room for a useful chart, show the current, average, and peak
values textually.

## Sampling and history

Continue reading the existing local `/metrics` endpoint through the established
three-second polling loop. Keep at most 200 timestamped samples, which represents
ten minutes at that cadence and gives the history a constant memory bound.

Each sample independently records the metrics actually reported by the server.
A missing generation value is missing data, not zero. A reported numeric zero is
a real idle measurement and may bring the line back to the baseline. Average and
peak use only reported generation values within the live ten-minute window.

Discard samples older than ten minutes even if polling was delayed. Clear the
history when the endpoint identity changes or a disconnected tunnel begins a new
connected session. Resizing the terminal only rerenders existing samples and
must not discard history.

## Chart rendering

Render a fixed-geometry Braille chart using Bubble Tea's existing update loop and
Lip Gloss styles. The chart component accepts timestamped optional values plus a
target width and height; it has no knowledge of Vast, SSH, endpoints, or polling.

The x-axis always represents the most recent ten minutes and labels `-10m`,
`-5m`, and `now`. Empty time before the first sample remains blank. Missing
samples create discontinuities rather than interpolated measurements.

The y-axis starts at zero and uses a stable rounded ceiling derived from the
visible values. The ceiling may grow immediately when a new peak arrives and
should shrink only after that peak leaves the ten-minute window, preventing the
chart from visually jumping between ordinary polls. A completely reported zero
window retains a visible zero baseline.

Generation uses the existing cyan accent, axes and old samples use subdued blue,
and text uses the current dashboard palette. While at least one request is active,
the newest reported point may receive a bounded brightness pulse. Its glyph and
position do not move. No animation, interpolation, or synthetic value implies
unmeasured throughput.

## States and failures

- **Healthy with data:** render the live chart and inspector normally.
- **Healthy before enough history exists:** render available points against the
  fixed ten-minute time domain, leaving older time blank.
- **Server reports zero:** render the real baseline and `0 tok/s`.
- **Metric absent for a poll:** retain prior history, add a missing sample, show
  `not reported` for the current value, and leave a gap in the chart.
- **Telemetry interrupted:** retain history in muted colors and show
  `Telemetry interrupted`; do not append fabricated values.
- **Tunnel disconnected:** stop adding samples and show the existing explicit
  disconnected state.
- **Reconnected session:** begin a fresh chart so unrelated sessions are not
  presented as one continuous run.

The health label continues to come from endpoint/tunnel verification, never from
throughput alone. A quiet model can be healthy at zero throughput.

## Component boundaries

Add a bounded throughput-history unit responsible for timestamp retention,
window pruning, reported-value statistics, and session reset. Add a chart unit
responsible only for converting a history snapshot into terminal cells. The
dashboard model owns both units and feeds them from existing stats results.

The current endpoint statistics reader remains the source of truth and requires
no new remote API. No new background goroutine, polling interval, public port, or
Vast operation is introduced.

## Verification

Automated tests cover:

- the 200-sample and ten-minute bounds;
- delayed polls and timestamp pruning;
- missing values versus reported zero;
- average and peak over reported values only;
- stable scaling while a peak remains visible;
- chart discontinuities for missing samples;
- geometry stability across animation frames;
- narrow and wide responsive layouts;
- history preservation during resize;
- history reset after endpoint/session changes;
- telemetry interruption and tunnel disconnection;
- absence of duplicated facts in Focus + Inspector.

Run the complete Go suite, dashboard-focused tests, race checks for the dashboard
and endpoint-stat packages, and static analysis. Tests use simulated local metrics
responses only and must not mutate Vast instances or user integration profiles.

## Out of scope

- Redesigning Crystal Forge, Magnetic Pulse, or any provisioning ASCII motif.
- Changing the three-second poll cadence.
- Persisting telemetry between CLI runs.
- Adding comparative recipe or offer metrics.
- Claiming latency, throughput, or health when the server did not report it.
