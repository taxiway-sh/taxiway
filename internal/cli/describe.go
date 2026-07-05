package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func newDescribeCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:               "describe <orch>",
		Short:             "Describe an orchestrator",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeOrchestrators(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return config.ValidateOrchName(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			orch := args[0]
			if _, err := config.InstallScript(state.RepoDir, orch); err != nil {
				return err
			}
			manifest, err := config.LoadOrchManifest(state.RepoDir, orch)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Orchestrator: %s\n", orch)
			if manifest != nil && manifest.Description != "" {
				fmt.Fprintf(out, "Description: %s\n", manifest.Description)
			}
			if manifest != nil && manifest.DocsURL != "" {
				fmt.Fprintf(out, "Docs: %s\n", manifest.DocsURL)
			}
			liteLLMModels, err := describeLiteLLMModels(state, manifest)
			if err != nil {
				return err
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Settings:")
			if manifest == nil || len(manifest.Settings) == 0 {
				fmt.Fprintln(out, "  (none declared)")
			} else {
				for _, setting := range manifest.Settings {
					fmt.Fprintf(out, "  %s\n", setting.Name)
					if setting.Description != "" {
						fmt.Fprintf(out, "    %s\n", setting.Description)
					}
					if setting.Default != "" {
						fmt.Fprintf(out, "    Default: %s\n", setting.Default)
					}
					if len(setting.Phases) > 0 {
						fmt.Fprintf(out, "    Phases: %s\n", strings.Join(setting.Phases, ", "))
					}
					examples := setting.Examples
					if setting.Name == "model" && len(liteLLMModels) > 0 {
						examples = describeModelExamples(setting.Default, liteLLMModels)
					}
					if len(examples) > 0 {
						fmt.Fprintln(out, "    Examples:")
						for _, example := range examples {
							fmt.Fprintf(out, "      --set %s=%s\n", setting.Name, example)
						}
					}
				}
			}
			if len(liteLLMModels) > 0 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "Available LiteLLM models:")
				for _, model := range liteLLMModels {
					fmt.Fprintf(out, "  %s\n", model)
				}
			}
			return nil
		},
	}
}

func describeLiteLLMModels(state *RootState, manifest *config.OrchManifest) ([]string, error) {
	if manifest == nil || len(manifest.Agents) == 0 {
		return nil, nil
	}
	providers := map[string]bool{}
	for _, agent := range manifest.Agents {
		agentManifest, err := config.LoadAgentManifest(state.RepoDir, agent)
		if err != nil {
			return nil, err
		}
		if agentManifest == nil || agentManifest.LiteLLM == nil {
			continue
		}
		for _, provider := range agentManifest.LiteLLM.Providers {
			providers[provider] = true
		}
	}
	if len(providers) == 0 {
		return nil, nil
	}

	data, err := os.ReadFile(liteLLMModelsAssetPath(state))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading LiteLLM model catalog: %w", err)
	}
	catalog, err := parseLiteLLMModelCatalog(data)
	if err != nil {
		return nil, err
	}

	models := []string{}
	for _, model := range catalog.Models {
		if providers[model.Provider] {
			models = append(models, model.Name)
		}
	}
	return models, nil
}

func describeModelExamples(defaultModel string, models []string) []string {
	for _, model := range models {
		if model != defaultModel {
			return []string{model}
		}
	}
	if len(models) > 0 {
		return []string{models[0]}
	}
	return nil
}
