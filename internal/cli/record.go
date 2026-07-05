package cli

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"charm.land/glamour/v2"
	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/recording"
)

const labRecordingDir = LabRecordingsRoot

const (
	recordPlayerPortFile     = "record-player.port"
	recordPlayerHostPortBase = 48080
	recordPlayerHostPortSpan = 16000
	recordingDefaultCols     = 160
	recordingDefaultRows     = 48
)

const analyzeProgressClearLine = "\x1b[2K\r"

//go:embed prompts/*.tmpl
var recordPromptTemplates embed.FS

func newRecordCmd(state *RootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Manage lab recordings",
	}
	cmd.AddCommand(
		newRecordStartCmd(state),
		newRecordStopCmd(state),
		newRecordListCmd(state),
		newRecordRmCmd(state),
		newRecordPlayerCmd(state),
		newRecordAnalyzeCmd(state),
	)
	return cmd
}

func newRecordStartCmd(state *RootState) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:               "start <lab>",
		Short:             "Start recording a lab shell",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecordStart(cmd, state, args[0], name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "recording name (default: UTC timestamp)")
	return cmd
}

func newRecordStopCmd(state *RootState) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:               "stop <lab>",
		Short:             "Stop a running lab recording",
		Long:              "Stop a running lab recording.\n\nWhen --name is omitted, Taxiway stops the latest active recording.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecordStop(cmd, state, args[0], name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "recording name to stop")
	return cmd
}

func newRecordListCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:               "list [lab]",
		Aliases:           []string{"ls"},
		Short:             "List lab recordings",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "help" {
				return cmd.Help()
			}
			if len(args) > 0 {
				return validateLabArg(args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runRecordListAll(cmd, state)
			}
			return runRecordList(cmd, state, args[0])
		},
	}
}

func newRecordRmCmd(state *RootState) *cobra.Command {
	var force bool
	var name string
	cmd := &cobra.Command{
		Use:               "rm <lab> --name <name>",
		Aliases:           []string{"remove", "delete"},
		Short:             "Remove a lab recording",
		Long:              "Remove a lab recording.\n\nRemoval is explicit: pass --name to select the recording. Use --force to stop an active recording before removing it.",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			if err := validateLabArg(args[0]); err != nil {
				return err
			}
			if len(args) == 2 {
				if name != "" {
					return fmt.Errorf("use either --name or positional recording name, not both")
				}
				name = args[1]
			}
			return recording.ValidateName(name)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecordRm(cmd, state, args[0], name, force)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "recording name to remove")
	cmd.Flags().BoolVar(&force, "force", false, "stop an active recording before removing it")
	return cmd
}

func newRecordPlayerCmd(state *RootState) *cobra.Command {
	var port int
	var noOpen bool
	var writeOnly bool
	cmd := &cobra.Command{
		Use:               "player <lab>",
		Short:             "Serve the browser player for lab recordings",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		Annotations:       map[string]string{"skipDriver": "true"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecordPlayer(cmd, state, args[0], port, !noOpen, writeOnly)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "local HTTP port for the recording player (default: stable per lab)")
	cmd.Flags().Lookup("port").DefValue = "auto"
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open the player in the default browser")
	cmd.Flags().BoolVar(&writeOnly, "write-only", false, "write index.html without starting the HTTP server")
	return cmd
}

func newRecordAnalyzeCmd(state *RootState) *cobra.Command {
	var recordName string
	var promptOnly bool
	var interactive bool
	var pretty bool
	var language string
	var runnerName string
	var noProgress bool
	detailLevel := analysisDetailSummary
	cmd := &cobra.Command{
		Use:     "analyze <lab>",
		Aliases: []string{"analyse"},
		Short:   "Analyze lab recordings",
		Long: `Analyze lab recordings with a local agent runner.

A runner is a local CLI, such as Codex or Claude Code, that receives the
generated analysis prompt and returns a first-pass analysis.

Prompt-only mode:
  Use --prompt-only to print the generated prompt and recording references
  without launching an agent runner.

Interactive mode:
  Use --interactive to launch the selected agent runner in its native
  interactive UI with the generated prompt as initial context.

Runner selection:
  Use --runner <name> for the current invocation, or set TAXIWAY_ANALYZE_RUNNER
  when --runner is absent. If neither is set, the taxiway CLI tries to detect installed
  runners locally.

Progress:
  In an interactive terminal, the taxiway CLI writes progress messages to stderr while the
  runner is working. The final analysis remains on stdout. Use --no-progress to
  disable these messages.

Pretty output:
  Use --pretty to render the final Markdown analysis for the terminal. This
  option cannot be combined with --prompt-only.

Language:
  Use --language <code> to request an output language. Values must use a simple
  language subtag such as "en" (English), "ja" (Japanese), or "kok" (Konkani).
  When absent, the taxiway CLI derives the language from LC_ALL, LC_MESSAGES, then LANG.

Supported runners:
  codex        Uses the local codex CLI
  claude-code  Uses the local claude CLI`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		Annotations:       map[string]string{"skipDriver": "true"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			if err := validateLabArg(args[0]); err != nil {
				return err
			}
			if recordName != "" {
				if err := recording.ValidateName(recordName); err != nil {
					return err
				}
			}
			if promptOnly && pretty {
				return fmt.Errorf("--pretty cannot be used with --prompt-only")
			}
			if promptOnly && interactive {
				return fmt.Errorf("--interactive cannot be used with --prompt-only")
			}
			if _, err := resolveExplicitAnalysisLanguage(language); err != nil {
				return err
			}
			return validateAnalysisDetailLevel(detailLevel)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecordAnalyze(cmd, state, args[0], recordName, promptOnly, runnerName, detailLevel, !noProgress, pretty, interactive, language)
		},
	}
	cmd.Flags().StringVar(&recordName, "record", "", "recording name to analyze")
	cmd.Flags().BoolVar(&promptOnly, "prompt-only", false, "print the generated prompt without launching an agent runner")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "launch the selected runner in its native interactive UI")
	cmd.Flags().StringVar(&runnerName, "runner", "", "agent runner for this invocation (codex or claude-code; overrides TAXIWAY_ANALYZE_RUNNER)")
	cmd.Flags().StringVar(&detailLevel, "detail", detailLevel, "analysis detail level (summary or full)")
	cmd.Flags().StringVar(&language, "language", "", `analysis output language code (for example "en" (English), "ja" (Japanese), or "kok" (Konkani))`)
	cmd.Flags().BoolVar(&noProgress, "no-progress", false, "disable progress messages on stderr")
	cmd.Flags().BoolVar(&pretty, "pretty", false, "render the final Markdown analysis for the terminal")
	return cmd
}

