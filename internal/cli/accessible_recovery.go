package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

func (p *AccessiblePrompter) InstanceCreated(recovery setup.InstanceRecovery) {
	if recovery.InstanceID > 0 && recovery.Destroy != nil {
		p.recovery = recovery
	}
}
func (p *AccessiblePrompter) SetupProgress(progress setup.Progress) {
	fmt.Fprintln(p.output, progressLabel(progress.Stage))
	if progress.InstanceID > 0 {
		fmt.Fprintf(p.output, "Instance #%d — billing may be active.\n", progress.InstanceID)
	}
}
func (p *AccessiblePrompter) recoverSetup(ctx context.Context, cause error, token string) error {
	sanitize := func(err error) string {
		value := err.Error()
		if token != "" {
			value = strings.ReplaceAll(value, token, "[redacted]")
		}
		return strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' || r >= 32 && r != 127 {
				return r
			}
			return -1
		}, ansi.Strip(value))
	}
	original := fmt.Errorf("%s", sanitize(cause))
	capability := p.recovery
	if capability.InstanceID <= 0 || capability.Destroy == nil {
		return original
	}
	fmt.Fprintln(p.output, original)
	for {
		fmt.Fprintf(p.output, "Instance #%d: billing may still be active.\n", capability.InstanceID)
		confirmed, err := p.confirm(ctx, fmt.Sprintf("Destroy instance #%d? All its data will be lost", capability.InstanceID))
		if err != nil || !confirmed {
			return original
		}
		cleanup, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
		fmt.Fprintf(p.output, "Destroying and verifying instance #%d…\n", capability.InstanceID)
		err = capability.Destroy(cleanup)
		cancel()
		if err == nil {
			p.recovery = setup.InstanceRecovery{}
			fmt.Fprintf(p.output, "Instance #%d destroyed. Do not reuse its saved route.\n", capability.InstanceID)
			// The original diagnostic was printed above. Do not repeat its obsolete
			// active-resource warning as the final error after verified cleanup.
			return fmt.Errorf("setup failed; instance #%d destruction was verified", capability.InstanceID)
		}
		fmt.Fprintln(p.output, "Destruction unconfirmed: "+sanitize(err))
		fmt.Fprintln(p.output, "Retry requires another explicit confirmation. Exiting does not guarantee destruction.")
	}
}
