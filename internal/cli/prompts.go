package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type HuhPrompter struct {
	Input           io.Reader
	Output          io.Writer
	runRecipePicker recipePickerRunner
}

type recipePickerRunner func(context.Context, catalogui.Model, io.Reader, io.Writer) (catalogui.Model, error)

func NewHuhPrompter(input io.Reader, output io.Writer) *HuhPrompter {
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	return &HuhPrompter{Input: input, Output: output}
}

func providerOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Vast", "vast"),
		huh.NewOption("Manual SSH", "manual"),
	}
}

const customHuggingFaceWorkload = "huggingface"

const (
	providerDescription         = "Vast creates a paid GPU instance. Manual SSH configures an existing GPU host."
	huggingFaceQueryDescription = "Search the Hub or enter an exact owner/model repository."
	vastAPIKeyDescription       = "Create or copy a key at https://console.vast.ai/keys. Used only for this setup and never stored."
	vastIdentityDescription     = "If missing, setup generates a dedicated Ed25519 key and registers its public key with Vast. Existing private keys are never overwritten."
)

func workloadOptions(recipes []recipe.Recipe) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(recipes)+1)
	for _, selectedRecipe := range recipes {
		status := strings.ToUpper(strings.TrimSpace(selectedRecipe.Profile.Status))
		if status == "" {
			status = "UNKNOWN/UNMEASURED"
		}
		gpuModel := strings.TrimSpace(selectedRecipe.Requirements.GPUModel)
		if gpuModel == "" {
			gpuModel = "unknown/unmeasured"
		}
		gpuCount := "unknown/unmeasured"
		if selectedRecipe.Requirements.GPUCount > 0 {
			gpuCount = strconv.Itoa(selectedRecipe.Requirements.GPUCount) + "×"
		}
		contextWindow := "unknown/unmeasured context"
		if selectedRecipe.Serve.ContextWindow > 0 {
			contextWindow = formatCount(selectedRecipe.Serve.ContextWindow) + " context"
		}
		label := fmt.Sprintf("Recipe · %s · %s · %s %s · %s", selectedRecipe.Name, status, gpuCount, gpuModel, contextWindow)
		options = append(options, huh.NewOption(label, selectedRecipe.ID))
	}
	return append(options, huh.NewOption(
		"Hugging Face model · CUSTOM · unknown/unmeasured GPU · unknown/unmeasured context",
		customHuggingFaceWorkload,
	))
}

func workloadEntries(available []recipe.Recipe) []catalogui.Entry {
	entries := make([]catalogui.Entry, 0, len(available)+1)
	for _, selectedRecipe := range available {
		entries = append(entries, catalogui.EntryFromRecipe(selectedRecipe))
	}
	custom := catalogui.CustomEntry()
	custom.Value = customHuggingFaceWorkload
	entries = append(entries, custom)
	return entries
}

func useAccessibleWorkload(getenv func(string) string) bool {
	return getenv("TERM") == "dumb" || getenv("ACCESSIBLE") != ""
}

func huggingFaceQueryField(query *string) *huh.Input {
	return huh.NewInput().
		Title("Hugging Face model").
		Description(huggingFaceQueryDescription).
		Value(query).
		Validate(required("Hugging Face model"))
}

func huggingFaceModelOptions(results []huggingface.SearchResult) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(results))
	for _, result := range results {
		label := fmt.Sprintf("%s · %s downloads · %s likes", result.Repository, formatCount(result.Downloads), formatCount(result.Likes))
		options = append(options, huh.NewOption(label, result.Repository))
	}
	return options
}

func formatCount(value int) string {
	digits := strconv.Itoa(value)
	var formatted strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			formatted.WriteByte(',')
		}
		formatted.WriteRune(digit)
	}
	return formatted.String()
}

func customHardwareFields(vram, disk *string) []huh.Field {
	if strings.TrimSpace(*disk) == "" {
		*disk = "100"
	}
	return []huh.Field{
		huh.NewInput().Title("Minimum GPU VRAM (GB)").Value(vram).Validate(required("minimum GPU VRAM")),
		huh.NewInput().Title("Minimum disk (GB)").Value(disk).Validate(required("minimum disk")),
	}
}

func offerOptions(views []setup.OfferView) []huh.Option[int] {
	options := make([]huh.Option[int], 0, len(views))
	for _, view := range views {
		options = append(options, huh.NewOption(offerLabel(view), view.Offer.ID))
	}
	return options
}

