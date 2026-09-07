# Visual redesign — review and outstanding decisions

## Requested outcome

Review the complete TUI visual language, present multiple confirmation layouts
and ASCII animation proposals, and make long provisioning waits informative and
visually compelling. The approved Focus + Inspector dashboard remains part of
the application; a motif was explicitly excluded from that dashboard.

## Delivered design artifacts

The self-contained comparison hub lives at
`.superpowers/brainstorm/25724-1788628184/content/visual-review-hub.html`.
It links to two interactive HTML prototypes in the same directory:

- `visual-directions.html`: Crystal Foundry, Orbital Engine, Assembly Grid;
- `confirmation-layouts.html`: centered modal, decision/consequence columns,
  bottom drawer.

These are simulated design proposals, not implemented terminal components.
Script syntax was checked for all three files. A browser rendering check could
not be performed because the computer-use service reported no available browser.
File navigation works through relative local files or the companion's `/files/`
route. The hub corrects the animation-return action in its embedded comparison.

## Current application evidence

- `application_exit.go` renders exit and destroy choices as text blocks.
- `application_chrome.go` supplies a generic panel capped at 76 columns.
- `application_view.go` composes confirmation text and child forms separately.
- `provisioning_loader.go` has a fixed 41-by-19 motif and narrow text region.
  It already owns an 80ms animation clock, stage state, and measured bytes.

The redesign should unify these presentation paths while retaining the existing
action handlers, captured instance identity, and explicit destructive decision.

## Common requirements across proposed directions

- Group target, action, and consequences inside one bounded confirmation.
- Maintain a stable keyboard focus and always-visible action footer.
- Keep the surrounding workflow recognizable behind the dialog.
- Keep logs, last provider update, elapsed stage time, and measured download
  bytes legible during long waits.
- Distinguish indefinite activity from measured download progress.
- Stop decorative motion on errors and give the cause and actions priority.
- Reduce decoration before hiding essential facts on short/narrow terminals.
- Use static text in accessible/no-color contexts.

## Decisions still open

The user has not yet selected a new provisioning animation or confirmation
layout. Recommended pairing is Crystal Foundry with decision/consequence columns.
Do not treat an automatic goal continuation as approval of this recommendation.
After selection, validate the real terminal geometry and performance, then apply
the shared presentation consistently to paid creation, exit, destruction,
configuration replacement, and provider installation.

## Dashboard completion audit

The full suite on 2026-09-05 exposed two embedded-dashboard regressions: core
endpoint/model/instance facts are hidden at 80x24. Independent review also found
stale metrics crossing session boundaries, old current metrics surviving model
changes, and time-window expiry tied to the last server timestamp. These fixes
are required before installation; a passing dashboard-only suite is insufficient.
