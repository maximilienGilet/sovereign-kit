# Recipe Picker TUI Design

**Date:** 2026-09-03

## Objective

Replace the plain workload `huh.Select` used during `sovkit setup` with a visual recipe picker that helps an operator decide whether one recipe fits their use case.

The picker presents one recipe at a time. The recipe list is navigation, not a comparison table. Every chart and claim describes only the active recipe and must have an auditable source.

## Design Principles

- Reuse the existing Charmbracelet stack: Bubble Tea for state and events, Bubbles for standard controls, Lip Gloss for layout and styles, and Huh for the remaining setup forms.
- Extend the existing `internal/catalogui` list/detail model instead of creating a second recipe-browser implementation.
- Do not rank recipes or compare them with each other.
- Do not derive a synthetic recommendation score.
- Render a quantitative chart only when its unit, source, and denominator are known.
- Keep exact values visible next to charts so the interface remains useful without color.
- Preserve a complete non-interactive and accessible fallback.

## Scope

This change covers the built-in recipe choice and the existing custom Hugging Face entry in the Vast setup flow.

It does not:

- change provider selection;
- search Vast offers before a recipe is selected;
- benchmark a recipe;
- claim unmeasured throughput or latency;
- compare recipe performance, cost, or capacity;
- change the deployment review that appears after an offer is selected;
- replace Huh in the rest of the setup flow.

## User Experience

### Wide and compact terminals

The picker opens in the alternate screen and follows the established Sovereign Kit TUI language.

The left side uses the existing Bubbles list to show recipe name, intended deployment, and evidence status. Moving the cursor changes the active recipe and immediately updates the detail panel. No metric from another recipe appears in that panel.

The detail panel contains:

1. the recipe name, evidence status, and short purpose;
2. a configured-context bar when the model's native context is known;
3. absolute server limits for output tokens and concurrent requests;
4. the strict accelerator, minimum VRAM, and minimum disk contract;
5. a concise “choose this recipe if” explanation derived from recipe metadata;
6. model, revision, runtime, and evidence details;
7. an explicit `NOT MEASURED` state for exact-profile throughput;
8. the action to use the active recipe.

The context bar represents:

```text
configured context window / native context window of the pinned model revision
```

It is rendered by the existing Bubbles progress component and accompanied by the exact numerator, denominator, percentage, and source label. Output, concurrency, VRAM, and disk remain absolute values because the recipe does not provide honest natural denominators for them.

The custom Hugging Face entry may have no native-context or hardware evidence. Its detail panel uses explicit unknown/unmeasured states and explains that inspection is the next step. It never renders an empty value as zero.

### Keyboard behavior

- `up`/`down` and `j`/`k`: change the active entry through the Bubbles list;
- `enter`: choose the active entry and exit the picker;
- `i`: toggle expanded evidence and technical details;
- `esc` or `q`: cancel without choosing a recipe;
- `ctrl+c`: cancel through Bubble Tea's normal quit path.

Filtering remains available in the standalone `sovkit catalog` browser. It is disabled in setup picker mode because the setup list is deliberately small and `enter` must have one unambiguous meaning.

### Terminal sizing

The picker responds to `tea.WindowSizeMsg`, using the existing project conventions:

- wide terminals show the recipe list and full detail dashboard side by side;
- compact terminals keep list and detail visible but shorten secondary copy and the progress axis;
- terminals below the rich-layout threshold show a minimal active-recipe summary with exact values and no decorative chart;
- every variant must fit the reported width and height without wrapping outside the viewport.

The implementation should reuse the sizing and Lip Gloss composition patterns already present in `internal/catalogui` and `internal/cli/rental_review.go`. It must not introduce a general-purpose layout engine.

### Accessible fallback

When `TERM=dumb` or `ACCESSIBLE` is set, `SelectWorkload` retains the current Huh selection flow. The labels include recipe name, evidence status, exact GPU, and configured context. The fallback returns the same stable recipe ID as the rich picker.

## Architecture

### Reusable catalog model

`internal/catalogui` remains the owner of recipe browsing and rendering. It gains two modes:

- browse mode for the existing `sovkit catalog` command;
- picker mode for `sovkit setup`.

The existing `catalogui.New` behavior remains browse-only. A picker constructor creates the same model with setup-specific key bindings and selection state. The model exposes the selected stable value and whether the user cancelled after the Bubble Tea program exits.

Both modes reuse:

