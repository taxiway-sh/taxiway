package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/event"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

const statusHeader = "%-20s  %-14s  %-10s  %-18s  %-6s  %s\n"

type statusRow struct {
	lab     string
	orch    string
	state   string
	phase   string
	driver  string
	created string
}

// printList lists all labs from the state dir, regardless of which driver
// is active for the session. Discovery uses created_at and the required
// ref.json sidecar. Runtime state is queried via the driver recorded in ref.json.
func printList(ctx context.Context, state *RootState, w io.Writer, onlyLabs ...string) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	routes, err := collectLabProxyRoutes(stateDir)
	if err != nil {
		return err
	}
	proxy, err := state.resolveProxyRuntime()
	if err != nil {
		return err
	}
	state.Proxy = proxy
	rows, err := collectLabRuntimeRows(ctx, state, stateDir, routes, proxy, detectDockerStatus())
	if err != nil {
		return err
	}
	if len(onlyLabs) > 0 {
		only := config.LabDirOf(onlyLabs[0])
		filtered := rows[:0]
		for _, row := range rows {
			if row.lab == onlyLabs[0] || config.LabDirOf(row.lab) == only {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no labs found)")
		return nil
	}
	printLabListTable(w, rows)
	return nil
}

type labRuntimeRow struct {
	lab         string
	orch        string
	status      string
	phase       string
	driver      string
	created     string
	stateDir    string
	gateway     string
	gatewayDir  string
	project     string
	access      string
	shell       string
	adminAPIKey string
	openEnabled bool
	uiURL       string
	services    []proxyStatusService
}

func collectLabRuntimeRows(ctx context.Context, state *RootState, stateDir string, routes []proxyRoute, proxy proxyRuntime, docker dockerRuntimeStatus) ([]labRuntimeRow, error) {
	labs, err := collectLabStatusRows(ctx, state, stateDir)
	if err != nil {
		return nil, err
	}
	gateways, err := collectProxyStatusGateways(stateDir, routes, proxy, docker)
	if err != nil {
		return nil, err
	}
	gatewayByLab := map[string]proxyStatusGateway{}
	for _, gateway := range gateways {
		gatewayByLab[gateway.Lab] = gateway
	}
	rows := make([]labRuntimeRow, 0, len(labs))
	for _, lab := range labs {
		gateway, ok := gatewayByLab[lab.lab]
		gatewayState := "not configured"
		access := "-"
		apiKey := ""
		openEnabled := false
		uiURL := ""
		project := ""
		var services []proxyStatusService
		if ok {
			gatewayState = gateway.Docker
			apiKey = gateway.APIKey
			openEnabled = gateway.OpenEnabled
			uiURL = gateway.UIURL
			project = gateway.Project
			services = gateway.Services
			if gateway.OpenEnabled && gateway.APIURL != "" {
				access = gateway.APIURL
			}
		}
		rows = append(rows, labRuntimeRow{
			lab:         lab.lab,
			orch:        lab.orch,
			status:      labRuntimeStatus(lab.state, lab.phase, gatewayState, ok, proxy, docker),
			phase:       lab.phase,
			driver:      lab.driver,
			created:     lab.created,
			stateDir:    filepath.Join(stateDir, lab.lab),
			gateway:     gatewayState,
			gatewayDir:  filepath.Join(stateDir, lab.lab, "gateway"),
			project:     project,
			access:      access,
			shell:       "taxiway shell " + lab.lab,
			adminAPIKey: apiKey,
			openEnabled: openEnabled,
			uiURL:       uiURL,
			services:    services,
		})
	}
	return rows, nil
}

func collectLabProxyRoutes(stateDir string) ([]proxyRoute, error) {
	labRoutes, err := readLabLiteLLMRoutes(stateDir)
	if err != nil {
		return nil, err
	}
	routes := make([]proxyRoute, 0, len(labRoutes))
	for _, route := range labRoutes {
		routes = append(routes, proxyRouteFromLabLiteLLMRoute(route))
	}
	return routes, nil
}

