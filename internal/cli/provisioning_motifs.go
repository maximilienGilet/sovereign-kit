package cli

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"math"
	"strings"
	"time"
)

func (l provisioningLoader) motifKind() string {
	switch l.stageID {
	case setup.ProgressSearching, setup.ProgressModelSearch, setup.ProgressModelInspect:
		return "scan"
	case setup.ProgressCreating, setup.ProgressCreated:
		return "orbit"
	case setup.ProgressWaiting:
		message := strings.ToLower(l.providerMessage + " " + l.detailSnapshot)
		if strings.Contains(message, "pull") || strings.Contains(message, "download") || strings.Contains(message, "extract") || strings.Contains(message, "checksum") {
			return "intake"
		}
		return "orbit"
	case setup.ProgressHostKeys:
		return "link"
	case setup.ProgressLaunching:
		if l.serverPhase == "inspection" || l.serverPhase == "activation" {
			return l.serverPhase
		}
		if l.transferActive && l.transferTotal > 0 && l.transferCurrent >= 0 && l.transferCurrent <= l.transferTotal {
			return "measured"
		}
		if l.transferActive {
			return "intake"
		}
		return "activation"
	case setup.ProgressSaving:
		return "inspection"
	default:
		return "waiting"
	}
}

// All indeterminate motifs share a fixed canvas and clock. None accumulates
// material or estimates progress from elapsed time.
func stageMotif(kind string, elapsed time.Duration, color bool) string {
	if !color {
		elapsed = 0
	}
	seconds := elapsed.Seconds()
	var glyphs [19][41]rune
	var light [19][41]float64
	put := func(x, y int, r rune, v float64) {
		if x >= 0 && x < 41 && y >= 0 && y < 19 {
			glyphs[y][x] = r
			light[y][x] = math.Max(light[y][x], v)
		}
	}
	{
		rows := strings.Split(ansi.Strip(crystalFoundry(forgeIntake, 0, elapsed, -1, color, "")), "\n")
		for y, row := range rows {
			for x, r := range []rune(row) {
				if r == ' ' {
					continue
				}
				// A recognizable solid core persists throughout every indefinite
				// phase. Only lighting changes, never simulated completion.
				v := .48
				if x < 10 || x > 30 || y < 2 || y > 16 {
					v = .06
				} else {
					switch kind {
					case "scan", "inspection":
						v += .45 * math.Exp(-math.Pow((float64(y)-math.Mod(seconds*5, 23)+2)/1.8, 2))
					case "link":
						v += .4 * math.Exp(-math.Pow((math.Abs(float64(x-20))-7*(.5+.5*math.Cos(seconds*2)))/1.6, 2))
					case "intake":
						v += .4 * math.Exp(-math.Pow((float64(y)-16+math.Mod(seconds*5, 17))/2, 2))
					default:
						v += .18 * (1 + math.Sin(seconds*2-float64(y)*.3))
					}
				}
				put(x, y, r, v)
			}
		}
	}
	var out strings.Builder
	for y, row := range glyphs {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x, r := range row {
			if r == 0 {
				r = ' '
			}
			s := string(r)
			if color && r != ' ' {
				s = lipgloss.NewStyle().Foreground(lipgloss.Color(magneticPulseTone(light[y][x]))).Render(s)
			}
			out.WriteString(s)
		}
	}
	return out.String()
}
