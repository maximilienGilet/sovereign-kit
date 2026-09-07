package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func cleanOfferText(value string) string {
	value = ansi.Strip(value)
	value = strings.Map(func(r rune) rune {
		if r >= 32 && r != 127 {
			return r
		}
		if r == '\n' {
			return ' '
		}
		return -1
	}, value)
	runes := []rune(value)
	if len(runes) > 1000 {
		value = string(runes[:1000]) + "…"
	}
	return value
}
func offerPrice(offer vast.Offer) string {
	if offer.PriceUnknown || math.IsNaN(offer.HourlyUSD) || math.IsInf(offer.HourlyUSD, 0) || offer.HourlyUSD < 0 {
		return "Price unknown"
	}
	return "$" + offerHourlyAmount(offer.HourlyUSD) + "/h"
}
func offerHourlyAmount(value float64) string {
	if math.Abs(value*100-math.Round(value*100)) < 1e-9 {
		return formatGroupedFixed(value, 2)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
func offerCostSummary(offer vast.Offer) string {
	if offerPrice(offer) == "Price unknown" {
		return "Price unknown · monthly estimate unknown"
	}
	return "$" + offerHourlyAmount(offer.HourlyUSD) + "/hour · $" + formatGroupedFixed(offer.HourlyUSD*730, 2) + "/month"
}
func offerReliability(offer vast.Offer) string {
	if offer.ReliabilityUnknown || math.IsNaN(offer.Reliability) || offer.Reliability < 0 || offer.Reliability > 1 {
		return "Reliability unknown"
	}
	return fmt.Sprintf("Reliability %.1f%% declared", offer.Reliability*100)
}

func offerCountry(offer vast.Offer) string {
	code := vast.CountryCode(offer.Location)
	if code == "" {
		return "Country unknown"
	}
	for _, country := range vast.Countries() {
		if country.Code == code {
			return country.Name + " (" + code + ")"
		}
	}
	return "Country unknown"
}
func offerInspection(view setup.OfferView) string {
	o, r := view.Offer, view.Recipe
	return strings.Join([]string{
		fmt.Sprintf("Offer %d · Machine %s", o.ID, providerInt(o.MachineID)),
		"Provider location: " + cleanOfferText(unknownProviderValue(o.Location)),
		"Driver: " + cleanOfferText(unknownProviderValue(o.DriverVersion)),
		"Model: " + cleanOfferText(r.Model.Repository),
		"Revision: " + cleanOfferText(r.Model.Revision),
		"Runtime: " + cleanOfferText(runtimeSummary(r)),
		"Image: " + cleanOfferText(r.Runtime.Image),
		"Runtime controls: " + cleanOfferText(unknownProviderValue(runtimeControls(r.Runtime))),
		fmt.Sprintf("Limits: %d context · %d output · %d concurrent", r.Serve.ContextWindow, r.Serve.MaxOutputTokens, r.Serve.MaxRunningRequests),
		"Evidence: " + cleanOfferText(unknownProviderValue(r.Profile.Evidence)),
		"Warning: billing starts immediately; compute only; excludes storage, egress, and tax.",
	}, "\n")
}

// Scale is 0..max(available,required); the marker is the recipe's requirement,
// never an invented score. Labels always describe the latest selected offer.
func capacityBar(label string, available float64, required int, unit string, width int, progress float64) string {
	w := max(4, width-2)
	if available <= 0 || math.IsNaN(available) || math.IsInf(available, 0) {
		return fmt.Sprintf("%s unknown · %d GB required", label, required)
	}
	if required <= 0 {
		return fmt.Sprintf("%s %s %s · requirement unknown", label, formatMetricNumber(available), unit)
	}
	scale := math.Max(available, float64(required))
	filled := min(w, max(0, int(math.Round(available/scale*float64(w)*math.Max(0, math.Min(1, progress))))))
	marker := min(w-1, max(0, int(math.Round(float64(required)/scale*float64(w-1)))))
	bar := make([]rune, w)
	for i := range bar {
		if i < filled {
			bar[i] = '█'
		} else {
			bar[i] = '─'
		}
	}
	bar[marker] = '│'
	maximum := formatMetricNumber(scale) + " GB"
	ruler := "0" + strings.Repeat(" ", max(1, w-1-ansi.StringWidth(maximum))) + maximum
	return fmt.Sprintf("%s %s %s · %d GB required\n%s\n%s\n│ recipe requirement", label, formatMetricNumber(available), unit, required, string(bar), ruler)
}

func offerFacts(view setup.OfferView, width int, progress float64) string {
	o := view.Offer
	r := view.Recipe.Requirements
	monthly := "Monthly compute estimate unknown"
	if !o.PriceUnknown && o.HourlyUSD >= 0 && !math.IsNaN(o.HourlyUSD) && !math.IsInf(o.HourlyUSD, 0) {
		monthly = fmt.Sprintf("$%.2f/month · 730h · compute only", o.HourlyUSD*730)
	}
	lines := []string{
		fmt.Sprintf("%s× %s", providerInt(o.GPUCount), cleanOfferText(unknownProviderValue(o.GPUName))),
		offerCountry(o),
		offerPrice(o),
		monthly,
		"Excludes storage, egress and tax.",
		offerCompatibility(view),
		"",
		capacityBar("VRAM", o.GPUVRAMGB, r.MinimumVRAMGB, "GB / GPU", width, progress),
		fmt.Sprintf("%s total VRAM · %s GPU(s)", providerFloat(o.TotalGPUVRAMGB, "GB"), providerInt(o.GPUCount)),
		"",
		capacityBar("Disk", o.DiskSpaceGB, r.MinimumDiskGB, "GB / instance", width, progress),
		"",
		fmt.Sprintf("CPU %s cores · RAM %s", providerFloat(o.CPUCores, ""), providerFloat(o.CPURAMGB, "GB")),
		fmt.Sprintf("Network ↓%s · ↑%s", providerFloat(o.InetDownMBps, "MB/s"), providerFloat(o.InetUpMBps, "MB/s")),
		offerReliability(o) + " · not guaranteed",
	}
	return ansi.Wrap(strings.Join(lines, "\n"), max(1, width), "")
}

func offerCompatibility(view setup.OfferView) string {
	o, r := view.Offer, view.Recipe.Requirements
	policy := "preferred"
	if r.StrictGPU {
		policy = "exact"
	}
	if r.GPUModel == "" || r.GPUCount < 1 {
		return "GPU policy: minimum VRAM/disk; SKU/count not specified"
	}
	match := "different from preference"
	if strings.TrimSpace(o.GPUName) == strings.TrimSpace(r.GPUModel) && o.GPUCount == r.GPUCount {
		match = "matches"
	}
	return fmt.Sprintf("GPU %s: %d× %s · %s", policy, r.GPUCount, cleanOfferText(r.GPUModel), match)
}
func formatGroupedFixed(value float64, precision int) string {
	raw := strconv.FormatFloat(value, 'f', precision, 64)
	parts := strings.SplitN(raw, ".", 2)
	digits := parts[0]
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = strings.TrimPrefix(digits, "-")
	}
	var grouped strings.Builder
	grouped.WriteString(sign)
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if len(parts) == 2 {
		grouped.WriteByte('.')
		grouped.WriteString(parts[1])
	}
	return grouped.String()
}

func providerInt(value int) string {
	if value <= 0 {
		return "unknown/unmeasured"
	}
	return strconv.Itoa(value)
}

func unknownProviderValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown/unmeasured"
	}
	return value
}

