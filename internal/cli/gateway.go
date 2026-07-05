package cli

import (
	"context"
	"fmt"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

var (
	ensureLabLangfuseProjectForGateway = ensureLabLangfuseProject
	shouldProvisionLabLangfuseProject  = isLangfuseStackRunning
)

func reconcileGateway(ctx context.Context, state *RootState, ref config.LabRef) error {
	if state.Driver.Name() != "mock" && !state.Flags.DryRun {
		proxy, err := state.ensureProxyRuntime()
		if err != nil {
			return err
		}
		stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
		if _, err := ensureProxyConfigForState(state, stateDir, proxy.StateDir); err != nil {
			return err
		}
		if _, _, err := ensureProxyRunning(state, proxy.StateDir); err != nil {
			return err
		}
	}
	if err := ensureLabGatewayEnv(state, ref); err != nil {
		return err
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	if shouldProvisionLabLangfuseProject(state) {
		if err := ensureLabLangfuseProjectForGateway(state, stateDir, observabilityStateDir(state), ref); err != nil {
			return err
		}
	}

	if err := ensureLabLiteLLMSidecarForUp(ctx, state, ref); err != nil {
		return err
	}

	values, err := readLabGatewayEnv(stateDir, ref)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for _, key := range []string{labLiteLLMAPIKeyEnv, labLiteLLMBaseURLEnv} {
		if values[key] != "" {
			env[key] = values[key]
		}
	}

	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	return writeAndCopyGatewayEnv(ctx, d, ref, env)
}

func isLangfuseStackRunning(state *RootState) bool {
	if !fileExists(observabilityEnvPath(state)) {
		return false
	}
	runtime := state.observabilityRuntime()
	if !observabilityRuntimeInitialized(runtime) {
		return false
	}
	docker := detectDockerStatus()
	if !docker.Available {
		return false
	}
	status, _ := summarizeLangfuseStatus(true, true, docker, langfuseServiceStates(runtime, docker))
	return status == "running"
}

func writeAndCopyGatewayEnv(ctx context.Context, d driver.Driver, ref config.LabRef, env map[string]string) error {
	existing, err := readExistingLabEnvFile(ctx, d, ref)
	if err != nil {
		return fmt.Errorf("gateway: read existing lab env failed: %w", err)
	}
	content := upsertManagedEnvBlock(existing, "gateway", "gateway", renderManagedEnvBlock("gateway", "gateway", env))
	if err := writeAndCopyEnvContentWithLabel(ctx, d, ref, content, len(env), "gateway"); err != nil {
		return fmt.Errorf("gateway: write lab env failed: %w", err)
	}
	return nil
}