func collectLabStatusRows(ctx context.Context, state *RootState, stateDir string) ([]statusRow, error) {
	entries, err := os.ReadDir(stateDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	type labEntry struct {
		id  string
		ref config.LabRef
	}
	var labs []labEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		lab := e.Name()
		if config.LabDirOf(lab) != lab {
			continue
		}
		if _, err := os.Stat(filepath.Join(stateDir, lab, "created_at")); err != nil {
			continue
		}
		id := idName(lab)
		ref, ok, err := config.ReadLabRef(stateDir, lab)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("lab %q is missing ref.json", lab)
		}
		labs = append(labs, labEntry{id: id, ref: ref})
	}
	if len(labs) == 0 {
		return nil, nil
	}

	drivers := map[string]driver.Driver{}
	if state.Driver != nil {
		drivers[state.Driver.Name()] = state.Driver
	}
	getDriver := func(name string) (driver.Driver, error) {
		if name == "" {
			return nil, fmt.Errorf("lab ref is missing driver")
		}
		if d, ok := drivers[name]; ok {
			return d, nil
		}
		d, err := newDriverByName(name, stateDir)
		if err != nil {
			return nil, err
		}
		drivers[name] = d
		return d, nil
	}

	sort.Slice(labs, func(i, j int) bool { return labs[i].id < labs[j].id })

	rows := make([]statusRow, 0, len(labs))
	for _, l := range labs {
		d, err := getDriver(l.ref.Driver)
		if err != nil {
			return nil, err
		}
		st, err := labStatus(ctx, stateDir, d, l.id, l.ref)
		if err != nil {
			st = driver.Status{Name: l.id, State: "unknown", Driver: l.ref.Driver}
		}
		rows = append(rows, statusRowFromStatus(st, l.ref, phaseLabel(stateDir, l.id)))
	}
	return rows, nil
}

func labRuntimeStatus(labState, labPhase, gatewayState string, gatewayConfigured bool, proxy proxyRuntime, docker dockerRuntimeStatus) string {
	if labState != "running" {
		return labState
	}
	if labPhase != completedPhaseLabel(phases.PhaseStart) {
		return "degraded"
	}
	if !gatewayConfigured || gatewayState != "running" {
		return "degraded"
	}
	if !docker.Available || proxy.Port == 0 || containerState(proxy.Container) != "running" {
		return "degraded"
	}
	return "running"
}

func labStatus(ctx context.Context, stateDir string, d driver.Driver, id string, ref config.LabRef) (driver.Status, error) {
	createdAt, hasCreatedAt := config.ReadCreatedAt(stateDir, id)
	st, err := d.Status(ctx, id)
	if err != nil {
		return st, err
	}
	if st.Created.IsZero() && hasCreatedAt {
		st.Created = createdAt
	}
	return st, nil
}

func phaseLabel(stateDir, id string) string {
	if _, ok := config.ReadCreatedAt(stateDir, id); !ok {
		return "-"
	}
	label := "initialized"
	for _, phase := range phases.Order {
		if phases.Done(stateDir, id, phase) {
			label = completedPhaseLabel(phase)
		}
	}
	return label
}

func completedPhaseLabel(phase phases.Phase) string {
	switch phase {
	case phases.PhaseCreate:
		return "created"
	case phases.PhaseBootstrap:
		return "bootstrapped"
	case phases.PhaseInstall:
		return "installed"
	case phases.PhaseVerify:
		return "verified"
	case phases.PhaseGateway:
		return "gateway ready"
	case phases.PhaseWorkspace:
		return "workspace created"
	case phases.PhaseAuth:
		return "authenticated"
	case phases.PhaseStart:
		return "started"
	default:
		return "-"
	}
}

func printStatusWithRef(w io.Writer, st driver.Status, ref config.LabRef, phase string) {
	row := statusRowFromStatus(st, ref, phase)
	fmt.Fprintf(w, statusHeader, row.lab, row.orch, row.state, row.phase, row.driver, row.created)
}

func statusRowFromStatus(st driver.Status, ref config.LabRef, phase string) statusRow {
	lab := ref.Lab
	if lab == "" {
		var err error
		lab, err = config.LabNameFromID(st.Name)
		if err != nil {
			lab = st.Name
		}
	}
	orchType := ref.Orch
	if orchType == "" {
		orchType = "-"
	}
	createdStr := "-"
	if !st.Created.IsZero() {
		createdStr = st.Created.UTC().Format("2006-01-02T15:04")
	}
	return statusRow{
		lab:     lab,
		orch:    orchType,
		state:   st.State,
		phase:   phase,
		driver:  st.Driver,
		created: createdStr,
	}
}

func printStatusTable(w io.Writer, rows []statusRow) {
	labW := maxLen("LAB", func(r statusRow) string { return r.lab }, rows)
	orchW := maxLen("TYPE", func(r statusRow) string { return r.orch }, rows)
	stateW := maxLen("STATE", func(r statusRow) string { return r.state }, rows)
	phaseW := maxLen("PHASE", func(r statusRow) string { return r.phase }, rows)
	driverW := maxLen("DRIVER", func(r statusRow) string { return r.driver }, rows)

	fmt.Fprintf(w, statusListFormat(labW, orchW, stateW, phaseW, driverW), "LAB", "TYPE", "STATE", "PHASE", "DRIVER", "CREATED")
	for _, row := range rows {
		fmt.Fprintf(w, statusListFormat(labW, orchW, stateW, phaseW, driverW),
			row.lab, row.orch, row.state, row.phase, row.driver, row.created)
	}
}