func offerLabel(view setup.OfferView) string {
	gpu := view.Offer.GPUName
	if strings.TrimSpace(gpu) == "" {
		gpu = "Unknown GPU"
	}
	vram := "unknown/unmeasured"
	if view.Offer.GPUVRAMGB > 0 {
		vram = fmt.Sprintf("%g GB VRAM", view.Offer.GPUVRAMGB)
	}
	reliability := "unknown/unmeasured"
	if view.Offer.Reliability > 0 {
		reliability = fmt.Sprintf("%.1f%%", view.Offer.Reliability*100)
	}
	location := strings.TrimSpace(view.Offer.Location)
	if location == "" {
		location = "unknown/unmeasured"
	}
	return fmt.Sprintf("%s · %s · $%.2f/h · Monthly $%.2f (730h monthly compute) · Annual $%.2f (8,760h annual compute) · Location: %s · Reliability: %s", gpu, vram, view.Offer.HourlyUSD, view.MonthlyUSD, view.AnnualUSD, location, reliability)
}

func customWorkloadConfirmationTitle(model huggingface.Model, hardware CustomHardware) string {
	status := strings.TrimSpace(string(model.Classification.Status))
	if status == "" {
		status = "unknown/unmeasured"
	}
	kind := strings.TrimSpace(model.Classification.Kind)
	if kind == "" {
		kind = "unknown/unmeasured"
	}
	engine := strings.TrimSpace(model.Classification.Engine)
	if engine == "" {
		engine = "unknown/unmeasured"
	}
	return fmt.Sprintf("Confirm custom workload %s@%s? Classification: kind=%s engine=%s status=%s. Hardware: %d GB VRAM, %d GB disk.", model.Repository, model.Revision, kind, engine, status, hardware.MinimumVRAMGB, hardware.MinimumDiskGB)
}

func unknownProviderValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown/unmeasured"
	}
	return value
}

func providerInt(value int) string {
	if value <= 0 {
		return "unknown/unmeasured"
	}
	return strconv.Itoa(value)
}

func providerMoney(value float64, period string) string {
	if value <= 0 {
		return "unknown/unmeasured"
	}
	return "$" + formatGroupedFixed(value, 2) + period
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

func providerPercent(value float64) string {
	if value <= 0 {
		return "unknown/unmeasured"
	}
	return fmt.Sprintf("%.1f%%", value*100)
}

func recipeStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "UNKNOWN/UNMEASURED"
	}
	return strings.ToUpper(value)
}

func recipeCount(value int, suffix string) string {
	if value < 1 {
		return "unknown/unmeasured " + suffix
	}
	return formatCount(value) + " " + suffix
}

func setupKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Input.Next.SetKeys("enter", "ctrl+j", "tab")
	keyMap.Input.Submit.SetKeys("enter", "ctrl+j")
	keyMap.Select.Next.SetKeys("enter", "ctrl+j", "tab")
	keyMap.Select.Submit.SetKeys("enter", "ctrl+j")
	keyMap.Select.Down.SetKeys("down", "j", "ctrl+n")
	keyMap.Confirm.Next.SetKeys("enter", "ctrl+j", "tab")
	keyMap.Confirm.Submit.SetKeys("enter", "ctrl+j")
	keyMap.Confirm.Toggle.SetKeys(" ", "h", "l", "right", "left")
	keyMap.Confirm.Toggle.SetHelp("space/←/→", "toggle")
	return keyMap
}

func providerKeyMap() *huh.KeyMap {
	keyMap := setupKeyMap()
	keyMap.Select.Next.SetKeys("enter", "ctrl+j", "tab", " ")
	keyMap.Select.Submit.SetKeys("enter", "ctrl+j", " ")
	keyMap.Select.Next.SetHelp("enter/space", "select")
	keyMap.Select.Submit.SetHelp("enter/space", "submit")
	keyMap.Select.Filter.SetEnabled(false)
	return keyMap
}

func providerSelectField(provider *string) *huh.Select[string] {
	return huh.NewSelect[string]().
		Title("Choose a setup provider").
		Description(providerDescription).
		Options(providerOptions()...).
		Value(provider)
}

func (p *HuhPrompter) SelectProvider(ctx context.Context) (string, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.SelectProvider(ctx)
	}
	var provider string
	field := providerSelectField(&provider)
	if err := p.runWithKeyMap(ctx, field, providerKeyMap()); err != nil {
		return "", err
	}
	return provider, nil
}