- `bubbles/list` for entry navigation;
- `bubbles/progress` for the configured/native context bar;
- `bubbles/help` or the list key map for terminal-aware help text;
- Lip Gloss for borders, alignment, width calculation, joining panels, and the existing color language;
- Bubble Tea commands, messages, alternate-screen lifecycle, and resize events.

No custom cursor, list, progress bar, terminal event loop, or ANSI renderer is introduced.

### Entry view model

`catalogui.Entry` gains a stable selection value and typed fields required by the detail view. The view model retains display copy but no longer stores chartable numbers only as formatted strings.

The typed fields cover:

- recipe ID;
- configured context window;
- native model context window when known;
- maximum output tokens;
- maximum running requests;
- strict GPU model and count;
- minimum VRAM and disk;
- model repository and pinned revision;
- runtime summary;
- profile status, summary, and evidence.

`DefaultEntries` remains the adapter from bundled `recipe.Recipe` values to catalog entries. The custom Hugging Face entry uses its existing stable sentinel value and unknown fields.

### Recipe data

The recipe model gains an optional typed native-context field associated with the pinned model revision. A non-zero native context must be positive and greater than or equal to the configured context window. Bundled recipes that display the context ratio declare the verified native value in TOML.

Custom recipes may leave this value unknown. In that case, the picker shows the configured context as an absolute value and omits the progress visualization.

No throughput schema is added in this change. The absence of an exact benchmark remains explicit rather than being represented by invented zero values.

### Setup integration

`HuhPrompter.SelectWorkload` remains the setup-facing interface. Its interactive path runs the catalog picker with the prompter's injected input and output streams and Bubble Tea's alternate screen. The result is converted to the same recipe ID or custom Hugging Face sentinel currently consumed by `setupVast`.

The rest of the setup orchestration is unchanged. Provider selection, API key input, custom model inspection, offer selection, and paid-instance confirmation continue to use their current components.

## Data Flow

1. Setup loads the bundled recipes.
2. `SelectWorkload` maps them through the catalog entry adapter.
3. The catalog picker selects the first entry and renders its autonomous detail dashboard.
4. Bubbles list navigation changes the active entry.
5. The active entry's typed values drive its own progress and fact panels.
6. Pressing `enter` stores the active stable value and quits the Bubble Tea program.
7. `SelectWorkload` returns the stable value to the existing setup orchestration.
8. Setup either continues with the selected built-in recipe or enters the custom Hugging Face inspection flow.

No provider request or mutable external action occurs while browsing recipes.

## Errors and Unknown Data

- An empty recipe list renders a clear “no launchable recipes” state and cannot confirm.
- Unknown optional data renders as `UNKNOWN` or `NOT MEASURED`, never as zero.
- A native context smaller than the configured context is rejected during recipe validation.
- A Bubble Tea runtime error is returned through `SelectWorkload` with context.
- Cancellation returns the same cancellation semantics already used by the setup prompts.
- The picker does not recover from invalid bundled recipe data; built-in loading and validation remain the earlier boundary.

## Testing

### Recipe model

- parse the native-context field from bundled TOML;
- accept an omitted native context for custom recipes;
- reject a native context smaller than the configured context;
- confirm both bundled recipes expose the intended native context.

### Catalog model

- navigation updates the active detail panel without displaying metrics from another entry;
- picker `enter` returns the active stable value;
- `esc`, `q`, and `ctrl+c` cancel without selecting;
- `i` toggles evidence detail;
- browse mode retains filtering and does not gain picker-only behavior;
- known native context renders the exact ratio through Bubbles progress;
- unknown native context omits the progress chart and renders an explicit unknown state;
- unmeasured throughput is never displayed as a numeric claim;
- every rendered line and view fits representative minimal, compact, and wide terminal sizes.

### Setup integration

- an interactive picker selection follows the existing built-in recipe path;
- the custom sentinel follows the existing Hugging Face path;
- accessible and dumb-terminal modes retain the Huh selection flow;
- cancellation and Bubble Tea errors propagate without starting an offer search or deployment;
- injected input and output streams remain testable without a real terminal.

## Acceptance Criteria

- The setup recipe choice is a Charmbracelet-native TUI, not a custom rendering framework.
- The active screen contains no comparison with another recipe.
- The recipe list is navigation only.
- The only proportional chart uses a verified denominator belonging to the active recipe's pinned model.
- Exact values and evidence states remain readable without color.
- The picker returns the existing stable workload values, so setup orchestration behavior does not change.
- The standalone catalog continues to work in browse mode.
- Rich, compact, minimal, and accessible paths are covered by tests.