func printLabListTable(w io.Writer, rows []labRuntimeRow) {
	labW := maxRuntimeLen("LAB", func(r labRuntimeRow) string { return r.lab }, rows)
	orchW := maxRuntimeLen("TYPE", func(r labRuntimeRow) string { return r.orch }, rows)
	statusW := maxRuntimeLen("STATUS", func(r labRuntimeRow) string { return r.status }, rows)
	phaseW := maxRuntimeLen("PHASE", func(r labRuntimeRow) string { return r.phase }, rows)
	driverW := maxRuntimeLen("DRIVER", func(r labRuntimeRow) string { return r.driver }, rows)
	format := labListFormat(labW, orchW, statusW, phaseW, driverW)

	fmt.Fprintf(w, format, "LAB", "TYPE", "STATUS", "PHASE", "DRIVER", "CREATED")
	for _, row := range rows {
		fmt.Fprintf(w, format, row.lab, row.orch, row.status, row.phase, row.driver, row.created)
	}
}

func printLabRuntimeTable(w io.Writer, rows []labRuntimeRow) {
	labW := maxRuntimeLen("LAB", func(r labRuntimeRow) string { return r.lab }, rows)
	orchW := maxRuntimeLen("TYPE", func(r labRuntimeRow) string { return r.orch }, rows)
	statusW := maxRuntimeLen("STATUS", func(r labRuntimeRow) string { return r.status }, rows)
	phaseW := maxRuntimeLen("PHASE", func(r labRuntimeRow) string { return r.phase }, rows)
	driverW := maxRuntimeLen("DRIVER", func(r labRuntimeRow) string { return r.driver }, rows)
	createdW := maxRuntimeLen("CREATED", func(r labRuntimeRow) string { return r.created }, rows)
	gatewayW := maxRuntimeLen("GATEWAY", func(r labRuntimeRow) string { return r.gateway }, rows)
	format := labRuntimeListFormat(labW, orchW, statusW, phaseW, driverW, createdW, gatewayW)

	fmt.Fprintf(w, format, "LAB", "TYPE", "STATUS", "PHASE", "DRIVER", "CREATED", "GATEWAY", "ACCESS")
	for _, row := range rows {
		fmt.Fprintf(w, format, row.lab, row.orch, row.status, row.phase, row.driver, row.created, row.gateway, row.access)
	}
}

func labListFormat(labW, orchW, statusW, phaseW, driverW int) string {
	return fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n", labW, orchW, statusW, phaseW, driverW)
}

func labRuntimeListFormat(labW, orchW, statusW, phaseW, driverW, createdW, gatewayW int) string {
	return fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n", labW, orchW, statusW, phaseW, driverW, createdW, gatewayW)
}

func maxRuntimeLen(header string, value func(labRuntimeRow) string, rows []labRuntimeRow) int {
	max := len(header)
	for _, row := range rows {
		if n := len(value(row)); n > max {
			max = n
		}
	}
	return max
}

func statusListFormat(labW, orchW, stateW, phaseW, driverW int) string {
	return fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n", labW, orchW, stateW, phaseW, driverW)
}

func maxLen(header string, value func(statusRow) string, rows []statusRow) int {
	max := len(header)
	for _, row := range rows {
		if n := len(value(row)); n > max {
			max = n
		}
	}
	return max
}

