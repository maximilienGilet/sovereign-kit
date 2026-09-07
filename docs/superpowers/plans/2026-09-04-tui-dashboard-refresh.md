# TUI dashboard and screen refresh

Approved direction: terminal-native Charm Bracelet application; endpoint-first
dashboard, coherent screen hierarchy, no fictitious telemetry or billing data,
no automatic integration installation, explicit remote shutdown confirmation.
User explicitly requested execution after approving the design in conversation.

## Implementation sequence

- [x] Dashboard: responsive activity/access/session sections and integration
  subpages, preserving keyboard actions and confirmation state machines.
  Ownership: internal/dashboardui. Add rendered-size, navigation and safety tests.
- [x] Common application shell: compact header/footer, bounded primary panels
  for home/resume/saved/replace/destroy/exit; error details before decoration.
  Ownership: internal/cli/application_view.go and application_chrome.go.
- [x] Motion: halve shimmer sweep/cycle duration without accelerating the cube
  or introducing additional timers. Preserve NO_COLOR and frozen error state.
- [x] Integration: wire only available session facts, retain worker cancellation
  and no paid mutations; run all tests and race checks.
- [x] Render fixtures at small/medium/large terminal sizes, review in one batch,
  correct material problems and install local CLI. No remote publication.

## Audit observations

The connected page is an unstyled integration list, not an operational dashboard.
Home/resume/saved/confirmation pages place sparse prose at the terminal origin.
Error screens reserve substantial height for the frozen cube ahead of the cause.
Application forms stretch to terminal width; child screens already have their
own responsive behavior and must not be cropped or have focus displaced.
Existing offer/recipe views carry the cyan/teal palette and useful evidence;
preserve their specialized navigation while unifying their surrounding frame.

## Verification contract

No new instance, remote shell or credentials needed. Tests use fake endpoints or
injected data and label previews as fixtures. Unknown measurements remain unknown.
Use failing tests before implementation; full `go test ./...` and focused race
tests before installing. Do not claim all underlying specialized screens rebuilt
if only their shared chrome changed.

## Delivered / verification

725 tests passed across 16 packages; focused race suite: 330 tests across four
packages. Review found and resolved an exit-error overflow (choices now stay
visible). Embedded dashboard tested with the actual root height budget.
Preview review used text-rendered fixtures, not screenshots of a live GPU session.

The dashboard polls optional llama.cpp metrics over the existing loopback route
with a two-second deadline and three-second interval, no redirects or proxy.
Missing measurements remain unavailable. GPU/region/hourly rate have an optional
view contract but are not filled from guesses: saved routes currently lack those
facts. Logs are the last provisioning snapshot, explicitly not a live stream.

Recipe/offer pickers retain their existing specialized designs; this pass fixes
name/badge priority and action labels, rather than replacing their internals.
Home, resume, saved, confirmations, errors, form sizing and the connected
dashboard receive the new shared hierarchy. Idle shutdown remains out of scope.
