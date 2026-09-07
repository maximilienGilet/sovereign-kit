# Provisioning Forge Animation Design

## Goal

Turn the provisioning illustration into an honest, continuous visual story:

- **Magnetic Pulse** communicates indeterminate container/image pull activity.
- **Crystal Forge** constructs the Sovereign Kit crystal from measured GGUF bytes.

The animation must feel alive without inventing progress, disturbing the timeline, or hiding actionable status and logs.

## Visual Narrative

### Magnetic Pulse — indeterminate pull

Before a trustworthy byte total exists, the left-hand motif shows the complete fixed dotted crystal silhouette around an empty nucleus. A broad sheet of light repeatedly travels from the outer cage toward the nucleus, brightening existing material without changing any glyph or cell position. The wave dissipates at the center, pauses briefly, and restarts from the cage. The interface never displays a percentage or an estimated completion time.

The wave has a soft leading edge and a longer fading trail, so adjacent crystal cells illuminate continuously rather than flashing independently. New provider activity emits one secondary, bounded inward ripple without reversing or teleporting the main wave. Recognized provider states such as `Downloading`, `Verifying Checksum`, and `Pull complete` may alter the highlight tone, but they do not advance a persistent fill. The most recent provider status remains visible in the textual activity area.

### Crystal Forge — measured model download

When a valid `SOVKIT_DOWNLOAD` event supplies `current >= 0` and `total > 0`, the motif switches to a construction state. The amount of visible, permanently built material is derived only from `current / total`, clamped to `[0, 1]`.

Construction proceeds in a deliberate order:

1. the central nucleus;
2. the suspended inner crystal and its faces;
3. the outer crystal cage and final edge highlights.

The boundary between built and unbuilt cells shimmers. Completed cells remain stable, so progress never appears to move backward when only the animation frame changes. The geometry is fixed at 41×19 terminal cells, using the existing Braille-based material and Sovereign Kit palette.

At 100%, the full crystal is visible and receives one bounded completion pulse. GGUF verification and server launch then reuse the completed crystal as a stable visual rather than reverting to the generic loader.

## Data and State

The animation consumes existing provisioning state; it does not create another source of truth.

- `transferActive && transferTotal > 0`: render Crystal Forge with a real ratio.
- download activity with an unknown or invalid total: render Magnetic Pulse and show bytes received with `total unknown`.
- provider/container loading before model telemetry: render Magnetic Pulse.
- later provisioning stages after a completed model download: retain the completed Crystal Forge.
- terminal success or error: freeze the current visual and apply the existing success or warning color treatment.

Invalid, regressing, or inconsistent counters never erase already rendered measured progress within one run. A new provisioning run resets the high-water mark. The setup parser remains responsible for rejecting malformed telemetry; the UI additionally clamps values before rendering.

## Layout and Copy

The motif replaces the current generic cube only while provisioning. The right-hand timeline remains the authoritative description of the current operation and completed steps.

For measured downloads, the textual detail remains:

`8.42 GiB / 14.03 GiB · 60.0%`

The current full-width Bubbles transfer bar is removed when the large Crystal Forge is visible because it duplicates the same measurement. Compact layouts retain a short textual meter when the motif cannot be displayed.

Responsive behavior:

- width ≥ 96 and height ≥ 19: motif on the left, timeline on the right;
- width 56–95: motif above the timeline only when enough height remains;
- width < 56 or height < 14: no large motif; show the compact spinner, status, bytes, and a small real meter only when the total is known.

The switch from Magnetic Pulse to Crystal Forge keeps the exact same 41×19 footprint, so the screen does not jump.

## Motion, Color, and Accessibility

The existing root-owned 80 ms clock remains the only animation clock. No additional timers or goroutines are introduced.

- normal color mode: cool cyan material, white construction frontier, mint completion pulse;
- error freeze: amber material;
- `NO_COLOR`: built cells use solid structural glyphs and unbuilt cells use sparse dots; meaning never depends on color;
- reduced/static behavior: geometry changes only when provider activity or measured progress changes; no continuous shimmer is required;
- accessible prompter: append status changes as text, including `Model download: 8.42 GiB / 14.03 GiB — 60.0%`, and never emit animation frames.

Animation must not cause line wrapping, terminal-width overflow, or geometry jitter.

## Component Boundaries

Introduce a focused renderer responsible only for forge geometry and material state. The provisioning loader chooses the visual mode and owns the monotonic progress high-water mark. Existing download parsing, activity extraction, timeline rendering, and frozen error presentation keep their current responsibilities.

The renderer accepts explicit inputs—mode, ratio, elapsed time, activity-change age, color capability, and terminal marker—and returns a fixed-size string. It does not inspect logs, mutate application state, or schedule work.

## Failure Handling

- Missing total: stay indeterminate and show no percentage.
- Current greater than total: treat the measurement as indeterminate rather than implying completion.
- Regressing valid current value: retain the highest ratio already shown for this provisioning run.
- Telemetry ends at verification/server launch: keep the completed crystal if 100% was observed.
- Error before measured download: freeze Magnetic Pulse in amber.
- Error during download: freeze the last honest construction state in amber.

## Testing

Automated tests cover:

- Magnetic Pulse changes color across animation frames while its stripped 41×19 glyph geometry remains byte-for-byte identical;
- the inward wave crosses adjacent spatial cells continuously and never teleports between construction-order indices;
- Magnetic Pulse never gains persistent fill or percentage;
- Crystal Forge has fixed 41×19 geometry at 0%, 25%, 60%, and 100%;
- the number of built cells is monotonic as the ratio increases;
- the same ratio has stable built cells across shimmer frames;
- regressing counters do not move construction backward;
- 100% transitions to a complete, stable crystal;
- success/error markers freeze animation and preserve geometry;
- `NO_COLOR` remains structurally legible;
- full, medium, compact, and accessible layouts do not overflow and do not duplicate the measured transfer bar;
- unknown totals never render a percentage;
- existing provider activity, provisioning timeline, and frozen error tests remain green.

No paid Vast instance is required for automated verification. A later manual smoke test may validate the animation against a real pull and GGUF download without changing its progress semantics.

## Out of Scope

- estimating container pull completion;
- deriving progress from Docker layer counts;
- download speed or ETA prediction;
- changing Vast provisioning, model download, or server launch behavior;
- changing the dashboard shown after provisioning.