// driverForRef returns the driver that owns the lab: the one recorded in ref.json.
func driverForRef(state *RootState, ref config.LabRef) (driver.Driver, error) {
	if ref.Driver == "" {
		return nil, fmt.Errorf("lab %q is missing driver in ref.json", ref.Lab)
	}
	if ref.Driver == state.Driver.Name() {
		return state.Driver, nil
	}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	d, err := newDriverByName(ref.Driver, stateDir)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// execScriptWithRef runs a script inside the lab identified by ref.
func execScriptWithRef(ctx context.Context, state *RootState, ref config.LabRef, scriptPath string, env map[string]string) error {
	return execScriptToWithRef(ctx, state, ref, scriptPath, os.Stdout, os.Stderr, env)
}

func execScriptToWithRef(ctx context.Context, state *RootState, ref config.LabRef, scriptPath string, stdout, stderr io.Writer, env map[string]string) error {
	id := idName(ref.Lab)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	jsonlPath := driver.EventsJSONLPath(stateDir, id)

	var evSink event.Sink = event.DiscardSink{}
	var closer func()
	closer = func() {}

	if !state.Flags.DryRun {
		s, c, err := makeExecSink(jsonlPath)
		if err == nil {
			evSink = s
			closer = c
		}
	}
	defer closer()

	// Translate host path to lab path (e.g. <repo>/infra/commands/bootstrap.sh → /lab/infra/commands/bootstrap.sh).
	labScript, err := hostScriptToLab(state.RepoDir, scriptPath)
	if err != nil {
		return fmt.Errorf("execScript: %w", err)
	}

	// Build argv: prepend env vars as explicit `env KEY=VAL ...` arguments so
	// they are visible to the script inside the lab regardless of SSH/limactl
	// environment forwarding behaviour.
	// Background: `limactl shell id -- bash script.sh` runs over SSH; host
	// process env vars (cmd.Env) are NOT forwarded into the lab shell session
	// by default. Passing them via `env VAR=val bash script.sh` in the argv
	// is the portable solution that works with any SSH configuration.
	var argv []string
	if len(env) > 0 {
		argv = append(argv, "env")
		// Sort keys for deterministic ordering (important for tests and logs).
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, k+"="+env[k])
		}
	}
	argv = append(argv, "bash", labScript)

	req := driver.ExecRequest{
		Workdir: LabRepoRoot,
		Argv:    argv,
		Stdout:  stdout,
		Stderr:  stderr,
		Events:  evSink,
		Env:     env, // still set for drivers that do forward env (e.g. mock)
	}

	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	res, err := d.Exec(ctx, id, req)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("script exited with code %d", res.ExitCode)
	}
	return nil
}

// labUp is the shared create-or-start logic.
// It initializes the ref.json and created_at sidecars before Create, and
// checks anti-collision if the lab already exists.
func labUp(ctx context.Context, state *RootState, ref config.LabRef, logW io.Writer) error {
	if logW == nil {
		logW = os.Stderr
	}
	id := idName(ref.Lab)
	d := state.Driver
	if ref.Driver != "" {
		resolved, err := driverForRef(state, ref)
		if err != nil {
			return err
		}
		d = resolved
	}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	gitDir := filepath.Join(stateDir, ref.Lab, "git")
	recordingsDir := filepath.Join(stateDir, ref.Lab, "recordings")

	exists, err := d.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !state.Flags.DryRun {
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			return fmt.Errorf("labUp: create git dir for %s: %w", id, err)
		}
		if err := os.MkdirAll(recordingsDir, 0o755); err != nil {
			return fmt.Errorf("labUp: create recordings dir for %s: %w", id, err)
		}
	}

	if exists {
		// Anti-collision: if sidecar exists and orch diverges → error with UX message.
		existing, ok, rErr := d.ReadLabRef(ctx, id)
		if rErr != nil {
			return fmt.Errorf("read lab ref for %s: %w", id, rErr)
		}
		if !ok {
			return fmt.Errorf("lab %q is missing ref.json", ref.Lab)
		}
		if existing.Orch != "" && ref.Orch != "" && existing.Orch != ref.Orch {
			fmt.Fprintf(logW,
				"⚠  Lab %q already exists with --type=%s\n   Use: taxiway up %s --type=%s to re-use it, or taxiway rm %s first.\n",
				ref.Lab, existing.Orch, ref.Lab, existing.Orch, ref.Lab,
			)
			return fmt.Errorf(
				"lab %q already exists with orchestrator type %q (requested %q)",
				ref.Lab, existing.Orch, ref.Orch,
			)
		}

		running, err := d.Running(ctx, id)
		if err != nil {
			return err
		}
		if running {
			fmt.Fprintf(logW, "Lab %q is already running\n", ref.Lab)
			return nil
		}
		fmt.Fprintf(logW, "Lab %q exists — starting\n", ref.Lab)
		return d.Start(ctx, id)
	}

	fmt.Fprintf(logW, "Creating lab %q\n", ref.Lab)

	if !state.Flags.DryRun {
		ref.Driver = d.Name()
		if err := d.WriteLabRef(ctx, id, ref); err != nil {
			_ = os.RemoveAll(filepath.Join(stateDir, ref.Lab))
			return fmt.Errorf("labUp: write lab ref for %s: %w", id, err)
		}
		if err := config.EnsureCreatedAt(stateDir, id); err != nil {
			_ = os.RemoveAll(filepath.Join(stateDir, ref.Lab))
			return fmt.Errorf("labUp: write created_at for %s: %w", id, err)
		}
	}

	if err := d.Create(ctx, id, driver.CreateOptions{
		TemplatePath:  limaYAMLTemplate(state.RepoDir),
		RepoDir:       state.RepoDir,
		Orch:          ref.Orch,
		Lab:           ref.Lab,
		GitDir:        gitDir,
		RecordingsDir: recordingsDir,
	}); err != nil {
		return err
	}

	return nil
}
