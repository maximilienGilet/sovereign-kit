package setup

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"strings"
	"time"
	"unicode"
)

// Activity is an observed provider snapshot, not a setup milestone.
type Activity struct {
	Stage           string
	InstanceID      int
	Status, Message string
	Detail          string
	CheckedAt       time.Time
}
type ActivityObserver interface{ InstanceActivity(Activity) }

func notifyActivity(operator Operator, instance vast.Instance, id int, at time.Time, token string) {
	if observer, ok := operator.(ActivityObserver); ok {
		clean := func(s string) string {
			if token != "" {
				s = strings.ReplaceAll(s, token, "[redacted]")
				if trimmed := strings.TrimSpace(token); trimmed != "" {
					s = strings.ReplaceAll(s, trimmed, "[redacted]")
				}
			}
			s = ansi.Strip(s)
			s = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, s)
			return ansi.Truncate(strings.TrimSpace(s), 500, "…")
		}
		detailLines := strings.Split(cleanServerLogs(instance.DaemonLogs, []string{token}), "\n")
		if len(detailLines) > 40 {
			detailLines = detailLines[len(detailLines)-40:]
		}
		observer.InstanceActivity(Activity{InstanceID: id, Status: clean(instance.Status), Message: clean(instance.StatusMessage), Detail: strings.Join(detailLines, "\n"), CheckedAt: at})
	}
}