func runRecordStart(cmd *cobra.Command, state *RootState, lab, name string) error {
	ctx := context.Background()
	driverID := idName(lab)
	ref, err := loadLabRef(ctx, state, driverID)
	if err != nil {
		return err
	}
	target, err := resolveShellTarget(ctx, state, ref)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if name == "" {
		name = recording.DefaultName(now)
	}
	recordingID, err := recording.NewID(now, name)
	if err != nil {
		return err
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	idx, err := store.Load()
	if err != nil {
		return err
	}
	if active, ok := idx.LatestActive(); ok {
		return fmt.Errorf("recording %q is already active for lab %q; stop it before starting a new one", active.Name, lab)
	}

	recorderSession := "taxiway-record-" + recordingID
	castlab := labRecordingDir + "/" + recordingID + ".cast"
	castHost := filepath.Join(stateDir, lab, "recordings", recordingID+".cast")

	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	if err := startRecordingProcess(ctx, d, driverID, target, recorderSession, castlab); err != nil {
		return err
	}

	session := recording.Session{
		ID:              recordingID,
		Name:            name,
		Lab:             lab,
		Driver:          d.Name(),
		DriverID:        driverID,
		State:           recording.StateRecording,
		ShellCommand:    target.AttachCommand,
		RecorderSession: recorderSession,
		CastPath:        castlab,
		CastPathHost:    castHost,
		StartedAt:       now,
	}
	idx.Upsert(session)
	if err := store.Save(idx); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Recording started: %s\nCast: %s\n", name, castHost)
	return nil
}

func runRecordStop(cmd *cobra.Command, state *RootState, lab, name string) error {
	ctx := context.Background()
	id := idName(lab)
	ref, err := loadLabRef(ctx, state, id)
	if err != nil {
		return err
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	idx, err := store.Load()
	if err != nil {
		return err
	}

	var session recording.Session
	var ok bool
	switch {
	case name != "":
		session, ok = idx.ActiveByName(name)
	case name == "":
		session, ok = idx.LatestActive()
	}
	if !ok {
		return fmt.Errorf("no active recording found for lab %q", lab)
	}

	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	if err := stopRecordingProcess(ctx, d, id, session.RecorderSession); err != nil {
		return err
	}

	now := time.Now().UTC()
	session.State = recording.StateStopped
	session.StoppedAt = &now
	idx.Upsert(session)
	if err := store.Save(idx); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recording stopped: %s\nCast: %s\n", session.Name, session.CastPathHost)
	return nil
}

func runRecordList(cmd *cobra.Command, state *RootState, lab string) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	idx, err := store.Load()
	if err != nil {
		return err
	}
	if len(idx.Sessions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no recordings found)")
		return nil
	}
	rows := make([]recordListRow, 0, len(idx.Sessions))
	for _, session := range idx.Sessions {
		rows = append(rows, recordListRowFromSession(session))
	}
	printRecordListRows(cmd.OutOrStdout(), false, rows)
	return nil
}

