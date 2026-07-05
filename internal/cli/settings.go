package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
)

type settingsSelection struct {
	values map[string]string
	clears []string
}

func addSetFlags(cmd *cobra.Command, setValues *[]string, clearSet *[]string) {
	cmd.Flags().StringArrayVar(setValues, "set", nil, "orchestrator setting key=value; list supported keys with: taxiway describe <orch>")
	cmd.Flags().StringArrayVar(clearSet, "clear-set", nil, "clear a persisted orchestrator setting by key")
}

func settingsSelectionFromFlags(setValues, clearSet []string) (settingsSelection, error) {
	sel := settingsSelection{
		values: map[string]string{},
		clears: append([]string(nil), clearSet...),
	}
	for _, raw := range setValues {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return settingsSelection{}, fmt.Errorf("--set %q must use key=value", raw)
		}
		if err := validateSettingKey(key); err != nil {
			return settingsSelection{}, fmt.Errorf("invalid --set key %q: %w", key, err)
		}
		sel.values[key] = value
	}
	for _, key := range sel.clears {
		if err := validateSettingKey(key); err != nil {
			return settingsSelection{}, fmt.Errorf("invalid --clear-set key %q: %w", key, err)
		}
	}
	if err := validateSettingEnvCollisions(sel.values); err != nil {
		return settingsSelection{}, err
	}
	return sel, nil
}

func validateSettingKey(key string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("must contain only letters, digits, '_', '-' or '.'")
	}
	return nil
}

func settingEnvName(key string) string {
	var b strings.Builder
	b.WriteString("TAXIWAY_SET_")
	for _, r := range key {
		switch r {
		case '.', '-':
			b.WriteByte('_')
		default:
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

func validateSettingEnvCollisions(settings map[string]string) error {
	seen := map[string]string{}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env := settingEnvName(key)
		if prev, ok := seen[env]; ok && prev != key {
			return fmt.Errorf("setting %q normalizes to %s, which conflicts with %q", key, env, prev)
		}
		seen[env] = key
	}
	return nil
}

func mergeSettings(current map[string]string, sel settingsSelection) (map[string]string, error) {
	if len(current) == 0 && len(sel.values) == 0 && len(sel.clears) == 0 {
		return nil, nil
	}
	next := map[string]string{}
	for key, value := range current {
		next[key] = value
	}
	for _, key := range sel.clears {
		delete(next, key)
	}
	for key, value := range sel.values {
		next[key] = value
	}
	if len(next) == 0 {
		return nil, nil
	}
	if err := validateSettingEnvCollisions(next); err != nil {
		return nil, err
	}
	return next, nil
}

func applySettingsSelection(ctx context.Context, state *RootState, id string, ref *config.LabRef, sel settingsSelection) (bool, error) {
	if len(sel.values) == 0 && len(sel.clears) == 0 {
		return false, nil
	}
	next, err := mergeSettings(ref.Settings, sel)
	if err != nil {
		return false, err
	}
	ref.Settings = next

	exists, err := state.Driver.Exists(ctx, id)
	if err != nil || !exists {
		return true, err
	}
	if err := state.Driver.WriteLabRef(ctx, id, *ref); err != nil {
		return true, fmt.Errorf("persisting orchestrator settings: %w", err)
	}
	return true, nil
}

func applySettingsFromFlags(ctx context.Context, state *RootState, id string, ref *config.LabRef, setValues, clearSet []string) (bool, error) {
	sel, err := settingsSelectionFromFlags(setValues, clearSet)
	if err != nil {
		return false, err
	}
	return applySettingsSelection(ctx, state, id, ref, sel)
}

func injectSettingsEnv(env map[string]string, settings map[string]string) {
	for key, value := range settings {
		env[settingEnvName(key)] = value
	}
}
