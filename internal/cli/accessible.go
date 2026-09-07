package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// UseTerminalApplication requires a terminal on both sides. Never obtain a
// replacement input stream implicitly when the caller supplied a pipe.
func UseTerminalApplication(input io.Reader, output io.Writer, getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	if useAccessibleWorkload(getenv) {
		return false
	}
	in, inOK := input.(interface{ Fd() uintptr })
	out, outOK := output.(interface{ Fd() uintptr })
	return inOK && outOK && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
}

// AccessiblePrompter deliberately does not use Huh's accessible runner:
// that version reads global stdin and prints password values after submission.
// Reads are synchronous: cancellation is checked between reads, but cannot
// interrupt an arbitrary borrowed io.Reader. The CLI keeps normal OS signal
// semantics and restores terminal state around this legacy command.
type AccessiblePrompter struct {
	lastServerLogs string
	lastDownload   string
	input          io.Reader
	output         io.Writer
	recovery       setup.InstanceRecovery
}

func NewAccessiblePrompter(input io.Reader, output io.Writer) *AccessiblePrompter {
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	return &AccessiblePrompter{input: input, output: output}
}

func (p *AccessiblePrompter) readLine(ctx context.Context, secret bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if fd, ok := p.input.(interface{ Fd() uintptr }); secret && ok && term.IsTerminal(fd.Fd()) {
		value, err := term.ReadPassword(fd.Fd())
		fmt.Fprintln(p.output)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return strings.TrimSpace(string(value)), nil
	}
	// Do not buffer ahead: a following password may need ReadPassword on the
	// same terminal. Incomplete lines at EOF never count as confirmation.
	var line strings.Builder
	var one [1]byte
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(p.input, one[:]); err != nil {
			return "", err
		}
		if one[0] == '\n' {
			return strings.TrimSpace(line.String()), nil
		}
		if line.Len() >= 65536 {
			return "", fmt.Errorf("input line is too long")
		}
		line.WriteByte(one[0])
	}
}

func (p *AccessiblePrompter) inputValue(ctx context.Context, title, initial string, secret bool) (string, error) {
	for {
		prompt := title
		if initial != "" && !secret {
			prompt += " [" + initial + "]"
		}
		if _, err := fmt.Fprint(p.output, prompt+": "); err != nil {
			return "", err
		}
		value, err := p.readLine(ctx, secret)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = initial
		}
		if err := required(title)(value); err != nil {
			fmt.Fprintln(p.output, err)
			continue
		}
		return value, nil
	}
}

func accessibleSelect[T comparable](ctx context.Context, p *AccessiblePrompter, title string, options []huh.Option[T]) (T, error) {
	var zero T
	if len(options) == 0 {
		return zero, fmt.Errorf("no choices available for %s", title)
	}
	if _, err := fmt.Fprintln(p.output, title); err != nil {
		return zero, err
	}
	for i, option := range options {
		if _, err := fmt.Fprintf(p.output, "%d. %s\n", i+1, option.Key); err != nil {
			return zero, err
		}
	}
	for {
		value, err := p.inputValue(ctx, "Choice number", "", false)
		if err != nil {
			return zero, err
		}
		index, err := strconv.Atoi(value)
		if err == nil && index >= 1 && index <= len(options) {
			return options[index-1].Value, nil
		}
		fmt.Fprintf(p.output, "Enter a number between 1 and %d.\n", len(options))
	}
}

func (p *AccessiblePrompter) confirm(ctx context.Context, title string) (bool, error) {
	for {
		if _, err := fmt.Fprint(p.output, title+" [y/N]: "); err != nil {
			return false, err
		}
		answer, err := p.readLine(ctx, false)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.output, "Enter yes or no.")
	}
}

func (p *AccessiblePrompter) ConfirmReplacement(ctx context.Context, path string) (bool, error) {
	return p.confirm(ctx, "Replace existing configuration at "+path+"? This does not stop or destroy any existing remote instance")
}

