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
5. a concise “choose this recipe if” explanation sourced from structured recipe metadata;
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
- `i`: open an expanded evidence and technical-detail viewport;
- `up`/`down`, `j`/`k`, `pgup`/`pgdown` while that viewport is open: scroll it;
- `i`, `esc`, or `q` while that viewport is open: close it and return to the recipe list;
- `esc` or `q`: cancel without choosing a recipe;
- `ctrl+c`: cancel through Bubble Tea's normal quit path.

Filtering remains available in the standalone `sovkit catalog` browser. It is disabled in setup picker mode because the setup list is deliberately small and `enter` must have one unambiguous meaning.

### Terminal sizing

The picker responds to `tea.WindowSizeMsg` and uses the breakpoints already established by the deployment review:

- terminals smaller than 72 columns or 24 lines use the minimal view: active recipe name, exact limits, hardware contract, and selection controls, with no proportional chart;
- terminals from 72 through 133 columns use the compact view: recipe list and shortened detail panel side by side, with a shortened progress bar and no expanded inline evidence;
- terminals at least 134 columns wide and 30 lines high use the wide view: recipe list and full detail dashboard side by side;
- a terminal at least 134 columns wide but fewer than 30 lines high remains compact;
- every variant must fit the reported width and height without wrapping outside the viewport.

The implementation should reuse the sizing and Lip Gloss composition patterns already present in `internal/catalogui` and `internal/cli/rental_review.go`. It must not introduce a general-purpose layout engine.

### Accessible fallback

When `TERM=dumb` or `ACCESSIBLE` is set, `SelectWorkload` retains the current Huh selection flow. Huh already enables accessible mode for `TERM=dumb`; the `ACCESSIBLE` branch must explicitly configure the form with `WithAccessible(true)`. The labels include recipe name, evidence status, exact GPU, and configured context. The fallback returns the same stable recipe ID as the rich picker.

## Architecture

### Reusable catalog model

`internal/catalogui` remains the owner of recipe browsing and rendering. It gains two modes:

- browse mode for the existing `sovkit catalog` command;
- picker mode for `sovkit setup`.

The existing `catalogui.New` behavior remains browse-only. A picker constructor creates the same model with setup-specific key bindings and selection state. The model exposes the selected stable value and whether the user cancelled after the Bubble Tea program exits.

Both modes reuse:

- `bubbles/list` for entry navigation;
- `bubbles/progress` for the configured/native context bar;
- the Bubbles help component driven by the picker's explicit key map for terminal-aware help text;
- `bubbles/viewport` for the expanded evidence and technical-detail screen;
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
- profile status, summary, structured `use_when` guidance, and evidence.

`DefaultEntries` remains the adapter from bundled `recipe.Recipe` values to catalog entries in browse mode. Picker callers supply every stable selection value. `SelectWorkload` appends the custom Hugging Face entry with the existing `customHuggingFaceWorkload` sentinel, so `catalogui` neither imports `internal/cli` nor duplicates that constant.

### Recipe data

The recipe model gains:

- `model.native_context_window`, an optional typed native-context field associated with the pinned model revision;
- `profile.use_when`, a list of short operator-facing conditions that supply the decision guidance without recipe-specific UI branches.

A non-zero native context must be positive and greater than or equal to the configured context window. Bundled recipes that display the context ratio declare the verified native value in TOML. Bundled recipes declare at least one non-empty `use_when` item. Custom recipes may omit decision guidance when inspection is the required next step.

Custom recipes may leave the native-context value unknown. In that case, the picker shows the configured context as an absolute value and omits the progress visualization.

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
- parse and validate structured `profile.use_when` guidance;
- accept an omitted native context for custom recipes;
- reject a native context smaller than the configured context;
- confirm both bundled recipes expose the intended native context.

### Catalog model

- navigation updates the active detail panel without displaying metrics from another entry;
- picker `enter` returns the active stable value;
- `esc`, `q`, and `ctrl+c` cancel without selecting;
- `i` opens a Bubbles viewport, its navigation scrolls without changing the recipe, and its close keys return focus to the list;
- browse mode retains filtering and does not gain picker-only behavior;
- known native context renders the exact ratio through Bubbles progress;
- unknown native context omits the progress chart and renders an explicit unknown state;
- unmeasured throughput is never displayed as a numeric claim;
- every rendered line and view fits representative minimal, compact, and wide terminal sizes.

### Setup integration

- an interactive picker selection follows the existing built-in recipe path;
- the custom sentinel follows the existing Hugging Face path;
- accessible and dumb-terminal modes retain the Huh selection flow, with Huh explicitly placed in accessible mode for `ACCESSIBLE`;
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