func (p *HuhPrompter) SelectWorkload(ctx context.Context, recipes []recipe.Recipe) (string, error) {
	if fallback := p.accessible(); fallback != nil && p.runRecipePicker == nil {
		return fallback.SelectWorkload(ctx, recipes)
	}
	runner := p.runRecipePicker
	if runner == nil {
		runner = runRecipePickerProgram
	}
	final, err := runner(ctx, catalogui.NewPicker(workloadEntries(recipes)), p.Input, p.Output)
	if err != nil {
		return "", fmt.Errorf("choose workload: %w", err)
	}
	if final.Cancelled() {
		return "", fmt.Errorf("setup cancelled")
	}
	selected, ok := final.SelectedValue()
	if !ok {
		return "", fmt.Errorf("choose workload: no recipe selected")
	}
	return selected, nil
}

func (p *HuhPrompter) accessible() *AccessiblePrompter {
	if !UseTerminalApplication(p.Input, p.Output, os.Getenv) {
		return NewAccessiblePrompter(p.Input, p.Output)
	}
	return nil
}

func runRecipePickerProgram(ctx context.Context, model catalogui.Model, input io.Reader, output io.Writer) (catalogui.Model, error) {
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithAltScreen(),
	)
	result, err := program.Run()
	if err != nil {
		return catalogui.Model{}, err
	}
	final, ok := result.(catalogui.Model)
	if !ok {
		return catalogui.Model{}, fmt.Errorf("recipe picker returned an unexpected model")
	}
	return final, nil
}

func (p *HuhPrompter) HuggingFaceQuery(ctx context.Context) (string, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.HuggingFaceQuery(ctx)
	}
	query := ""
	if err := p.run(ctx, huggingFaceQueryField(&query)); err != nil {
		return "", err
	}
	return strings.TrimSpace(query), nil
}

func (p *HuhPrompter) SelectHuggingFaceModel(ctx context.Context, results []huggingface.SearchResult) (string, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.SelectHuggingFaceModel(ctx, results)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no Hugging Face models found")
	}
	selected := ""
	field := huh.NewSelect[string]().
		Title("Choose a Hugging Face model").
		Options(huggingFaceModelOptions(results)...).
		Value(&selected)
	if err := p.run(ctx, field); err != nil {
		return "", err
	}
	return selected, nil
}

func (p *HuhPrompter) CustomHardware(ctx context.Context) (CustomHardware, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.CustomHardware(ctx)
	}
	vram, disk := "", ""
	if err := p.runGroup(ctx, customHardwareFields(&vram, &disk)...); err != nil {
		return CustomHardware{}, err
	}
	return parseCustomHardware(vram, disk)
}

func parseCustomHardware(vram, disk string) (CustomHardware, error) {
	minimumVRAM, err := strconv.Atoi(strings.TrimSpace(vram))
	if err != nil || minimumVRAM < 1 {
		return CustomHardware{}, fmt.Errorf("minimum GPU VRAM must be a positive number")
	}
	minimumDisk, err := strconv.Atoi(strings.TrimSpace(disk))
	if err != nil || minimumDisk < 1 {
		return CustomHardware{}, fmt.Errorf("minimum disk must be a positive number")
	}
	return CustomHardware{MinimumVRAMGB: minimumVRAM, MinimumDiskGB: minimumDisk}, nil
}

func (p *HuhPrompter) ConfirmCustomWorkload(ctx context.Context, model huggingface.Model, hardware CustomHardware) (bool, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ConfirmCustomWorkload(ctx, model, hardware)
	}
	confirmed := false
	field := huh.NewConfirm().
		Title(customWorkloadConfirmationTitle(model, hardware)).
		Affirmative("Use this workload").
		Negative("Cancel").
		Value(&confirmed)
	if err := p.run(ctx, field); err != nil {
		return false, err
	}
	return confirmed, nil
}

func vastAPIKeyField(token *string) *huh.Input {
	return huh.NewInput().
		Title("Vast API key").
		Description(vastAPIKeyDescription).
		EchoMode(huh.EchoModePassword).
		Value(token).
		Validate(required("Vast API key"))
}

func (p *HuhPrompter) VastAPIKey(ctx context.Context) (string, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.VastAPIKey(ctx)
	}
	var token string
	if err := p.run(ctx, vastAPIKeyField(&token)); err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

func vastIdentityField(identity *string) *huh.Input {
	return huh.NewInput().
		Title("SSH identity file").
		Description(vastIdentityDescription).
		Value(identity).
		Validate(required("SSH identity file"))
}