func runRecordListAll(cmd *cobra.Command, state *RootState) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	entries, err := os.ReadDir(stateDir)
	if os.IsNotExist(err) {
		fmt.Fprintln(cmd.OutOrStdout(), "(no recordings found)")
		return nil
	}
	if err != nil {
		return err
	}

	var sessions []recording.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		store := recording.NewStore(stateDir, entry.Name())
		idx, err := store.Load()
		if err != nil {
			return err
		}
		sessions = append(sessions, idx.Sessions...)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no recordings found)")
		return nil
	}

	rows := make([]recordListRow, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, recordListRowFromSession(session))
	}
	printRecordListRows(cmd.OutOrStdout(), true, rows)
	return nil
}

type recordListRow struct {
	lab     string
	name    string
	state   string
	started string
	stopped string
	cast    string
}

func recordListRowFromSession(session recording.Session) recordListRow {
	stopped := "-"
	if session.StoppedAt != nil {
		stopped = session.StoppedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return recordListRow{
		lab:     session.Lab,
		name:    session.Name,
		state:   string(session.State),
		started: session.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		stopped: stopped,
		cast:    recordingCastDisplay(session),
	}
}

func printRecordListRows(w io.Writer, includeLab bool, rows []recordListRow) {
	nameW := maxRecordListLen("NAME", func(row recordListRow) string { return row.name }, rows)
	stateW := maxRecordListLen("STATE", func(row recordListRow) string { return row.state }, rows)
	startedW := maxRecordListLen("STARTED", func(row recordListRow) string { return row.started }, rows)
	stoppedW := maxRecordListLen("STOPPED", func(row recordListRow) string { return row.stopped }, rows)

	if includeLab {
		labW := maxRecordListLen("LAB", func(row recordListRow) string { return row.lab }, rows)
		format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n", labW, nameW, stateW, startedW, stoppedW)
		fmt.Fprintf(w, format, "LAB", "NAME", "STATE", "STARTED", "STOPPED", "CAST")
		for _, row := range rows {
			fmt.Fprintf(w, format, row.lab, row.name, row.state, row.started, row.stopped, row.cast)
		}
		return
	}

	format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n", nameW, stateW, startedW, stoppedW)
	fmt.Fprintf(w, format, "NAME", "STATE", "STARTED", "STOPPED", "CAST")
	for _, row := range rows {
		fmt.Fprintf(w, format, row.name, row.state, row.started, row.stopped, row.cast)
	}
}

func maxRecordListLen(header string, value func(recordListRow) string, rows []recordListRow) int {
	max := len(header)
	for _, row := range rows {
		if n := len(value(row)); n > max {
			max = n
		}
	}
	return max
}

func runRecordRm(cmd *cobra.Command, state *RootState, lab, name string, force bool) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	idx, err := store.Load()
	if err != nil {
		return err
	}
	session, ok := idx.RemoveByName(name)
	if !ok {
		return fmt.Errorf("recording %q not found for lab %q", name, lab)
	}
	if session.State == recording.StateRecording {
		if !force {
			idx.Upsert(session)
			return fmt.Errorf("recording %q is active for lab %q; stop it before removing it", name, lab)
		}
		ref, err := loadLabRef(context.Background(), state, idName(lab))
		if err != nil {
			idx.Upsert(session)
			return err
		}
		d, err := driverForRef(state, ref)
		if err != nil {
			idx.Upsert(session)
			return err
		}
		if err := stopRecordingProcess(context.Background(), d, session.DriverID, session.RecorderSession); err != nil {
			idx.Upsert(session)
			return err
		}
	}
	if session.CastPathHost != "" {
		if err := os.Remove(session.CastPathHost); err != nil && !os.IsNotExist(err) {
			idx.Upsert(session)
			return fmt.Errorf("remove cast %s: %w", session.CastPathHost, err)
		}
	}
	if err := store.Save(idx); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recording removed: %s\n", session.Name)
	return nil
}

func recordingCastDisplay(session recording.Session) string {
	if session.CastPathHost == "" {
		return "-"
	}
	return filepath.Base(session.CastPathHost)
}

