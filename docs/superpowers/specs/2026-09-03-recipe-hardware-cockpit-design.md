# Recipe Hardware Cockpit

**Date:** 2026-09-03

## Approved direction

Evolve the setup recipe picker into a graphic hardware cockpit, using only
the existing Charmbracelet stack. The user approved this direction after
the sidebar sizing fix and the Bubbles context animation.

This document supersedes the main-dashboard presentation in
`2026-09-03-recipe-picker-tui-design.md`. Its data-truthfulness, selection,
accessibility, and no-comparison requirements remain unchanged.

## Composition

The navigation sidebar remains a Bubbles list of names, statuses, and
purposes. It keeps its responsive width and consistent selected-item inset.
Only the active recipe supplies metrics to the cockpit.

The main area contains:

1. Recipe name, a textual evidence-status badge, and its purpose.
2. Three prominent figures: configured context, maximum output tokens,
   and maximum simultaneous requests. Each figure has its unit and qualifier.
   Lip Gloss emphasis, contrast, padding, and alignment create hierarchy;
   no custom bitmap font or terminal font-size protocol is needed.
3. A capacity zone with a graduated context ruler and configured concurrency
   slots.
4. A hardware zone with a stylized GPU enclosure, exact GPU model, GPU count,
   strict/preferred contract, minimum VRAM, minimum disk, and use-case guidance.
5. A visible `THROUGHPUT: NOT MEASURED` statement, an `i` evidence hint,
   and the existing choose/inspect action.

In wide mode the capacity and hardware zones appear side by side. The hero
figures span both zones. On extremely wide terminals the content receives a
bounded reading width instead of stretching a single context bar across the
entire screen. Extra space separates groups rather than introducing fake data.

## Honest visual encodings

### Context ruler

- Keep the existing animated Bubbles progress bar.
- The denominator is the active recipe's known native context.
- Show the exact configured/native values and percentage immediately.
- Add evenly spaced scale ticks, endpoints, and a marker for the configured
  limit. The marker follows the target, not the animation's intermediate value.
- Label the source as the pinned model configuration only when the recipe
  provides that provenance; otherwise say that the source is unknown.
- Omit the proportional graphic when native context is unknown. Display
  the configured absolute value and `NATIVE CONTEXT UNKNOWN` instead.

### Concurrency slots

- Draw one outlined slot per configured concurrent request for small counts.
- Label the row `CONFIGURED CAPACITY`, with the exact maximum request count.
- Slots are not active connections, occupancy, availability, or a usage gauge.
- Bound the rendered slot count by available width; large counts use a few
  slots plus an explicit remaining-count label and the exact total.
- Unknown concurrency shows `UNKNOWN`, not an empty or zero-capacity diagram.

### Hardware enclosure

- Use Lip Gloss borders and layout to suggest a physical accelerator module.
- Show the exact model, count, strict/preferred requirement, minimum VRAM,
  and disk requirement as absolute values.
- Do not show utilization, memory allocation, thermal readings, or a percentage.
- Do not multiply minimum VRAM by GPU count: preserve the existing recipe
  requirement's meaning.
- The custom Hugging Face entry shows an inspection-first state without
  fabricating a GPU model, limits, or native-context evidence.

## Visual identity and motion

- Use a small restrained accent palette, with stable identity per recipe.
- Status badges retain explicit words; identity color never means quality,
  recommendation, readiness, or measured performance.
- Apply the active accent consistently to the hero, ruler, GPU enclosure,
  selection accent, and action.
- Retain the existing short, critically damped Bubbles progress transition.
- Recipe name, status, exact numbers, marker, and action update immediately.
- No looping decorative motion, sliding panels, delayed input, or animated
  counters that temporarily present false values.

## Responsive and accessible behavior

- Below 72 columns or 24 rows: retain the minimal text presentation.
- At least 72 columns and 24 rows: retain sidebar navigation and a compact
  single-column detail presentation with exact limits and a context graphic
  only when it fits.
- At least 134 columns and 30 rows: show the cockpit, with two zones when
  their measured inner widths permit it. Reduce spacing before removing detail.
- Reserve space for the action and keyboard footer before sizing content.
- Technical provenance remains fully available through the existing `i`
  viewport rather than being repeated as a large inline audit table.
- Huh remains the unchanged accessible/`TERM=dumb` selection path, without
  decorative graphics or animation.
- All information remains understandable without color.

## Implementation boundaries

- Keep `internal/catalogui/model.go` responsible for shared Bubbles state.
- Keep `internal/catalogui/picker.go` responsible for picker interaction,
  resize coordination, and selection lifecycle.
- Add a focused cockpit renderer in `internal/catalogui/cockpit.go` for the
  hero figures, capacity ruler, concurrency slots, GPU enclosure, and accents.
- Reuse Bubbles list, progress, help, key bindings, and viewport; compose with
  Lip Gloss and the installed Charm ANSI width/wrapping helpers.
- Do not introduce a new UI framework, plotting engine, dependencies, recipe
  metrics, provider queries, or deployment behavior.
- Preserve the standalone catalog and unrelated working-tree changes.

## Verification and acceptance

- Assert that hero values and all diagrams describe only the active entry.
- Verify exact ruler endpoints and target marker for 12.5% and 100% context.
- Verify one and five configured slots, unknown counts, and a bounded large count.
- Verify strict, non-strict, and unknown hardware contracts without live metrics.
- Check the custom inspection-first state and complete evidence navigation.
- Test every builtin at 30x10, 72x24, 108x30, 134x30, 150x34, and 320x80.
- Check rendered bounds, stable panel edges, visible action/footer, and
  selection while the context animation is running.
- Inspect actual terminal text renders for composition, not only dimensions.
- Run the full Go test suite, `go vet ./...`, and `git diff --check`.

## Out of scope

Recipe comparison, radar scores, benchmark invention, offer prices, provider
search, live telemetry, new keyboard workflows, and changes to paid-instance
confirmation are excluded.
