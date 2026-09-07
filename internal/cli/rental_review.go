package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

// The root owns terminal lifetime; standalone rental review keeps its adapter.
type rentalReviewResultMsg struct{ Confirmed bool }
type embeddedRentalReview struct {
	model    rentalReviewModel
	viewport viewport.Model
}

func newEmbeddedRentalReview(view setup.OfferView, disk int) embeddedRentalReview {
	model := newRentalReviewModel(view, disk)
	return embeddedRentalReview{model: model, viewport: viewport.New(80, 16)}
}
func (m embeddedRentalReview) Init() tea.Cmd { return nil }
func (m embeddedRentalReview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.model.width = msg.Width
		m.model.height = msg.Height
		m.viewport.Width = max(1, msg.Width)
		m.viewport.Height = max(1, msg.Height-lipgloss.Height(rentalButtons(m.model.createSelected, msg.Width)))
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "right", "h", "l", " ", "tab":
			m.model.createSelected = !m.model.createSelected
		case "enter":
			confirmed := m.model.createSelected
			return m, func() tea.Msg { return rentalReviewResultMsg{confirmed} }
		case "esc", "n":
			return m, func() tea.Msg { return rentalReviewResultMsg{false} }
		default:
			m.viewport, _ = m.viewport.Update(msg)
		}
	}
	content := m.model.review.visualSummary(m.viewport.Width)
	m.viewport.SetContent(ansi.Wrap(content, max(1, m.viewport.Width), ""))
	return m, nil
}
func (m embeddedRentalReview) View() string {
	buttons := rentalButtons(m.model.createSelected, m.viewport.Width)
	return m.viewport.View() + "\n" + lipgloss.NewStyle().Width(m.viewport.Width).Align(lipgloss.Center).Render(buttons)
}

type rentalReview struct {
	view                                       setup.OfferView
	diskGB                                     int
	skuMatch, countMatch, vramKnown, diskKnown bool
	vramRatio, diskRatio                       float64
}
type rentalReviewModel struct {
	review                          rentalReview
	width, height                   int
	createSelected, confirmed, done bool
	viewport                        viewport.Model
}

var (
	rentalCyan       = lipgloss.Color("#00B8FF")
	rentalAmber      = lipgloss.Color("#FFB454")
	rentalMuted      = lipgloss.Color("#7E98A8")
	rentalInk        = lipgloss.Color("#E9F4F8")
	rentalLine       = lipgloss.Color("#385163")
	rentalAccent     = lipgloss.NewStyle().Foreground(rentalCyan).Bold(true)
	rentalWarning    = lipgloss.NewStyle().Foreground(rentalAmber).Bold(true)
	rentalSecondary  = lipgloss.NewStyle().Foreground(rentalMuted)
	rentalPrimary    = lipgloss.NewStyle().Foreground(rentalInk)
	rentalIdleButton = lipgloss.NewStyle().Foreground(rentalMuted).Border(lipgloss.RoundedBorder()).BorderForeground(rentalLine).Padding(0, 1)
	rentalHotButton  = lipgloss.NewStyle().Foreground(lipgloss.Color("#071217")).Background(rentalAmber).Border(lipgloss.RoundedBorder()).BorderForeground(rentalAmber).Padding(0, 1).Bold(true)
	rentalSafeButton = lipgloss.NewStyle().Foreground(lipgloss.Color("#071217")).Background(rentalCyan).Border(lipgloss.RoundedBorder()).BorderForeground(rentalCyan).Padding(0, 1).Bold(true)
)

func newRentalReview(view setup.OfferView, diskGB int) rentalReview {
	offer := view.Offer
	requirements := view.Recipe.Requirements
	review := rentalReview{
		view:       view,
		diskGB:     diskGB,
		skuMatch:   strings.TrimSpace(offer.GPUName) != "" && strings.TrimSpace(offer.GPUName) == strings.TrimSpace(requirements.GPUModel),
		countMatch: offer.GPUCount > 0 && offer.GPUCount == requirements.GPUCount,
		vramKnown:  offer.GPUVRAMGB > 0 && requirements.MinimumVRAMGB > 0,
		diskKnown:  offer.DiskSpaceGB > 0 && diskGB > 0,
	}
	if review.vramKnown {
		review.vramRatio = offer.GPUVRAMGB / float64(requirements.MinimumVRAMGB)
	}
	if review.diskKnown {
		review.diskRatio = offer.DiskSpaceGB / float64(diskGB)
	}
	return review
}