func runRecordAnalyze(cmd *cobra.Command, state *RootState, lab, recordName string, promptOnly bool, runnerName string, detailLevel string, progressAllowed bool, pretty bool, interactive bool, language string) error {
	progress := newAnalyzeProgress(cmd, progressAllowed && !promptOnly)
	progress.Stepf("Preparing record analysis for lab %q.", lab)

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	idx, err := store.Load()
	if err != nil {
		return err
	}

	sessions, err := selectAnalysisRecordings(lab, idx, recordName)
	if err != nil {
		return err
	}
	progress.Stepf("Selected %d stopped recording(s).", len(sessions))
	resolvedLanguage, err := resolveAnalysisLanguage(language, os.Getenv)
	if err != nil {
		return err
	}
	prompt, err := buildRecordAnalysisPrompt(recordAnalysisPromptOptions{
		Lab:           lab,
		RecordingsDir: store.Dir(),
		Sessions:      sessions,
		DetailLevel:   detailLevel,
		Language:      resolvedLanguage,
	})
	if err != nil {
		return err
	}
	progress.Stepf("Built analysis prompt. Detail: %s.", detailLevel)
	if promptOnly {
		fmt.Fprint(cmd.OutOrStdout(), prompt)
		return nil
	}

	runner, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Explicit:    runnerName,
		Env:         os.Getenv("TAXIWAY_ANALYZE_RUNNER"),
		Lookup:      exec.LookPath,
		Interactive: analyzeRunnerInteractiveInput(cmd),
		ClearPrompt: analyzeRunnerInteractiveOutput(cmd),
		In:          cmd.InOrStdin(),
		Out:         cmd.OutOrStdout(),
	})
	if err != nil {
		return err
	}
	progress.Stepf("Selected runner: %s.", runner.Name)
	if interactive {
		if !analyzeRunnerInteractiveInput(cmd) || !analyzeRunnerInteractiveOutput(cmd) {
			return fmt.Errorf("--interactive requires interactive stdin and stdout")
		}
		return runAnalyzeRunnerInteractive(runAnalyzeRunnerInteractiveOptions{
			Runner: runner,
			Prompt: prompt,
			Exec:   defaultAnalyzeRunnerInteractiveExec,
			In:     cmd.InOrStdin(),
			Out:    cmd.OutOrStdout(),
			Err:    cmd.ErrOrStderr(),
		})
	}
	return runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner:   runner,
		Prompt:   prompt,
		Out:      cmd.OutOrStdout(),
		Exec:     defaultAnalyzeRunnerExec,
		Progress: progress,
		Pretty:   pretty,
	})
}

func selectAnalysisRecordings(lab string, idx recording.Index, recordName string) ([]recording.Session, error) {
	if recordName != "" {
		for _, session := range idx.Sessions {
			if session.Name != recordName {
				continue
			}
			if session.State != recording.StateStopped {
				return nil, fmt.Errorf("recording %q for lab %q is not stopped; stop it before analysis", recordName, lab)
			}
			return []recording.Session{session}, nil
		}
		return nil, fmt.Errorf("recording %q not found for lab %q", recordName, lab)
	}

	var selected []recording.Session
	for _, session := range idx.Sessions {
		if session.State == recording.StateStopped {
			selected = append(selected, session)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no stopped recordings found for lab %q; stop a recording first or choose a stopped recording with --record", lab)
	}
	return selected, nil
}

const (
	analysisDetailSummary = "summary"
	analysisDetailFull    = "full"
)

var analysisLanguagePattern = regexp.MustCompile(`^[a-zA-Z]{2,8}$`)

func validateAnalysisDetailLevel(level string) error {
	switch level {
	case analysisDetailSummary, analysisDetailFull:
		return nil
	default:
		return fmt.Errorf("unsupported analysis detail level %q; valid values: %s, %s", level, analysisDetailSummary, analysisDetailFull)
	}
}

func resolveAnalysisLanguage(explicit string, getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if explicit != "" {
		return resolveExplicitAnalysisLanguage(explicit)
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			continue
		}
		if isPosixLocale(value) {
			return "", nil
		}
		language := value
		if i := strings.IndexAny(language, "_.@-"); i >= 0 {
			language = language[:i]
		}
		if isPosixLocale(language) {
			return "", nil
		}
		normalized, err := normalizeAnalysisLanguage(language)
		if err != nil {
			continue
		}
		return normalized, nil
	}
	return "", nil
}

func resolveExplicitAnalysisLanguage(language string) (string, error) {
	if strings.TrimSpace(language) == "" {
		return "", nil
	}
	return normalizeAnalysisLanguage(language)
}

func isPosixLocale(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return value == "C" || value == "POSIX"
}

func normalizeAnalysisLanguage(language string) (string, error) {
	language = strings.TrimSpace(language)
	if !analysisLanguagePattern.MatchString(language) {
		return "", fmt.Errorf("unsupported analysis language %q; expected [a-zA-Z]{2,8}", language)
	}
	return strings.ToLower(language), nil
}

type recordAnalysisPromptOptions struct {
	Lab           string
	RecordingsDir string
	Sessions      []recording.Session
	DetailLevel   string
	Language      string
}

func buildRecordAnalysisPrompt(opts recordAnalysisPromptOptions) (string, error) {
	prompt, err := renderRecordAnalysisPrompt(opts)
	if err != nil {
		return "", fmt.Errorf("render record analysis prompt: %w", err)
	}
	return prompt, nil
}

