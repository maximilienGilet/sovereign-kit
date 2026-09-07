package cli

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestProvisioningForgeKeepsFixedGeometry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  forgeMode
		ratio float64
	}{{"intake", forgeIntake, 0}, {"empty", forgeMeasured, 0}, {"partial", forgeMeasured, .6}, {"complete", forgeMeasured, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			frame := ansi.Strip(provisioningForge(tc.mode, tc.ratio, time.Second, time.Second, false, ""))
			lines := strings.Split(frame, "\n")
			if len(lines) != 19 {
				t.Fatalf("height=%d", len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != 41 {
					t.Fatalf("width=%d in %q", ansi.StringWidth(line), line)
				}
			}
		})
	}
}

func TestProvisioningForgeIntakeMovesWithoutClaimingProgress(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	a := provisioningForge(forgeIntake, 0, 400*time.Millisecond, time.Second, true, "")
	b := provisioningForge(forgeIntake, 0, 1200*time.Millisecond, time.Second, true, "")
	if a == b {
		t.Fatal("magnetic lighting did not move")
	}
	if ansi.Strip(a) == ansi.Strip(b) {
		t.Fatal("Crystal Foundry must rotate its geometry, not only its light")
	}
	for _, frame := range []string{a, b} {
		plain := ansi.Strip(frame)
		if strings.Contains(plain, "%") || strings.Contains(plain, "100") {
			t.Fatalf("indeterminate forge invented progress: %q", frame)
		}
	}
}

func TestFoundryDoesNotPunchHoleInNucleus(t *testing.T) {
	lines := strings.Split(provisioningForge(forgeIntake, 0, time.Second, time.Second, false, ""), "\n")
	if got := []rune(lines[9])[20]; got == ' ' {
		t.Fatal("renderer erased the center of the crystal")
	}
}

func TestMagneticPulseTravelsContinuouslyTowardCore(t *testing.T) {
	peak := func(elapsed time.Duration, activityAge time.Duration) float64 {
		bestDistance, bestLight := 0.0, -1.0
		for _, cell := range forgeCells {
			if cell.x == 20 && cell.y == 9 {
				continue
			}
			distance := math.Hypot(float64(cell.x-20)/20, float64(cell.y-9)/9)
			light := magneticPulseIntensity(distance, elapsed, activityAge)
			if light > bestLight {
				bestDistance, bestLight = distance, light
			}
		}
		return bestDistance
	}
	previous := peak(0, 2*time.Second)
	for elapsed := 80 * time.Millisecond; elapsed <= 2240*time.Millisecond; elapsed += 80 * time.Millisecond {
		current := peak(elapsed, 2*time.Second)
		if current > previous+.03 || previous-current > .20 {
			t.Fatalf("pulse jumped/reversed at %s: %.3f → %.3f", elapsed, previous, current)
		}
		previous = current
	}
}

func TestMagneticPulseProviderActivityAddsBoundedRipple(t *testing.T) {
	const elapsed = time.Second
	basePeak := func(age time.Duration) (float64, float64) {
		bestDistance, bestDelta := 0.0, -1.0
		for _, cell := range forgeCells {
			distance := math.Hypot(float64(cell.x-20)/20, float64(cell.y-9)/9)
			base := magneticPulseIntensity(distance, elapsed, -1)
			delta := magneticPulseIntensity(distance, elapsed, age) - base
			if delta > bestDelta {
				bestDistance, bestDelta = distance, delta
			}
		}
		return bestDistance, bestDelta
	}
	previous, _ := basePeak(0)
	for age := 80 * time.Millisecond; age < 800*time.Millisecond; age += 80 * time.Millisecond {
		current, delta := basePeak(age)
		if delta <= 0 || current > previous+.03 || previous-current > .30 {
			t.Fatalf("activity ripple jumped/reversed at %s: %.3f → %.3f (delta %.3f)", age, previous, current, delta)
		}
		previous = current
	}
	for _, age := range []time.Duration{-time.Second, 800 * time.Millisecond, 2 * time.Second} {
		for _, distance := range []float64{0, .4, .8, 1.2} {
			if got, want := magneticPulseIntensity(distance, elapsed, age), magneticPulseIntensity(distance, elapsed, -1); got != want {
				t.Fatalf("ripple did not settle at %s: got %.4f want %.4f", age, got, want)
			}
		}
	}
}

func TestProvisioningForgePlainIntakeIsStatic(t *testing.T) {
	a := provisioningForge(forgeIntake, 0, 400*time.Millisecond, 0, false, "")
	b := provisioningForge(forgeIntake, 0, 1200*time.Millisecond, time.Second, false, "")
	if a != b {
		t.Fatal("plain/reduced forge should update only from real state")
	}
}

func TestProvisioningForgeMeasuredConstructionIsMonotonicAndStable(t *testing.T) {
	previous := -1
	for _, ratio := range []float64{0, .25, .6, 1} {
		a := provisioningForge(forgeMeasured, ratio, 300*time.Millisecond, 0, false, "")
		b := provisioningForge(forgeMeasured, ratio, 1600*time.Millisecond, 0, false, "")
		if a != b {
			t.Fatalf("plain measured geometry changed with time at %.2f", ratio)
		}
		built := forgeBuiltCellCount(a)
		if built <= previous {
			t.Fatalf("construction did not advance at %.2f: %d <= %d", ratio, built, previous)
		}
		previous = built
	}
}

func TestProvisioningForgeMarkersFreezeCurrentGeometry(t *testing.T) {
	for _, mode := range []forgeMode{forgeIntake, forgeMeasured} {
		for _, marker := range []string{"!", "✓"} {
			a := provisioningForge(mode, .6, time.Second, 0, true, marker)
			b := provisioningForge(mode, .6, 3*time.Second, time.Second, true, marker)
			if a != b || !strings.Contains(ansi.Strip(a), marker) {
				t.Fatalf("mode %d marker %q did not freeze/persist", mode, marker)
			}
		}
	}
}

func TestProvisioningForgeNoColorDistinguishesBuiltAndPending(t *testing.T) {
	frame := provisioningForge(forgeMeasured, .5, time.Second, 0, false, "")
	if !strings.Contains(frame, "·") || forgeBuiltCellCount(frame) == 0 {
		t.Fatalf("plain construction lacks structural distinction: %q", frame)
	}
}

func TestProvisioningForgeCompletionPulseSettles(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	early := provisioningForge(forgeMeasured, 1, time.Second, 100*time.Millisecond, true, "")
	middle := provisioningForge(forgeMeasured, 1, time.Second, 350*time.Millisecond, true, "")
	settledA := provisioningForge(forgeMeasured, 1, time.Second, 2*time.Second, true, "")
	settledB := provisioningForge(forgeMeasured, 1, time.Second, 4*time.Second, true, "")
	if early == middle || settledA != settledB || ansi.Strip(early) != ansi.Strip(settledA) {
		t.Fatal("completion must pulse briefly without moving geometry, then settle")
	}
}

func forgeBuiltCellCount(frame string) int {
	count := 0
	for _, r := range ansi.Strip(frame) {
		if r != ' ' && r != '\n' && r != '·' {
			count++
		}
	}
	return count
}

func TestFoundryRotationAlwaysFitsItsCanvas(t *testing.T) {
	for frame := 0; frame < 225; frame++ {
		view := provisioningForge(forgeMeasured, .65, time.Duration(frame)*80*time.Millisecond, -1, true, "")
		lines := strings.Split(ansi.Strip(view), "\n")
		if len(lines) != 19 {
			t.Fatal("rotation changed canvas height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) != 41 {
				t.Fatal("rotation overflow")
			}
		}
	}
}
