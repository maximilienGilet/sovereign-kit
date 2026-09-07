package cli

import (
	"context"
	"fmt"
	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ResumePrompter interface {
	ConfirmResume(context.Context, int, recipe.Recipe, string) (bool, error)
}

func parseResumeID(value string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("enter a positive Vast instance ID")
	}
	return id, nil
}

func (p *AccessiblePrompter) ResumeInstanceID(ctx context.Context) (int, error) {
	value, err := p.inputValue(ctx, "Existing Vast instance ID (no new instance will be created)", "", false)
	if err != nil {
		return 0, err
	}
	return parseResumeID(value)
}

func (p *HuhPrompter) ResumeInstanceID(ctx context.Context) (int, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ResumeInstanceID(ctx)
	}
	value := ""
	err := p.run(ctx, huh.NewInput().Title("Existing Vast instance ID").Value(&value).Validate(func(s string) error { _, err := parseResumeID(s); return err }))
	if err != nil {
		return 0, err
	}
	return parseResumeID(value)
}

// ResumeSetup never delegates to ordinary setup or offer selection. Legacy
// imports require an explicit recipe and identity; journal resumes use pins.
func ResumeSetup(ctx context.Context, output io.Writer, path string, deps SetupDependencies, instanceID int, recoveryOnly bool) error {
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.HomeDir == nil {
		deps.HomeDir = os.UserHomeDir
	}
	if deps.Prompter == nil || deps.RunVast == nil {
		return fmt.Errorf("resume prompter and runner are required")
	}
	if accessible, ok := deps.Prompter.(*AccessiblePrompter); ok {
		accessible.recovery = setup.InstanceRecovery{}
	}
	cp, err := setup.ReadCheckpoint(setup.CheckpointPath(path))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read pending deployment: %w", err)
	}
	workload := VastWorkload{Resume: true, ResumeInstanceID: instanceID, RecoveryOnly: recoveryOnly}
	identity := ""
	if err == nil {
		if instanceID > 0 && cp.InstanceID > 0 && instanceID != cp.InstanceID {
			return fmt.Errorf("pending instance #%d does not match requested #%d", cp.InstanceID, instanceID)
		}
		workload.Recipe = cp.Recipe
		identity = cp.IdentityFile
		if instanceID == 0 {
			instanceID = cp.InstanceID
		}
	} else {
		if instanceID <= 0 {
			p, ok := deps.Prompter.(interface {
				ResumeInstanceID(context.Context) (int, error)
			})
			if !ok {
				return fmt.Errorf("no pending deployment; use sovkit resume INSTANCE_ID to recover an existing instance")
			}
			instanceID, err = p.ResumeInstanceID(ctx)
			if err != nil {
				return err
			}
			if instanceID <= 0 {
				return fmt.Errorf("a positive instance ID is required")
			}
			workload.ResumeInstanceID = instanceID
		}
		available := deps.Recipes
		if len(available) == 0 && deps.LoadRecipes != nil {
			available, err = deps.LoadRecipes()
			if err != nil {
				return err
			}
		}
		p, ok := deps.Prompter.(WorkloadPrompter)
		if !ok {
			return fmt.Errorf("resume requires explicit recipe selection")
		}
		selected, err := resolveVastWorkload(ctx, p, available, deps.SearchModels, deps.InspectModel)
		if err != nil {
			return err
		}
		workload.Recipe = selected.Recipe
		home, err := deps.HomeDir()
		if err != nil {
			return err
		}
		identity, err = deps.Prompter.VastIdentity(ctx, filepath.Join(home, ".ssh", "sovkit_vast_ed25519"))
		if err != nil {
			return err
		}
	}
	if instanceID <= 0 {
		return fmt.Errorf("creation outcome is uncertain; check Vast, then use sovkit resume INSTANCE_ID; no new instance will be created")
	}
	accessibleDestroy := false
	if accessible, ok := deps.Prompter.(*AccessiblePrompter); ok && !recoveryOnly {
		action, err := accessibleSelect(ctx, accessible, fmt.Sprintf("Existing instance #%d — billing may be active", instanceID), []huh.Option[string]{huh.NewOption("Resume", "resume"), huh.NewOption("Destroy", "destroy"), huh.NewOption("Quit", "quit")})
		if err != nil {
			return err
		}
		if action == "quit" {
			return fmt.Errorf("resume cancelled; instance #%d may still be billed", instanceID)
		}
		if action == "destroy" {
			accessibleDestroy, recoveryOnly, workload.RecoveryOnly = true, true, true
		}
	}
	p, ok := deps.Prompter.(ResumePrompter)
	if !ok {
		return fmt.Errorf("resume confirmation is required")
	}
	if !recoveryOnly {
		approved, err := p.ConfirmResume(ctx, instanceID, workload.Recipe, identity)
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("resume cancelled; instance #%d may still be billed", instanceID)
		}
	}
	token, err := vastAPIKey(ctx, deps)
	if err != nil {
		return err
	}
	operator, ok := deps.Prompter.(setup.Operator)
	if !ok {
		return fmt.Errorf("resume setup operator is required")
	}
	result, err := deps.RunVast(ctx, token, identity, workload, operator)
	if err != nil {
		if p, ok := deps.Prompter.(*AccessiblePrompter); ok {
			return p.recoverSetup(ctx, err, token)
		}
		return &redactedSetupError{cause: err, token: token}
	}
	if recoveryOnly {
		if accessibleDestroy {
			return deps.Prompter.(*AccessiblePrompter).recoverSetup(ctx, fmt.Errorf("recovery requested for instance #%d; provisioning was not resumed", instanceID), token)
		}
		return nil
	}
	fmt.Fprintf(output, "Instance #%d resumed. Configuration saved: %s\n", result.InstanceID, result.ConfigPath)
	return nil
}

func (p *AccessiblePrompter) ConfirmResume(ctx context.Context, id int, r recipe.Recipe, identity string) (bool, error) {
	return p.confirm(ctx, fmt.Sprintf("Resume existing instance #%d with %s (%s) using %s? No new instance will be created", id, cleanOfferText(r.Name), cleanOfferText(r.Model.Repository), cleanOfferText(identity)))
}

func (p *HuhPrompter) ConfirmResume(ctx context.Context, id int, r recipe.Recipe, identity string) (bool, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.ConfirmResume(ctx, id, r, identity)
	}
	approved := false
	err := p.run(ctx, huh.NewConfirm().Title(fmt.Sprintf("Resume instance #%d with %s using %s? No new instance will be created", id, cleanOfferText(r.Name), cleanOfferText(identity))).Affirmative("Resume").Negative("Cancel").Value(&approved))
	return approved, err
}
