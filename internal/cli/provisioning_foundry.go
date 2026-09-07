package cli

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Facets are filled solid and depth-buffered so rear faces stay behind the
// front ones; facet edges carry the structure, no surrounding scaffolding.
func crystalFoundry(mode forgeMode, ratio float64, elapsed, wave time.Duration, color bool, marker string) string {
	if !color || marker != "" {
		elapsed, wave = 0, -1
	}
	if math.IsNaN(ratio) {
		ratio = 0
	}
	ratio = math.Max(0, math.Min(1, ratio))
	type point struct{ x, y, z float64 }
	type pixel struct {
		depth, light        float64
		occupied, on, built bool
	}
	var pixels [76][82]pixel
	angle := elapsed.Seconds()*2*math.Pi/18 + .35
	cs, sn := math.Cos(angle), math.Sin(angle)
	project := func(p point) point {
		x, z := p.x*cs+p.z*sn, -p.x*sn+p.z*cs
		return point{40 + x*28, 37 - p.y*28 + z*9, z + p.y*9/28}
	}
	top, bottom := point{0, 1.05, 0}, point{0, -1.05, 0}
	// Uneven facet radii break the six-fold symmetry so every rotation
	// angle is its own frame instead of repeating after 60 degrees.
	radii := [6]float64{.74, .63, .71, .66, .75, .62}
	var ring [6]point
	for i := range ring {
		a := float64(i) * math.Pi / 3
		ring[i] = point{radii[i] * math.Cos(a), .12, radii[i] * math.Sin(a)}
	}
	for i, a := range ring {
		b := ring[(i+1)%6]
		for _, tip := range []point{top, bottom} {
			pa, pb, pc := project(a), project(b), project(tip)
			// Outward normal, oriented using the face centroid.
			u, v := point{b.x - a.x, b.y - a.y, b.z - a.z}, point{tip.x - a.x, tip.y - a.y, tip.z - a.z}
			n := point{u.y*v.z - u.z*v.y, u.z*v.x - u.x*v.z, u.x*v.y - u.y*v.x}
			center := point{(a.x + b.x + tip.x) / 3, (a.y + b.y + tip.y) / 3, (a.z + b.z + tip.z) / 3}
			if n.x*center.x+n.y*center.y+n.z*center.z < 0 {
				n = point{-n.x, -n.y, -n.z}
			}
			nx, nz := n.x*cs+n.z*sn, -n.x*sn+n.z*cs
			if nz+n.y*9/28 >= 0 {
				continue
			} // camera looks toward increasing depth
			length := math.Sqrt(nx*nx + n.y*n.y + nz*nz)
			lighting := (-nx*.6 + n.y*.4 - nz*.7) / length
			faceTone := .34
			if lighting > .2 {
				faceTone = .58
			}
			if lighting > .65 {
				faceTone = .82
			}
			denom := (pb.y-pc.y)*(pa.x-pc.x) + (pc.x-pb.x)*(pa.y-pc.y)
			if math.Abs(denom) < .0001 {
				continue
			}
			minX, maxX := max(0, int(math.Floor(math.Min(pa.x, math.Min(pb.x, pc.x))))), min(81, int(math.Ceil(math.Max(pa.x, math.Max(pb.x, pc.x)))))
			minY, maxY := max(0, int(math.Floor(math.Min(pa.y, math.Min(pb.y, pc.y))))), min(75, int(math.Ceil(math.Max(pa.y, math.Max(pb.y, pc.y)))))
			edgeDistance := func(x, y float64, p, q point) float64 {
				return math.Abs((q.y-p.y)*x-(q.x-p.x)*y+q.x*p.y-q.y*p.x) / math.Max(.001, math.Hypot(q.y-p.y, q.x-p.x))
			}
			for y := minY; y <= maxY; y++ {
				for x := minX; x <= maxX; x++ {
					fx, fy := float64(x), float64(y)
					wa := ((pb.y-pc.y)*(fx-pc.x) + (pc.x-pb.x)*(fy-pc.y)) / denom
					wb := ((pc.y-pa.y)*(fx-pc.x) + (pa.x-pc.x)*(fy-pc.y)) / denom
					wc := 1 - wa - wb
					if wa < 0 || wb < 0 || wc < 0 {
						continue
					}
					depth := wa*pa.z + wb*pb.z + wc*pc.z
					p := &pixels[y][x]
					if p.occupied && depth > p.depth+.001 {
						continue
					}
					objY := wa*a.y + wb*b.y + wc*tip.y
					built := mode == forgeIntake || (objY+1.05)/2.1 <= ratio
					edge := math.Min(edgeDistance(fx, fy, pa, pb), math.Min(edgeDistance(fx, fy, pb, pc), edgeDistance(fx, fy, pc, pa))) < .7
					light := faceTone
					if edge {
						light = 1
					}
					if mode == forgeMeasured && ratio >= 1 && wave >= 0 && wave < 900*time.Millisecond {
						light += .12 * (1 - wave.Seconds()/.9)
					}
					on := true
					if !built {
						on = edge && (x+y)%3 == 0
						light = .09
					}
					*p = pixel{depth: depth, light: light, occupied: true, on: on, built: built}
				}
			}
		}
	}
	var out strings.Builder
	bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	for y := 0; y < 19; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x := 0; x < 41; x++ {
			var mask rune
			light := 0.0
			built := false
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					p := pixels[y*4+dy][x*2+dx]
					if p.on {
						mask |= bits[dy][dx]
						light = math.Max(light, p.light)
						built = built || p.built
					}
				}
			}
			glyph := " "
			if mask != 0 {
				glyph = string(rune(0x2800) | mask)
				if !color && !built {
					glyph = "·"
				}
			}
			tone := magneticPulseTone(light)
			if marker == "!" {
				tone = "#D6AA68"
			}
			if marker == "✓" {
				tone = "#8AE8CC"
			}
			if marker != "" && x == 20 && y == 9 {
				glyph = marker
			}
			if color && glyph != " " {
				glyph = lipgloss.NewStyle().Foreground(lipgloss.Color(tone)).Render(glyph)
			}
			out.WriteString(glyph)
		}
	}
	return out.String()
}