func newRentalReviewModel(view setup.OfferView, diskGB int) rentalReviewModel {
	return rentalReviewModel{review: newRentalReview(view, diskGB), width: 120, height: 30, viewport: viewport.New(120, 23)}
}
func (model rentalReviewModel) Init() tea.Cmd { return nil }
func (model rentalReviewModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = max(1, message.Width), max(1, message.Height)
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "esc", "q", "n":
			model.done = true
			return model, tea.Quit
		case "left", "h":
			model.createSelected = true
		case "right", "l":
			model.createSelected = false
		case "tab", "shift+tab", " ":
			model.createSelected = !model.createSelected
		case "enter":
			model.confirmed = model.createSelected
			model.done = true
			return model, tea.Quit
		default:
			model.viewport, _ = model.viewport.Update(message)
		}
	}
	model.resize()
	return model, nil
}
func (model *rentalReviewModel) resize() {
	model.viewport.Width = max(1, min(146, model.width))
	model.viewport.Height = max(1, model.height-3-lipgloss.Height(rentalButtons(model.createSelected, model.width)))
	model.viewport.SetContent(model.review.visualSummary(model.viewport.Width))
}
func (model rentalReviewModel) View() string {
	if model.done {
		return ""
	}
	model.resize()
	header := fitBrowser("SOVEREIGN KIT / DEPLOYMENT REVIEW", model.width, 1)
	buttons := rentalButtons(model.createSelected, model.viewport.Width)
	content := header + "\n" + model.viewport.View() + "\n" + buttons + "\n" + fitBrowser("←→ select · Enter · Esc · PgDn", model.width, 1)
	return fitBrowser(content, model.width, model.height)
}
func (review rentalReview) visualSummary(width int) string {
	view := review.view
	view.Recipe.Requirements.MinimumDiskGB = review.diskGB
	heading := cleanOfferText(view.Recipe.Name) + " · " + recipeStatus(view.Recipe.Profile.Status)
	warning := "Billing starts immediately · compute only · excludes storage, egress, and tax."
	facts := offerFacts(view, width, 1)
	if width >= 134 {
		leftWidth := min(64, width/2-2)
		left := offerFacts(view, leftWidth, 1)
		right := ansi.Wrap(offerInspection(view), width-leftWidth-3, "")
		facts = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(leftWidth).Render(left), "   ", right)
	} else {
		facts += "\n\n" + offerInspection(view)
	}
	return ansi.Wrap(heading+"\n"+warning+"\n\n"+facts, max(1, width), "")
}

func (review rentalReview) costSummary(width int) string {
	cost := rentalWarning.Render(offerCostSummary(review.view.Offer))
	warning := rentalWarning.Render("Billing starts immediately") + rentalSecondary.Render(" · compute only · excludes storage, egress, and tax")
	if width >= 130 {
		return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(cost + "    " + warning)
	}
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(cost + "\n" + warning)
}

func rentalButtons(createSelected bool, width int) string {
	if width < 48 {
		if createSelected {
			return rentalWarning.Bold(true).Render("> [Create paid]") + "  " + rentalSecondary.Render("[Cancel]")
		}
		return rentalSecondary.Render("[Create paid]") + "  " + rentalAccent.Bold(true).Render("> [Cancel]")
	}
	createButton := rentalIdleButton.Render("  Create paid instance")
	cancelButton := rentalSafeButton.Render("> Cancel")
	if createSelected {
		createButton = rentalHotButton.Render("> Create paid instance")
		cancelButton = rentalIdleButton.Render("  Cancel")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, createButton, "  ", cancelButton)
}

func (review rentalReview) costAndControls(createSelected bool, width int) string {
	buttons := rentalButtons(createSelected, width)
	help := rentalSecondary.Render("←/→ select  ·  enter confirm  ·  esc cancel")
	if width >= 130 {
		buttons = lipgloss.JoinHorizontal(lipgloss.Center, buttons, "    ", help)
	} else {
		buttons += "\n" + help
	}
	return review.costSummary(width) + "\n" + lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(buttons)
}