func identitySetupTitle(path string, generate bool) string {
	if generate {
		return fmt.Sprintf("Generate a dedicated Ed25519 key at %s and register its public key with Vast? This creates local files and changes your Vast account.", path)
	}
	return fmt.Sprintf("Register the public key for %s with Vast? This changes your Vast account.", path)
}

func (p *HuhPrompter) ConfirmIdentitySetup(ctx context.Context, path string, generate bool) (bool, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ConfirmIdentitySetup(ctx, path, generate)
	}
	confirmed := false
	field := huh.NewConfirm().
		Title(identitySetupTitle(path, generate)).
		Affirmative("Continue").
		Negative("Cancel").
		Value(&confirmed)
	if err := p.run(ctx, field); err != nil {
		return false, err
	}
	return confirmed, nil
}

func (p *HuhPrompter) ManualRoute(ctx context.Context, defaultUser string) (ManualRoute, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ManualRoute(ctx, defaultUser)
	}
	var route ManualRoute
	portText := "22"
	fields := manualRouteFields(&route, &portText, defaultUser)
	if err := p.runGroup(ctx, fields...); err != nil {
		return ManualRoute{}, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return ManualRoute{}, fmt.Errorf("SSH port must be a number")
	}
	route.Port = port
	return route, nil
}

func manualRouteFields(route *ManualRoute, portText *string, defaultUser string) []huh.Field {
	route.User = defaultUser
	return []huh.Field{
		huh.NewInput().Title("GPU host").Value(&route.Host).Validate(required("GPU host")),
		huh.NewInput().Title("SSH port").Value(portText).Validate(required("SSH port")),
		huh.NewInput().Title("SSH user").Value(&route.User).Validate(required("SSH user")),
		huh.NewInput().Title("SSH identity file").Value(&route.IdentityFile).Validate(required("SSH identity file")),
		huh.NewInput().Title("Verified known-hosts file").Value(&route.KnownHostsFile).Validate(required("verified known-hosts file")),
	}
}

func (p *HuhPrompter) VastIdentity(ctx context.Context, defaultValue string) (string, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.VastIdentity(ctx, defaultValue)
	}
	identity := defaultValue
	if err := p.run(ctx, vastIdentityField(&identity)); err != nil {
		return "", err
	}
	return strings.TrimSpace(identity), nil
}

func (p *HuhPrompter) SelectOffer(ctx context.Context, views []setup.OfferView) (vast.Offer, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.SelectOffer(ctx, views)
	}
	if len(views) == 0 {
		return vast.Offer{}, fmt.Errorf("no Vast offers available")
	}
	var selected int
	field := huh.NewSelect[int]().Title("Choose a Vast GPU offer").Options(offerOptions(views)...).Value(&selected)
	if err := p.run(ctx, field); err != nil {
		return vast.Offer{}, err
	}
	for _, view := range views {
		if view.Offer.ID == selected {
			return view.Offer, nil
		}
	}
	return vast.Offer{}, fmt.Errorf("selected Vast offer %d is unavailable", selected)
}

func (p *HuhPrompter) ConfirmCost(ctx context.Context, view setup.OfferView, diskGB int) (bool, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ConfirmCost(ctx, view, diskGB)
	}
	return runRentalReview(ctx, p, view, diskGB)
}

func (p *HuhPrompter) run(ctx context.Context, field huh.Field) error {
	return p.mapFormError(huh.NewForm(huh.NewGroup(field)).WithKeyMap(setupKeyMap()).WithInput(p.Input).WithOutput(p.Output).RunWithContext(ctx))
}

func (p *HuhPrompter) runWithKeyMap(ctx context.Context, field huh.Field, keyMap *huh.KeyMap) error {
	return p.mapFormError(huh.NewForm(huh.NewGroup(field)).WithKeyMap(keyMap).WithInput(p.Input).WithOutput(p.Output).RunWithContext(ctx))
}

func (p *HuhPrompter) runGroup(ctx context.Context, fields ...huh.Field) error {
	return p.mapFormError(huh.NewForm(huh.NewGroup(fields...)).WithKeyMap(setupKeyMap()).WithInput(p.Input).WithOutput(p.Output).RunWithContext(ctx))
}

func (p *HuhPrompter) mapFormError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return fmt.Errorf("setup cancelled")
	}
	return err
}

func required(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", strings.ToLower(label))
		}
		return nil
	}
}
