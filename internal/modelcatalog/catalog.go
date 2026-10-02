// Package modelcatalog describes the models Taxiway can route through its gateway.
package modelcatalog

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Defaults map[string]map[string]string `yaml:"defaults,omitempty"`
	Models   []Model                      `yaml:"models"`
}

type Model struct {
	Name                 string `yaml:"name"`
	Provider             string `yaml:"provider"`
	Upstream             string `yaml:"upstream"`
	APIBase              string `yaml:"api_base,omitempty"`
	APIKey               string `yaml:"api_key,omitempty"`
	API                  string `yaml:"api,omitempty"`
	ForwardClientHeaders bool   `yaml:"forward_client_headers,omitempty"`
	Status               string `yaml:"status,omitempty"`
	RetirementDate       string `yaml:"retirement_date,omitempty"`
	Replacement          string `yaml:"replacement,omitempty"`
	Source               string `yaml:"source,omitempty"`
}

func Parse(data []byte) (Catalog, error) {
	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return catalog, fmt.Errorf("parsing LiteLLM model catalog: %w", err)
	}
	seen := map[string]Model{}
	for _, model := range catalog.Models {
		if model.Name == "" || model.Provider == "" || model.Upstream == "" {
			return catalog, fmt.Errorf("model %q requires name, provider and upstream", model.Name)
		}
		if _, exists := seen[model.Name]; exists {
			return catalog, fmt.Errorf("duplicate model %q", model.Name)
		}
		switch model.Status {
		case "", "active", "deprecated", "retired":
		default:
			return catalog, fmt.Errorf("model %q has invalid status %q", model.Name, model.Status)
		}
		if model.RetirementDate != "" {
			if _, err := time.Parse("2006-01-02", model.RetirementDate); err != nil {
				return catalog, fmt.Errorf("model %q has invalid retirement date: %w", model.Name, err)
			}
		}
		seen[model.Name] = model
	}
	for provider, aliases := range catalog.Defaults {
		for alias, name := range aliases {
			model, exists := seen[name]
			if !exists || model.Provider != provider || model.StatusAt(time.Now()) == "retired" {
				return catalog, fmt.Errorf("default %s.%s must reference an available %s model, got %q", provider, alias, provider, name)
			}
		}
	}
	return catalog, nil
}

func (model Model) StatusAt(now time.Time) string {
	if model.Status == "retired" {
		return "retired"
	}
	if model.RetirementDate != "" {
		date, err := time.Parse("2006-01-02", model.RetirementDate)
		if err == nil && !now.Before(date) {
			return "retired"
		}
	}
	if model.Status == "" {
		return "active"
	}
	return model.Status
}

func (model Model) SelectionError(now time.Time) error {
	if model.StatusAt(now) != "retired" {
		return nil
	}
	if model.Replacement != "" {
		return fmt.Errorf("model %q is retired; select %q instead", model.Name, model.Replacement)
	}
	return fmt.Errorf("model %q is retired", model.Name)
}