type recordAnalysisPromptData struct {
	Lab           string
	RecordingsDir string
	Sessions      []recordAnalysisPromptSession
	FullDetail    bool
	Language      string
}

type recordAnalysisPromptSession struct {
	Name         string
	State        string
	CastPathHost string
	CastPath     string
	StartedAt    string
	StoppedAt    string
	ShellCommand string
}

func renderRecordAnalysisPrompt(opts recordAnalysisPromptOptions) (string, error) {
	data := recordAnalysisPromptData{
		Lab:           opts.Lab,
		RecordingsDir: opts.RecordingsDir,
		FullDetail:    opts.DetailLevel == analysisDetailFull,
		Language:      opts.Language,
	}
	for _, session := range opts.Sessions {
		stopped := "-"
		if session.StoppedAt != nil {
			stopped = session.StoppedAt.UTC().Format(time.RFC3339)
		}
		data.Sessions = append(data.Sessions, recordAnalysisPromptSession{
			Name:         session.Name,
			State:        string(session.State),
			CastPathHost: session.CastPathHost,
			CastPath:     session.CastPath,
			StartedAt:    session.StartedAt.UTC().Format(time.RFC3339),
			StoppedAt:    stopped,
			ShellCommand: session.ShellCommand,
		})
	}
	tmpl, err := template.ParseFS(recordPromptTemplates, "prompts/record_analysis.md.tmpl")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

type analyzeRunner struct {
	Name              string
	Command           string
	Args              []string
	InteractiveArgs   []string
	OutputLastMessage bool
}

type analyzeRunnerSelectionOptions struct {
	Explicit    string
	Env         string
	Lookup      func(string) (string, error)
	Interactive bool
	ClearPrompt bool
	In          io.Reader
	Out         io.Writer
}

var supportedAnalyzeRunners = []analyzeRunner{
	{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
	{Name: "claude-code", Command: "claude", Args: []string{"-p"}},
}

var defaultAnalyzeRunnerExec = execAnalyzeRunner

var defaultAnalyzeRunnerInteractiveExec = execAnalyzeRunnerInteractive

var analyzeRunnerInteractiveInput = defaultAnalyzeRunnerInteractiveInput

var analyzeRunnerInteractiveOutput = defaultAnalyzeRunnerInteractiveOutput

var analyzeProgressEnabled = defaultAnalyzeProgressEnabled

var analyzeProgressInterval = 10 * time.Second

func defaultAnalyzeRunnerInteractiveInput(cmd *cobra.Command) bool {
	return cobraStreamIsCharDevice(cmd.InOrStdin())
}

func defaultAnalyzeRunnerInteractiveOutput(cmd *cobra.Command) bool {
	return cobraStreamIsCharDevice(cmd.OutOrStdout())
}

func cobraStreamIsCharDevice(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func defaultAnalyzeProgressEnabled(cmd *cobra.Command) bool {
	file, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type analyzeProgressReporter struct {
	out      io.Writer
	enabled  bool
	started  time.Time
	interval time.Duration
	dynamic  bool
}

func newAnalyzeProgress(cmd *cobra.Command, allowed bool) *analyzeProgressReporter {
	enabled := allowed && analyzeProgressEnabled(cmd)
	return &analyzeProgressReporter{
		out:      cmd.ErrOrStderr(),
		enabled:  enabled,
		started:  time.Now(),
		interval: analyzeProgressInterval,
	}
}

func (p *analyzeProgressReporter) Stepf(format string, args ...any) {
	if p == nil || !p.enabled {
		return
	}
	p.clearDynamicLine()
	fmt.Fprintf(p.out, format+"\n", args...)
}

func (p *analyzeProgressReporter) Statusf(format string, args ...any) {
	if p == nil || !p.enabled {
		return
	}
	p.dynamic = true
	fmt.Fprint(p.out, analyzeProgressClearLine)
	fmt.Fprintf(p.out, format, args...)
}

func (p *analyzeProgressReporter) clearDynamicLine() {
	if !p.dynamic {
		return
	}
	fmt.Fprint(p.out, analyzeProgressClearLine)
	p.dynamic = false
}

func (p *analyzeProgressReporter) StartRunner(runner string) func() {
	if p == nil || !p.enabled {
		return func() {}
	}
	p.Stepf("")
	p.Statusf("Running analysis with %s. Duration varies by runner and recording size.", runner)
	if p.interval <= 0 {
		return func() {
			p.clearDynamicLine()
			p.Stepf("Analysis finished in %s.", formatAnalysisElapsed(time.Since(p.started)))
			p.Stepf("")
		}
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.Statusf("Still analyzing with %s... elapsed %s.", runner, formatAnalysisElapsed(time.Since(p.started)))
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
		p.clearDynamicLine()
		p.Stepf("Analysis finished in %s.", formatAnalysisElapsed(time.Since(p.started)))
		p.Stepf("")
	}
}

func formatAnalysisElapsed(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

func selectAnalyzeRunner(opts analyzeRunnerSelectionOptions) (analyzeRunner, error) {
	lookup := opts.Lookup
	if lookup == nil {
		lookup = exec.LookPath
	}
	if opts.Explicit != "" {
		return validateAvailableAnalyzeRunner(opts.Explicit, lookup)
	}
	if opts.Env != "" {
		return validateAvailableAnalyzeRunner(opts.Env, lookup)
	}

	var detected []analyzeRunner
	for _, runner := range supportedAnalyzeRunners {
		if _, err := lookup(runner.Command); err == nil {
			detected = append(detected, runner)
		}
	}
	switch len(detected) {
	case 0:
		return analyzeRunner{}, fmt.Errorf("no supported analysis runner detected; install one of %s or pass --runner / TAXIWAY_ANALYZE_RUNNER", supportedAnalyzeRunnerNames())
	case 1:
		return detected[0], nil
	default:
		if !opts.Interactive {
			return analyzeRunner{}, fmt.Errorf("multiple analysis runners detected (%s); pass --runner or TAXIWAY_ANALYZE_RUNNER", runnerNames(detected))
		}
		return promptForAnalyzeRunner(detected, opts.In, opts.Out, opts.ClearPrompt)
	}
}

func validateAvailableAnalyzeRunner(name string, lookup func(string) (string, error)) (analyzeRunner, error) {
	for _, runner := range supportedAnalyzeRunners {
		if runner.Name != name {
			continue
		}
		if _, err := lookup(runner.Command); err != nil {
			return analyzeRunner{}, fmt.Errorf("analysis runner %q requires command %q on PATH", runner.Name, runner.Command)
		}
		return runner, nil
	}
	return analyzeRunner{}, fmt.Errorf("unsupported analysis runner %q; valid values: %s", name, supportedAnalyzeRunnerNames())
}

func promptForAnalyzeRunner(runners []analyzeRunner, in io.Reader, out io.Writer, clearPrompt bool) (analyzeRunner, error) {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintln(out, "Choose analysis runner:")
	for i, runner := range runners {
		fmt.Fprintf(out, "  %d. %s\n", i+1, runner.Name)
	}
	fmt.Fprint(out, "Runner: ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return analyzeRunner{}, fmt.Errorf("read analysis runner choice: %w", err)
		}
		return analyzeRunner{}, fmt.Errorf("no analysis runner selected")
	}
	var choice int
	if _, err := fmt.Sscanf(strings.TrimSpace(scanner.Text()), "%d", &choice); err != nil || choice < 1 || choice > len(runners) {
		return analyzeRunner{}, fmt.Errorf("invalid analysis runner choice")
	}
	if clearPrompt {
		clearAnalyzeRunnerPrompt(out, len(runners)+2)
	}
	return runners[choice-1], nil
}

func clearAnalyzeRunnerPrompt(out io.Writer, lines int) {
	if lines <= 0 {
		return
	}
	fmt.Fprintf(out, "\x1b[%dA\x1b[J", lines)
}

func supportedAnalyzeRunnerNames() string {
	return runnerNames(supportedAnalyzeRunners)
}

func runnerNames(runners []analyzeRunner) string {
	names := make([]string, 0, len(runners))
	for _, runner := range runners {
		names = append(names, runner.Name)
	}
	return strings.Join(names, ", ")
}

type analyzeRunnerExecution struct {
	Command    string
	Args       []string
	Stdin      string
	Stdout     io.Writer
	Stderr     io.Writer
	OutputPath string
}

type analyzeRunnerInteractiveExecution struct {
	Command string
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

type runAnalyzeRunnerOptions struct {
	Runner   analyzeRunner
	Prompt   string
	Out      io.Writer
	Exec     func(analyzeRunnerExecution) error
	Progress *analyzeProgressReporter
	Pretty   bool
}

var renderAnalysisMarkdown = renderAnalysisMarkdownWithGlamour

type runAnalyzeRunnerInteractiveOptions struct {
	Runner analyzeRunner
	Prompt string
	Exec   func(analyzeRunnerInteractiveExecution) error
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
}

func runAnalyzeRunnerInteractive(opts runAnalyzeRunnerInteractiveOptions) error {
	execFn := opts.Exec
	if execFn == nil {
		execFn = execAnalyzeRunnerInteractive
	}
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	args := slices.Clone(opts.Runner.InteractiveArgs)
	args = append(args, opts.Prompt)
	if err := execFn(analyzeRunnerInteractiveExecution{
		Command: opts.Runner.Command,
		Args:    args,
		Stdin:   in,
		Stdout:  out,
		Stderr:  errOut,
	}); err != nil {
		return fmt.Errorf("interactive analysis runner %q failed: %w", opts.Runner.Name, err)
	}
	return nil
}

func runAnalyzeRunner(opts runAnalyzeRunnerOptions) error {
	execFn := opts.Exec
	if execFn == nil {
		execFn = execAnalyzeRunner
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	args := slices.Clone(opts.Runner.Args)
	var outputPath string
	if opts.Runner.OutputLastMessage {
		tmp, err := os.CreateTemp("", "taxiway-record-analysis-*.txt")
		if err != nil {
			return fmt.Errorf("analysis runner %q output file: %w", opts.Runner.Name, err)
		}
		outputPath = tmp.Name()
		if err := tmp.Close(); err != nil {
			_ = os.Remove(outputPath)
			return fmt.Errorf("analysis runner %q output file: %w", opts.Runner.Name, err)
		}
		defer os.Remove(outputPath)
		args = appendOutputLastMessageArg(args, outputPath)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stopProgress := opts.Progress.StartRunner(opts.Runner.Name)
	execution := analyzeRunnerExecution{
		Command:    opts.Runner.Command,
		Args:       args,
		Stdin:      opts.Prompt,
		Stdout:     &stdout,
		Stderr:     &stderr,
		OutputPath: outputPath,
	}
	if err := execFn(execution); err != nil {
		stopProgress()
		if stderr.Len() > 0 {
			return fmt.Errorf("analysis runner %q failed: %w: %s", opts.Runner.Name, err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("analysis runner %q failed: %w", opts.Runner.Name, err)
	}
	stopProgress()
	result := stdout.String()
	if outputPath != "" {
		data, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("analysis runner %q read final output: %w", opts.Runner.Name, err)
		}
		if len(data) > 0 {
			result = string(data)
		}
	}
	if strings.TrimSpace(result) == "" {
		return fmt.Errorf("analysis runner %q produced no analysis output", opts.Runner.Name)
	}
	if opts.Pretty {
		rendered, err := renderAnalysisMarkdown(result)
		if err != nil {
			return fmt.Errorf("render analysis markdown: %w", err)
		}
		result = rendered
	}
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	if _, err := io.WriteString(out, result); err != nil {
		return fmt.Errorf("write analysis output: %w", err)
	}
	return nil
}

func renderAnalysisMarkdownWithGlamour(markdown string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(markdown)
}

func appendOutputLastMessageArg(args []string, outputPath string) []string {
	if len(args) == 0 {
		return []string{"-o", outputPath}
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[:len(args)-1]...)
	out = append(out, "-o", outputPath)
	out = append(out, args[len(args)-1])
	return out
}

func execAnalyzeRunner(execution analyzeRunnerExecution) error {
	cmd := exec.Command(execution.Command, execution.Args...)
	cmd.Stdin = strings.NewReader(execution.Stdin)
	cmd.Stdout = execution.Stdout
	cmd.Stderr = execution.Stderr
	return cmd.Run()
}

func execAnalyzeRunnerInteractive(execution analyzeRunnerInteractiveExecution) error {
	cmd := exec.Command(execution.Command, execution.Args...)
	cmd.Stdin = execution.Stdin
	cmd.Stdout = execution.Stdout
	cmd.Stderr = execution.Stderr
	return cmd.Run()
}

func runRecordPlayer(cmd *cobra.Command, state *RootState, lab string, port int, openBrowser, writeOnly bool) error {
	if port < 0 {
		return fmt.Errorf("--port must be greater than 0")
	}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	playerPath, err := recording.EnsurePlayer(store)
	if err != nil {
		return fmt.Errorf("recording player: write index.html: %w", err)
	}
	if writeOnly {
		printRecordPlayerReady(cmd, playerPath, store.Dir(), "")
		return nil
	}

	port, err = recordPlayerPort(stateDir, lab, port, true)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("recording player: listen on %s: %w", addr, err)
	}
	defer ln.Close()

	url := fmt.Sprintf("http://localhost:%d", port)
	printRecordPlayerReady(cmd, playerPath, store.Dir(), url)

	if openBrowser {
		if err := openBrowserURL(url); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not open browser: %v\n", err)
		}
	}

	server := &http.Server{Handler: http.FileServer(http.Dir(store.Dir()))}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("recording player: serve: %w", err)
	}
	return nil
}

func recordPlayerPort(stateDir, lab string, explicitPort int, checkHostPort bool) (int, error) {
	if explicitPort < 0 || explicitPort > 65535 {
		return 0, fmt.Errorf("--port must be between 1 and 65535")
	}
	if explicitPort > 0 {
		return explicitPort, nil
	}

	portPath := filepath.Join(stateDir, lab, recordPlayerPortFile)
	if port, ok := readRecordPlayerPort(portPath); ok && (!checkHostPort || hostPortAvailable(port)) {
		return port, nil
	}

	used := allocatedRecordPlayerPorts(stateDir)
	for attempt := 0; attempt < recordPlayerHostPortSpan; attempt++ {
		port := recordPlayerPortCandidate(lab, attempt)
		if used[port] || (checkHostPort && !hostPortAvailable(port)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(portPath), 0o755); err != nil {
			return 0, fmt.Errorf("recording player: mkdir port state: %w", err)
		}
		if err := os.WriteFile(portPath, []byte(strconv.Itoa(port)+"\n"), 0o644); err != nil {
			return 0, fmt.Errorf("recording player: write port state: %w", err)
		}
		return port, nil
	}
	return 0, fmt.Errorf("recording player: no available local port found")
}

func readRecordPlayerPort(path string) (int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func allocatedRecordPlayerPorts(stateDir string) map[int]bool {
	used := map[int]bool{}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return used
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		port, ok := readRecordPlayerPort(filepath.Join(stateDir, entry.Name(), recordPlayerPortFile))
		if ok {
			used[port] = true
		}
	}
	return used
}

func recordPlayerPortCandidate(lab string, attempt int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(lab))
	return recordPlayerHostPortBase + int((h.Sum32()+uint32(attempt))%recordPlayerHostPortSpan)
}

func printRecordPlayerReady(cmd *cobra.Command, playerPath, recordingsDir, url string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Recording player ready")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Player:     %s\n", playerPath)
	fmt.Fprintf(out, "  Recordings: %s\n", recordingsDir)
	if url != "" {
		fmt.Fprintf(out, "  URL:        %s\n", url)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "The browser should open automatically. Use --no-open to only print the URL.")
	}
}

func openBrowserURL(url string) error {
	opener, args := browserOpenCommand(url)
	if _, err := exec.LookPath(opener); err != nil {
		return fmt.Errorf("%s not found; open manually: %s", opener, url)
	}
	return exec.Command(opener, args...).Run()
}

func startRecordingProcess(ctx context.Context, d driver.Driver, id string, target shellTarget, recorderSession, castPath string) error {
	if err := checkRecordingDependencies(ctx, d, id); err != nil {
		return err
	}
	if err := checkRecordingMount(ctx, d, id); err != nil {
		return err
	}
	attachCommand := target.AttachCommand
	if target.RequiresTmux && target.SessionName != "" {
		attachCommand = "tmux attach-session -f read-only,ignore-size -t " + target.SessionName
	}
	resizeTargetCmd := ""
	if target.RequiresTmux && target.SessionName != "" {
		resizeTargetCmd = fmt.Sprintf(
			"tmux resize-window -t %s -x %d -y %d 2>/dev/null || true; ",
			target.SessionName,
			recordingDefaultCols,
			recordingDefaultRows,
		)
	}
	recordCmd := resizeTargetCmd + fmt.Sprintf("tmux new-session -d -x %d -y %d", recordingDefaultCols, recordingDefaultRows) +
		" -s " + shellQuote(recorderSession) +
		" -c " + shellQuotePath(target.Workdir) +
		" " + shellQuote("asciinema rec --overwrite -c "+shellQuote(attachCommand)+" "+shellQuotePath(castPath))
	res, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", recordCmd},
	})
	if err != nil {
		return fmt.Errorf("record start failed: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("record start failed: exit code %d", res.ExitCode)
	}
	return nil
}

func checkRecordingMount(ctx context.Context, d driver.Driver, id string) error {
	lab, err := config.LabNameFromID(id)
	if err != nil {
		return err
	}
	checkCmd := "test -d " + shellQuotePath(labRecordingDir) + " && test -w " + shellQuotePath(labRecordingDir)
	res, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", checkCmd},
	})
	if err != nil {
		return fmt.Errorf("cannot check recording mount: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("recording mount %s is not available in lab %q; recreate or update the lab so it mounts the recordings state directory", labRecordingDir, lab)
	}
	return nil
}

func stopRecordingProcess(ctx context.Context, d driver.Driver, id, recorderSession string) error {
	stopCmd := "tmux send-keys -t " + shellQuote(recorderSession) + " C-b d"
	res, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", stopCmd},
	})
	if err != nil {
		return fmt.Errorf("record stop failed: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("record stop failed: exit code %d", res.ExitCode)
	}
	return nil
}

func checkRecordingDependencies(ctx context.Context, d driver.Driver, id string) error {
	res, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", "command -v asciinema >/dev/null 2>&1 && command -v tmux >/dev/null 2>&1"},
	})
	if err != nil {
		return fmt.Errorf("cannot check recording dependencies: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("recording requires asciinema and tmux in the lab runtime")
	}
	return nil
}
