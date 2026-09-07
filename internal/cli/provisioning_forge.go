package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type forgeMode uint8

const (
	forgeIntake forgeMode = iota
	forgeMeasured
)

type forgeCell struct {
	x, y     int
	glyph    rune
	priority float64
}

var forgeCells = buildForgeCells()

func buildForgeCells() []forgeCell {
	base := strings.Split(provisioningCube(0, -1, false, ""), "\n")
	cells := make([]forgeCell, 0, 180)
	for y, line := range base {
		for x, glyph := range []rune(line) {
			if glyph == ' ' {
				continue
			}
			dx, dy := math.Abs(float64(x-20))/20, math.Abs(float64(y-9))/9
			priority := .58 + math.Hypot(dx, dy)*.30
			if dx <= .50 && dy <= .55 {
				priority = .20 + math.Hypot(dx, dy)*.30
			}
			if dx <= .30 && dy <= .38 {
				priority = .02 + math.Hypot(dx, dy)*.24
			}
			if x == 20 && y == 9 {
				priority = 0
			}
			cells = append(cells, forgeCell{x: x, y: y, glyph: glyph, priority: priority})
		}
	}
	sort.SliceStable(cells, func(i, j int) bool {
		if cells[i].priority == cells[j].priority {
			if cells[i].y == cells[j].y {
				return cells[i].x < cells[j].x
			}
			return cells[i].y < cells[j].y
		}
		return cells[i].priority < cells[j].priority
	})
	return cells
}

// provisioningForge keeps the existing loader clock and real transfer ratio.
func provisioningForge(mode forgeMode, ratio float64, elapsed, wave time.Duration, color bool, marker string) string {
	return crystalFoundry(mode, ratio, elapsed, wave, color, marker)
}

// magneticPulseIntensity is a spatial wave: the main front moves from the
// cage toward the nucleus, while a provider-state change adds one short,
// independent inward ripple. Neither signal changes geometry.
func magneticPulseIntensity(distance float64, elapsed, activityAge time.Duration) float64 {
	waveAt := func(front float64) float64 {
		delta := distance - front
		if delta >= 0 {
			return math.Exp(-delta * 4.2)
		}
		return math.Exp(-math.Pow(delta/.13, 2))
	}
	cycle := math.Mod(max(0, elapsed.Seconds()), 2.8)
	main := 0.0
	if cycle < 2.3 {
		main = waveAt(1.35 - cycle/2.3*1.35)
	}
	ripple := 0.0
	if activityAge >= 0 && activityAge < 800*time.Millisecond {
		age := activityAge.Seconds() / .8
		ripple = waveAt(1.35-age*1.35) * (1 - age) * .72
	}
	return math.Max(0, math.Min(1, .07+main*.78+ripple))
}

func magneticPulseTone(intensity float64) string {
	base := [3]float64{35, 68, 80}
	high := [3]float64{199, 248, 255}
	intensity = math.Max(0, math.Min(1, intensity))
	return fmt.Sprintf("#%02X%02X%02X",
		int(base[0]+(high[0]-base[0])*intensity),
		int(base[1]+(high[1]-base[1])*intensity),
		int(base[2]+(high[2]-base[2])*intensity))
}

func forgeCompletionTone(age time.Duration, index int) string {
	if age < 0 || age >= 900*time.Millisecond {
		return "#8AE8CC"
	}
	phase := int(age/(90*time.Millisecond)) + index%4
	switch phase % 4 {
	case 0:
		return "#F1FFFF"
	case 1:
		return "#B9F4E8"
	default:
		return "#8AE8CC"
	}
}

func forgeFrontierTone(elapsed time.Duration, index int) string {
	phase := int(elapsed/(80*time.Millisecond)) + index
	switch phase % 5 {
	case 0:
		return "#F1FFFF"
	case 1, 4:
		return "#B9F4FF"
	default:
		return "#7DE7F4"
	}
}
