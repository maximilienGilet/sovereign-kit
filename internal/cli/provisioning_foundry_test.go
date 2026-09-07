package cli

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"time"
)

func TestFoundrySolidVolumeStaysOneConnectedShape(t *testing.T) {
	for _, elapsed := range []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 7 * time.Second, 11 * time.Second} {
		frame := ansi.Strip(crystalFoundry(forgeIntake, 0, elapsed, -1, true, ""))
		lines := strings.Split(frame, "\n")
		if len(lines) != 19 {
			t.Fatalf("rotation changed canvas height: %d", len(lines))
		}
		on := func(x, y int) bool {
			cell := []rune(lines[y])[x]
			return cell >= 0x2801 && cell <= 0x28ff
		}
		if !on(20, 9) {
			t.Fatal("crystal left a hole at the nucleus")
		}
		// The filled crystal must be one volume: no detached blocks that read
		// as artifacts next to the silhouette.
		seen := make(map[[2]int]bool)
		stack := [][2]int{{20, 9}}
		seen[[2]int{20, 9}] = true
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := top[0], top[1]
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					nx, ny := x+dx, y+dy
					if (nx == x || ny == y) && nx >= 0 && nx < 41 && ny >= 0 && ny < 19 && !seen[[2]int{nx, ny}] && on(nx, ny) {
						seen[[2]int{nx, ny}] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
		}
		total := 0
		for y := range 19 {
			for x := range 41 {
				if on(x, y) {
					total++
				}
			}
		}
		if len(seen) != total {
			t.Fatalf("rotation %s left %d detached cells outside the main volume", elapsed, total-len(seen))
		}
	}
}

func TestFoundryCoreOccupiesMostOfItsFrameHeight(t *testing.T) {
	// Core must not look like a tiny pendant inside an oversized cage.
	frame := strings.Split(ansi.Strip(crystalFoundry(forgeIntake, 0, 0, -1, true, "")), "\n")
	occupiedRows := 0
	for _, row := range frame[2:17] {
		for _, r := range []rune(row)[15:26] {
			if r >= 0x2801 && r <= 0x28ff {
				occupiedRows++
				break
			}
		}
	}
	if occupiedRows < 15 {
		t.Fatalf("core too short within frame: %d rows", occupiedRows)
	}
}