func speculativeDisplayName(algorithm string) string {
	if algorithm == "qwen3_5_mtp" {
		return "MTP"
	}
	return algorithm
}

func runtimeSummary(selectedRecipe recipe.Recipe) string {
	parts := []string{unknownProviderValue(selectedRecipe.Runtime.Engine)}
	if selectedRecipe.Runtime.Quantization != "" {
		parts = append(parts, selectedRecipe.Runtime.Quantization)
	}
	if selectedRecipe.Runtime.KVCacheDType != "" {
		parts = append(parts, selectedRecipe.Runtime.KVCacheDType)
	}
	if selectedRecipe.Speculative != nil && selectedRecipe.Speculative.Algorithm != "" {
		speculative := speculativeDisplayName(selectedRecipe.Speculative.Algorithm)
		if selectedRecipe.Speculative.NumDraftTokens > 0 {
			speculative += fmt.Sprintf(" ×%d", selectedRecipe.Speculative.NumDraftTokens)
		}
		parts = append(parts, speculative)
	}
	return strings.Join(parts, " · ")
}

func runtimeControls(runtime recipe.Runtime) string {
	parts := make([]string, 0, 2)
	if runtime.GPUMemoryUtilization > 0 {
		parts = append(parts, fmt.Sprintf("GPU memory: %.0f%%", runtime.GPUMemoryUtilization*100))
	}
	if runtime.DisableAsyncScheduling {
		parts = append(parts, "async scheduling disabled")
	}
	return strings.Join(parts, " · ")
}

func formatMetricNumber(value float64) string {
	precision := 0
	if math.Abs(value-math.Round(value)) > 0.000001 {
		precision = 1
	}
	return formatGroupedFixed(value, precision)
}

func providerFloat(value float64, suffix string) string {
	if value <= 0 {
		return "unknown/unmeasured"
	}
	formatted := formatMetricNumber(value)
	if suffix == "" {
		return formatted
	}
	return formatted + " " + suffix
}
