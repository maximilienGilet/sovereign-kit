package cli

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Fixed isometric geometry avoids diagonal jitter in a terminal. Only edge
// lighting travels; the inner suspended cube breathes through color.
func provisioningCube(elapsed, wave time.Duration, color bool, marker string) string {
	const width, height = 41, 19
	type cell struct {
		glyph rune
		tone  string
	}
	var canvas [height][width]cell
	type point struct{ x, y float64 }
	vertices := []point{{20, 1}, {38, 6}, {20, 11}, {2, 6}, {2, 12}, {20, 17}, {38, 12}}
	edges := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {3, 4}, {4, 5}, {5, 6}, {6, 1}, {2, 5}}
	// Freeze terminal states before calculating any material lighting. The
	// animation is a liveness signal, never a proxy for provider progress.
	if marker == "!" || marker == "✓" || !color {
		elapsed, wave = 0, -1
	}
	seconds := elapsed.Seconds()
	phase := math.Mod(seconds/5.6, 1)
	material := func(x, y, depth float64, core bool) string {
		base := [3]float64{23, 71, 96}
		high := [3]float64{204, 249, 255}
		// A diagonal sheet of light crosses the entire object: shared spatial
		// coordinates keep highlights continuous through cage intersections.
		position := (x/41*.62 + y/19*.38)
		distance := math.Mod(phase-position+1, 1)
		if distance > .5 {
			distance--
		}
		trail := math.Exp(-distance * 15)
		if distance < 0 {
			// Soft leading edge; a longer exponential wake cools behind it.
			trail = math.Exp(-math.Pow(distance/.06, 2))
		}
		heart := .5 - .5*math.Cos(seconds*2*math.Pi/3.8)
		light := .12 + depth*.12 + trail*.76
		if core {
			base = [3]float64{19, 99, 133}
			light = .12 + depth*.24 + heart*.20 + trail*.30
		}
		// A bounded radial pulse travels from the core to the cage only when
		// an actual setup stage changes, then settles without a hard flash.
		if wave >= 0 && wave < 800*time.Millisecond {
			radius := math.Hypot((x-20)/18, (y-9)/8)
			front := wave.Seconds() / .8 * 1.7
			pulse := math.Exp(-math.Pow((radius-front)/.20, 2))
			light += pulse * .55 * (1 - wave.Seconds()/.8)
		}
		if marker == "!" {
			base, high, light = [3]float64{79, 52, 30}, [3]float64{214, 170, 104}, .25+depth*.25
		}
		if marker == "✓" {
			base, high, light = [3]float64{24, 77, 74}, [3]float64{151, 229, 209}, .30+depth*.25
		}
		light = math.Max(0, math.Min(1, light))
		return fmt.Sprintf("#%02X%02X%02X", int(base[0]+(high[0]-base[0])*light), int(base[1]+(high[1]-base[1])*light), int(base[2]+(high[2]-base[2])*light))
	}
	put := func(x, y int, r rune, tone string) {
		if x >= 0 && x < width && y >= 0 && y < height {
			canvas[y][x] = cell{r, tone}
		}
	}
	// Braille supplies a 2×4 sub-cell grid for clean, continuous diagonals.
	dot := func(x, y int, tone string) {
		if x < 0 || x >= width*2 || y < 0 || y >= height*4 {
			return
		}
		bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
		c := &canvas[y/4][x/2]
		c.glyph = 0x2800 | c.glyph | bits[y%4][x%2]
		c.tone = tone
	}
	// Fine, fixed stippling gives the suspended core three material faces
	// without turning the cage into a solid block or changing its silhouette.
	for face, indices := range [][4]int{{0, 1, 2, 3}, {3, 2, 5, 4}, {2, 1, 6, 5}} {
		a, b, d := vertices[indices[0]], vertices[indices[1]], vertices[indices[3]]
		for u := 1; u < 12; u++ {
			for v := 1; v < 8; v++ {
				x := 20 + (a.x+(b.x-a.x)*float64(u)/12+(d.x-a.x)*float64(v)/8-20)*.43
				y := 9 + (a.y+(b.y-a.y)*float64(u)/12+(d.y-a.y)*float64(v)/8-9)*.43
				dot(int(math.Round(x*2)), int(math.Round(y*4)), material(x, y, []float64{.85, .15, .45}[face], true))
			}
		}
	}
	draw := func(scale float64, inner bool) {
		for edge, pair := range edges {
			a, b := vertices[pair[0]], vertices[pair[1]]
			a = point{20 + (a.x-20)*scale, 9 + (a.y-9)*scale}
			b = point{20 + (b.x-20)*scale, 9 + (b.y-9)*scale}
			steps := int(math.Ceil(math.Max(math.Abs(b.x-a.x)*2, math.Abs(b.y-a.y)*4) * 2))
			for i := 0; i <= steps; i++ {
				f := float64(i) / float64(steps)
				x, y := a.x+(b.x-a.x)*f, a.y+(b.y-a.y)*f
				depth := .6
				if edge < 4 {
					depth = 1
				}
				if scale == .94 {
					depth *= .35
				}
				dot(int(math.Round(x*2)), int(math.Round(y*4)), material(x, y, depth, inner))
			}
		}
	}
	draw(1, false)
	draw(.94, false)
	draw(.43, true)
	center, tone := '◇', "#B9F4FF"
	if marker != "" {
		center = []rune(marker)[0]
		tone = "#F2BE65"
		if marker == "✓" {
			tone = "#8AE8CC"
		}
	}
	put(20, 9, center, tone)
	lines := make([]string, height)
	for y, row := range canvas {
		var b strings.Builder
		for _, c := range row {
			if c.glyph == 0 {
				b.WriteByte(' ')
				continue
			}
			value := string(c.glyph)
			if color {
				value = lipgloss.NewStyle().Foreground(lipgloss.Color(c.tone)).Render(value)
			}
			b.WriteString(value)
		}
		lines[y] = b.String()
	}
	return strings.Join(lines, "\n")
}
