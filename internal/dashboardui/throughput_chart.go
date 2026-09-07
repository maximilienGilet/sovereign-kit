package dashboardui

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	maxThroughputChartWidth  = 4096
	maxThroughputChartHeight = 1024
)

type throughputChartCell struct {
	rune  rune
	style byte
}

const (
	chartPlain byte = iota
	chartBright
	chartAccent
	chartMuted
)

// throughputCeiling returns the smallest 1, 2, or 5 multiple of a power
// of ten that contains every reported generation rate.
func throughputCeiling(samples []throughputSample) float64 {
	peak := 0.0
	for _, sample := range samples {
		if sample.Generation == nil {
			continue
		}
		value := *sample.Generation
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= peak {
			continue
		}
		peak = value
	}
	if peak <= 0 {
		return 1
	}

	power := math.Pow(10, math.Floor(math.Log10(peak)))
	fraction := peak / power
	multiplier := 10.0
	switch {
	case fraction <= 1:
		multiplier = 1
	case fraction <= 2:
		multiplier = 2
	case fraction <= 5:
		multiplier = 5
	}
	ceiling := multiplier * power
	if math.IsInf(ceiling, 0) {
		return math.MaxFloat64
	}
	return ceiling
}

func renderThroughputChart(samples []throughputSample, now time.Time, width, height int, interrupted bool) string {
	width, height = sanitizeThroughputChartDimensions(width, height)
	if width == 0 || height == 0 {
		return ""
	}

	rows := make([][]throughputChartCell, height)
	for row := range rows {
		rows[row] = make([]throughputChartCell, width)
		for column := range rows[row] {
			rows[row][column].rune = ' '
		}
	}

	axisWidth := 0
	if width > 1 {
		axisWidth = min(7, width-1)
	}
	plotWidth := width - axisWidth
	plotHeight := max(1, height-1)
	ceiling := throughputCeiling(samples)
	drawThroughputAxes(rows, axisWidth, plotHeight, ceiling)

	braille := make([][]byte, plotHeight)
	for row := range braille {
		braille[row] = make([]byte, plotWidth)
	}
	reported := plotThroughputSamples(braille, samples, now, ceiling)
	plotStyle := chartAccent
	if interrupted {
		plotStyle = chartMuted
	}
	for row := range braille {
		for column, dots := range braille[row] {
			if dots == 0 {
				continue
			}
			rows[row][axisWidth+column] = throughputChartCell{
				rune:  rune(0x2800) + rune(dots),
				style: plotStyle,
			}
		}
	}

	if !reported {
		putThroughputText(rows[max(0, (plotHeight-1)/2)], axisWidth, "Throughput unavailable", chartMuted)
	}
	if interrupted {
		putThroughputStatus(rows[0], axisWidth, "Telemetry interrupted")
	}

	return renderThroughputRows(rows)
}

func sanitizeThroughputChartDimensions(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	return min(width, maxThroughputChartWidth), min(height, maxThroughputChartHeight)
}

func drawThroughputAxes(rows [][]throughputChartCell, axisWidth, plotHeight int, ceiling float64) {
	if len(rows) == 0 {
		return
	}
	if axisWidth > 0 {
		for row := 0; row < plotHeight; row++ {
			rows[row][axisWidth-1] = throughputChartCell{rune: '│', style: chartBright}
		}
		putThroughputTextRight(rows[0], axisWidth-1, fmt.Sprintf("%g", ceiling), chartBright)
		putThroughputTextRight(rows[plotHeight-1], axisWidth-1, "0", chartBright)
	}
	if len(rows) < 2 {
		return
	}
	labelRow := rows[len(rows)-1]
	plotWidth := len(labelRow) - axisWidth
	putThroughputText(labelRow, axisWidth, "-10m", chartBright)
	putThroughputText(labelRow, axisWidth+(plotWidth-len("-5m"))/2, "-5m", chartBright)
	putThroughputText(labelRow, len(labelRow)-len("now"), "now", chartBright)
}