func (review rentalReview) accessibleSummary() string {
	offer := review.view.Offer
	selectedRecipe := review.view.Recipe
	lines := []string{
		fmt.Sprintf("%s · %s", unknownProviderValue(selectedRecipe.Name), recipeStatus(selectedRecipe.Profile.Status)),
		unknownProviderValue(selectedRecipe.Profile.Summary),
		"Evidence: " + unknownProviderValue(selectedRecipe.Profile.Evidence),
		fmt.Sprintf("Offer %s · Machine %s", providerInt(offer.ID), providerInt(offer.MachineID)),
		fmt.Sprintf("Hardware: %s× %s · %s per GPU · %s total VRAM", providerInt(offer.GPUCount), unknownProviderValue(offer.GPUName), providerFloat(offer.GPUVRAMGB, "GB"), providerFloat(offer.TotalGPUVRAMGB, "GB")),
		fmt.Sprintf("Host: %s CPU cores · %s RAM", providerFloat(offer.CPUCores, ""), providerFloat(offer.CPURAMGB, "GB")),
		fmt.Sprintf("Storage: %s allocated · %s available", recipeCount(review.diskGB, "GB"), providerFloat(offer.DiskSpaceGB, "GB")),
		fmt.Sprintf("Network: %s down · %s up", providerFloat(offer.InetDownMBps, "MB/s"), providerFloat(offer.InetUpMBps, "MB/s")),
		fmt.Sprintf("Driver: %s", unknownProviderValue(offer.DriverVersion)),
		fmt.Sprintf("Region: %s · %s · not guaranteed", unknownProviderValue(offer.Location), offerReliability(offer)),
		"Cost: " + offerCostSummary(offer),
		fmt.Sprintf("Model: %s", unknownProviderValue(selectedRecipe.Model.Repository)),
		fmt.Sprintf("Revision: %s", unknownProviderValue(selectedRecipe.Model.Revision)),
		fmt.Sprintf("Runtime: %s · %s", runtimeSummary(selectedRecipe), unknownProviderValue(selectedRecipe.Runtime.Image)),
	}
	if controls := runtimeControls(selectedRecipe.Runtime); controls != "" {
		lines = append(lines, "Runtime controls: "+controls)
	}
	lines = append(lines,
		fmt.Sprintf("Limits: %s · %s · %s", recipeCount(selectedRecipe.Serve.ContextWindow, "context"), recipeCount(selectedRecipe.Serve.MaxOutputTokens, "output"), recipeCount(selectedRecipe.Serve.MaxRunningRequests, "concurrent")),
		fmt.Sprintf("Fit: SKU %s · GPU count %s · %s · %s", matchLabel(review.skuMatch), matchLabel(review.countMatch), ratioLabel("VRAM", review.vramRatio, review.vramKnown), ratioLabel("Disk", review.diskRatio, review.diskKnown)),
		"Warning: billing starts immediately; compute only; excludes storage, egress, and tax.",
	)
	for i := range lines {
		lines[i] = cleanOfferText(lines[i])
	}
	return strings.Join(lines, "\n")
}

func runAccessibleRentalReview(ctx context.Context, prompter *HuhPrompter, review rentalReview) (bool, error) {
	if _, err := fmt.Fprintf(prompter.Output, "Deployment review\n\n%s\n", review.accessibleSummary()); err != nil {
		return false, fmt.Errorf("write deployment review: %w", err)
	}
	reader := bufio.NewReader(prompter.Input)
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		if _, err := fmt.Fprint(prompter.Output, "\nCreate this paid Vast instance? [y/N] "); err != nil {
			return false, fmt.Errorf("write deployment confirmation: %w", err)
		}
		readResult := make(chan struct {
			answer string
			err    error
		}, 1)
		go func() {
			answer, err := reader.ReadString('\n')
			readResult <- struct {
				answer string
				err    error
			}{answer: answer, err: err}
		}()
		var answer string
		var err error
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case result := <-readResult:
			answer, err = result.answer, result.err
		}
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("read deployment confirmation: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		default:
			if _, writeErr := fmt.Fprintln(prompter.Output, "Enter yes or no."); writeErr != nil {
				return false, fmt.Errorf("write deployment confirmation guidance: %w", writeErr)
			}
			if err == io.EOF {
				return false, nil
			}
		}
	}
}

func runRentalReview(ctx context.Context, prompter *HuhPrompter, view setup.OfferView, diskGB int) (bool, error) {
	review := newRentalReview(view, diskGB)
	if os.Getenv("TERM") == "dumb" || os.Getenv("ACCESSIBLE") != "" {
		return runAccessibleRentalReview(ctx, prompter, review)
	}
	program := tea.NewProgram(
		newRentalReviewModel(view, diskGB),
		tea.WithContext(ctx),
		tea.WithInput(prompter.Input),
		tea.WithOutput(prompter.Output),
		tea.WithAltScreen(),
	)
	result, err := program.Run()
	if err != nil {
		return false, err
	}
	final, ok := result.(rentalReviewModel)
	if !ok {
		return false, fmt.Errorf("rental review returned an unexpected model")
	}
	return final.confirmed, nil
}

func factLine(name, value string) string {
	return rentalSecondary.Render(fmt.Sprintf("%-9s", name)) + rentalPrimary.Render(value)
}

func ratioLabel(name string, ratio float64, known bool) string {
	if !known {
		return name + " unknown"
	}
	return fmt.Sprintf("%s ×%.2f", name, ratio)
}

func matchLabel(matches bool) string {
	if matches {
		return "EXACT"
	}
	return "UNKNOWN"
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

func speculativeDisplayName(algorithm string) string {
	if algorithm == "qwen3_5_mtp" {
		return "MTP"
	}
	return algorithm
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

func wrapReviewText(value string, width int) string {
	return lipgloss.NewStyle().Width(max(width, 10)).Render(unknownProviderValue(value))
}