func (p *AccessiblePrompter) SelectProvider(ctx context.Context) (string, error) {
	return accessibleSelect(ctx, p, "Choose a setup provider (Vast creates a paid GPU instance; Manual SSH uses an existing host)", providerOptions())
}
func (p *AccessiblePrompter) SelectWorkload(ctx context.Context, recipes []recipe.Recipe) (string, error) {
	return accessibleSelect(ctx, p, "Choose a workload", workloadOptions(recipes))
}
func (p *AccessiblePrompter) HuggingFaceQuery(ctx context.Context) (string, error) {
	return p.inputValue(ctx, "Hugging Face model (search or owner/model)", "", false)
}
func (p *AccessiblePrompter) SelectHuggingFaceModel(ctx context.Context, results []huggingface.SearchResult) (string, error) {
	return accessibleSelect(ctx, p, "Choose a Hugging Face model", huggingFaceModelOptions(results))
}
func (p *AccessiblePrompter) CustomHardware(ctx context.Context) (CustomHardware, error) {
	vram, err := p.inputValue(ctx, "Minimum GPU VRAM (GB)", "", false)
	if err != nil {
		return CustomHardware{}, err
	}
	disk, err := p.inputValue(ctx, "Minimum disk (GB)", "100", false)
	if err != nil {
		return CustomHardware{}, err
	}
	return parseCustomHardware(vram, disk)
}
func (p *AccessiblePrompter) ConfirmCustomWorkload(ctx context.Context, model huggingface.Model, hardware CustomHardware) (bool, error) {
	return p.confirm(ctx, customWorkloadConfirmationTitle(model, hardware))
}
func (p *AccessiblePrompter) VastAPIKey(ctx context.Context) (string, error) {
	return p.inputValue(ctx, "Vast API key (https://console.vast.ai/keys; never stored)", "", true)
}
func (p *AccessiblePrompter) VastIdentity(ctx context.Context, initial string) (string, error) {
	return p.inputValue(ctx, "SSH identity file (missing keys require explicit generation/registration consent)", initial, false)
}
func (p *AccessiblePrompter) ManualRoute(ctx context.Context, defaultUser string) (ManualRoute, error) {
	var route ManualRoute
	var err error
	if route.Host, err = p.inputValue(ctx, "GPU host", "", false); err != nil {
		return route, err
	}
	port, err := p.inputValue(ctx, "SSH port", "22", false)
	if err != nil {
		return route, err
	}
	if route.Port, err = strconv.Atoi(port); err != nil {
		return route, fmt.Errorf("SSH port must be a number")
	}
	if route.Port < 1 || route.Port > 65535 {
		return route, fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if route.User, err = p.inputValue(ctx, "SSH user", defaultUser, false); err != nil {
		return route, err
	}
	if route.IdentityFile, err = p.inputValue(ctx, "SSH identity file", "", false); err != nil {
		return route, err
	}
	route.KnownHostsFile, err = p.inputValue(ctx, "Verified known-hosts file", "", false)
	return route, err
}
func (p *AccessiblePrompter) SelectOffer(ctx context.Context, views []setup.OfferView) (vast.Offer, error) {
	id, err := accessibleSelect(ctx, p, "Choose a Vast GPU offer", offerOptions(views))
	if err != nil {
		return vast.Offer{}, err
	}
	for _, view := range views {
		if view.Offer.ID == id {
			return view.Offer, nil
		}
	}
	return vast.Offer{}, fmt.Errorf("selected offer is unavailable")
}
func (p *AccessiblePrompter) ConfirmCost(ctx context.Context, view setup.OfferView, diskGB int) (bool, error) {
	if _, err := fmt.Fprintf(p.output, "Deployment review\n\n%s\n%s\n", newRentalReview(view, diskGB).accessibleSummary(), offerLabel(view)); err != nil {
		return false, err
	}
	return p.confirm(ctx, "Create this paid Vast instance? Exiting only closes the local tunnel; billing continues until the instance is destroyed on Vast.")
}
func (p *AccessiblePrompter) ConfirmIdentitySetup(ctx context.Context, path string, generate bool) (bool, error) {
	return p.confirm(ctx, identitySetupTitle(path, generate))
}