func plotThroughputSamples(canvas [][]byte, samples []throughputSample, now time.Time, ceiling float64) bool {
	if len(canvas) == 0 || len(canvas[0]) == 0 {
		return false
	}
	pixelWidth, pixelHeight := len(canvas[0])*2, len(canvas)*4
	windowStart := now.Add(-throughputWindow)
	reported := false
	havePrevious := false
	previousX, previousY := 0, 0
	for _, sample := range samples {
		if sample.Generation == nil || math.IsNaN(*sample.Generation) || math.IsInf(*sample.Generation, 0) {
			havePrevious = false
			continue
		}

		xRatio := float64(sample.At.Sub(windowStart)) / float64(throughputWindow)
		yRatio := *sample.Generation / ceiling
		x := clampThroughputCoordinate(int(math.Round(xRatio*float64(pixelWidth-1))), pixelWidth)
		y := clampThroughputCoordinate(int(math.Round((1-yRatio)*float64(pixelHeight-1))), pixelHeight)
		if havePrevious {
			drawThroughputLine(canvas, previousX, previousY, x, y)
		} else {
			setThroughputPixel(canvas, x, y)
		}
		previousX, previousY = x, y
		havePrevious = true
		reported = true
	}
	return reported
}

func clampThroughputCoordinate(value, size int) int {
	if value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}

func drawThroughputLine(canvas [][]byte, startX, startY, endX, endY int) {
	deltaX := absThroughputCoordinate(endX - startX)
	stepX := -1
	if startX < endX {
		stepX = 1
	}
	deltaY := -absThroughputCoordinate(endY - startY)
	stepY := -1
	if startY < endY {
		stepY = 1
	}
	errorValue := deltaX + deltaY
	for {
		setThroughputPixel(canvas, startX, startY)
		if startX == endX && startY == endY {
			return
		}
		twiceError := 2 * errorValue
		if twiceError >= deltaY {
			errorValue += deltaY
			startX += stepX
		}
		if twiceError <= deltaX {
			errorValue += deltaX
			startY += stepY
		}
	}
}

func absThroughputCoordinate(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func setThroughputPixel(canvas [][]byte, x, y int) {
	if len(canvas) == 0 || len(canvas[0]) == 0 {
		return
	}
	x = clampThroughputCoordinate(x, len(canvas[0])*2)
	y = clampThroughputCoordinate(y, len(canvas)*4)
	cellX, cellY := x/2, y/4
	localX, localY := x%2, y%4
	bits := [4][2]byte{
		{1 << 0, 1 << 3},
		{1 << 1, 1 << 4},
		{1 << 2, 1 << 5},
		{1 << 6, 1 << 7},
	}
	canvas[cellY][cellX] |= bits[localY][localX]
}

func putThroughputStatus(row []throughputChartCell, start int, value string) {
	valueRunes := []rune(value)
	available := len(row) - start
	if available <= 0 {
		return
	}
	if len(valueRunes) > available {
		valueRunes = valueRunes[:available]
	}
	column := len(row) - len(valueRunes)
	if column > start {
		rowsBeforeStatus := column - 1
		row[rowsBeforeStatus] = throughputChartCell{rune: ' ', style: chartPlain}
	}
	putThroughputText(row, column, string(valueRunes), chartMuted)
}

func putThroughputText(row []throughputChartCell, start int, value string, style byte) {
	if start < 0 {
		valueRunes := []rune(value)
		if -start >= len(valueRunes) {
			return
		}
		value = string(valueRunes[-start:])
		start = 0
	}
	for _, character := range value {
		if start >= len(row) {
			return
		}
		row[start] = throughputChartCell{rune: character, style: style}
		start++
	}
}

func putThroughputTextRight(row []throughputChartCell, end int, value string, style byte) {
	valueRunes := []rune(value)
	if len(valueRunes) > end {
		valueRunes = valueRunes[len(valueRunes)-end:]
	}
	putThroughputText(row, end-len(valueRunes), string(valueRunes), style)
}

func renderThroughputRows(rows [][]throughputChartCell) string {
	lines := make([]string, len(rows))
	for rowIndex, row := range rows {
		var line strings.Builder
		for start := 0; start < len(row); {
			style := row[start].style
			end := start + 1
			for end < len(row) && row[end].style == style {
				end++
			}
			var run strings.Builder
			for _, cell := range row[start:end] {
				run.WriteRune(cell.rune)
			}
			value := run.String()
			switch style {
			case chartBright:
				value = bright.Render(value)
			case chartAccent:
				value = accent.Render(value)
			case chartMuted:
				value = muted.Render(value)
			}
			line.WriteString(value)
			start = end
		}
		lines[rowIndex] = line.String()
	}
	return strings.Join(lines, "\n")
}
