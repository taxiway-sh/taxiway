package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/recording"
)

func writeRecordAnalysisSessions(t *testing.T, state *RootState, lab string, sessions ...recording.Session) {
	t.Helper()
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, lab)
	require.NoError(t, store.Save(recording.Index{Sessions: sessions}))
}

func analysisSession(lab, name, state string, started time.Time) recording.Session {
	id := started.UTC().Format("20060102-150405") + "-" + name
	session := recording.Session{
		ID:              id,
		Name:            name,
		Lab:             lab,
		Driver:          "mock",
		DriverID:        config.IDOf(lab),
		State:           state,
		ShellCommand:    "tmux attach-session -t sampleorch",
		RecorderSession: "taxiway-record-" + id,
		CastPath:        "/lab/recordings/" + id + ".cast",
		CastPathHost:    filepath.Join("/tmp/taxiway-state", lab, "recordings", id+".cast"),
		StartedAt:       started,
	}
	if state == recording.StateStopped {
		stopped := started.Add(10 * time.Minute)
		session.StoppedAt = &stopped
	}
	return session
}

func TestRecordAnalyzeHelpDocumentsFlagsAndRunners(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Analyze lab recordings")
	require.Contains(t, out, "Analyze lab recordings with a local agent runner")
	require.Contains(t, out, "A runner is a local CLI, such as Codex or Claude Code")
	require.Contains(t, out, "\n\nPrompt-only mode:\n")
	require.Contains(t, out, "\n\nRunner selection:\n")
	require.Contains(t, out, "\n\nProgress:\n")
	require.Contains(t, out, "final analysis remains on stdout")
	require.Contains(t, out, "\n\nPretty output:\n")
	require.Contains(t, out, "\n\nSupported runners:\n")
	require.Contains(t, out, "--record")
	require.Contains(t, out, "--prompt-only")
	require.Contains(t, out, "--interactive")
	require.Contains(t, out, "--runner")
	require.Contains(t, out, "--detail")
	require.Contains(t, out, "--language")
	require.Contains(t, out, "--no-progress")
	require.Contains(t, out, "--pretty")
	require.Contains(t, out, "summary")
	require.Contains(t, out, "full")
	require.Contains(t, out, `"en" (English)`)
	require.Contains(t, out, `"kok" (Konkani)`)
	require.Contains(t, out, "TAXIWAY_ANALYZE_RUNNER")
	require.Contains(t, out, "codex")
	require.Contains(t, out, "claude-code")
	require.Empty(t, errOut)
}

func TestRecordAnalyseAliasShowsAnalyzeHelp(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyse", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Analyze lab recordings with a local agent runner")
	require.Contains(t, out, "Usage:\n  taxiway record analyze <lab> [flags]")
	require.Empty(t, errOut)
}

func TestRecordAnalyzePromptOnlyUsesStoppedRecordingsOnly(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
		analysisSession("demo", "active", recording.StateRecording, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)),
	)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only")
	require.NoError(t, err)
	require.Contains(t, out, "taxiway record analysis prompt")
	require.Contains(t, out, "Lab: demo")
	require.Contains(t, out, "walkthrough")
	require.Contains(t, out, "20260520-100000-walkthrough.cast")
	require.Contains(t, out, "Produce an executive summary")
	require.Contains(t, out, "Keep the response concise")
	require.Contains(t, out, "Do not prefix every evidence bullet with `Observed:`")
	require.Contains(t, out, "Use `Inference:` only for bullets that are not directly supported by visible terminal evidence")
	require.Contains(t, out, "Do not mention internal recording parsing mechanics")
	require.Contains(t, out, "Do not treat successful .cast parsing as user-facing verification evidence")
	require.Contains(t, out, "Required output format")
	require.Contains(t, out, "## Outcome")
	require.Contains(t, out, "## Key Evidence")
	require.Contains(t, out, "## Failures and Recovery")
	require.Contains(t, out, "## Verification")
	require.Contains(t, out, "## Follow-ups")
	require.Contains(t, out, "Do not produce a full event-by-event timeline")
	require.NotContains(t, out, "A timeline of important visible events")
	require.Contains(t, out, "How to analyze these recordings")
	require.Contains(t, out, "Treat each .cast file as an asciinema v2 JSONL artifact")
	require.Contains(t, out, "Analyze only output events where event_type == \"o\"")
	require.Contains(t, out, "Strip ANSI/control sequences before extracting visible terminal text")
	require.Contains(t, out, "Signal extraction priorities")
	require.Contains(t, out, "User-entered shell commands")
	require.Contains(t, out, "Ignore repeated tmux redraws unless the visible state changed")
	require.Contains(t, out, "Runner note")
	require.Contains(t, out, "--prompt-only is the intended mode when another agent will perform the analysis")
	require.NotContains(t, out, "active")
	require.Empty(t, errOut)
}

func TestRecordAnalyzeWrapsRunnerOutputWithReadableHeader(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
		analysisSession("demo", "verification", recording.StateStopped, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex")
	require.NoError(t, err)
	require.Equal(t, "analysis from codex\n", out)
	require.Empty(t, errOut)
	require.Len(t, *calls, 1)
}

func TestRecordAnalyzeProgressGoesToStderrWhenEnabled(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))
	analyzeProgressEnabled = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { analyzeProgressEnabled = defaultAnalyzeProgressEnabled })

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex")
	require.NoError(t, err)
	require.Equal(t, "analysis from codex\n", out)
	require.Contains(t, errOut, "Preparing record analysis for lab \"demo\"")
	require.Contains(t, errOut, "Selected 1 stopped recording(s)")
	require.Contains(t, errOut, "Detail: summary")
	require.Contains(t, errOut, "Selected runner: codex.\n\n")
	require.Contains(t, errOut, "Running analysis with codex")
	require.Contains(t, errOut, "Analysis finished in")
	require.Contains(t, errOut, "\n\n")
	require.Len(t, *calls, 1)
}

func TestRecordAnalyzeNoProgressDisablesProgress(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))
	analyzeProgressEnabled = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { analyzeProgressEnabled = defaultAnalyzeProgressEnabled })

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex", "--no-progress")
	require.NoError(t, err)
	require.Equal(t, "analysis from codex\n", out)
	require.Empty(t, errOut)
	require.Len(t, *calls, 1)
}

func TestRecordAnalyzeRejectsPrettyPromptOnly(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only", "--pretty")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--pretty cannot be used with --prompt-only")
}

func TestRecordAnalyzeRejectsInteractivePromptOnly(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only", "--interactive")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--interactive cannot be used with --prompt-only")
}

func TestRecordAnalyzeInteractiveRequiresTerminal(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex", "--interactive")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--interactive requires interactive stdin and stdout")
}

func TestRecordAnalyzeInteractiveLaunchesRunnerWithInitialPrompt(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))
	analyzeRunnerInteractiveInput = func(*cobra.Command) bool { return true }
	analyzeRunnerInteractiveOutput = func(*cobra.Command) bool { return true }
	var got analyzeRunnerInteractiveExecution
	originalExec := defaultAnalyzeRunnerInteractiveExec
	defaultAnalyzeRunnerInteractiveExec = func(exec analyzeRunnerInteractiveExecution) error {
		got = exec
		return nil
	}
	t.Cleanup(func() {
		analyzeRunnerInteractiveInput = defaultAnalyzeRunnerInteractiveInput
		analyzeRunnerInteractiveOutput = defaultAnalyzeRunnerInteractiveOutput
		defaultAnalyzeRunnerInteractiveExec = originalExec
	})

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex", "--interactive")
	require.NoError(t, err)
	require.Empty(t, out)
	require.Empty(t, errOut)
	require.Equal(t, "codex", got.Command)
	require.Len(t, got.Args, 1)
	require.Contains(t, got.Args[0], "taxiway record analysis prompt")
	require.Contains(t, got.Args[0], "walkthrough")
}

func TestRecordAnalyzeProgressRefreshesHeartbeatWithSpacing(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))
	analyzeProgressEnabled = func(*cobra.Command) bool { return true }
	originalInterval := analyzeProgressInterval
	analyzeProgressInterval = time.Millisecond
	originalExec := defaultAnalyzeRunnerExec
	defaultAnalyzeRunnerExec = func(exec analyzeRunnerExecution) error {
		time.Sleep(5 * time.Millisecond)
		_, err := exec.Stdout.Write([]byte("analysis\n"))
		return err
	}
	t.Cleanup(func() {
		analyzeProgressEnabled = defaultAnalyzeProgressEnabled
		analyzeProgressInterval = originalInterval
		defaultAnalyzeRunnerExec = originalExec
	})

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex")
	require.NoError(t, err)
	require.Equal(t, "analysis\n", out)
	require.Contains(t, errOut, "Selected runner: codex.\n\n")
	require.Contains(t, errOut, "Running analysis with codex")
	require.Contains(t, errOut, "Still analyzing with codex... elapsed")
	require.Contains(t, errOut, "\x1b[2K\r")
	require.Equal(t, 0, strings.Count(errOut, "\nStill analyzing with codex"), "heartbeat should refresh in place instead of adding lines")
	require.Contains(t, errOut, "Analysis finished in")
	require.Contains(t, errOut, ".\n\n")
}

func TestRecordAnalyzePromptOnlyFullDetailRequestsDetailedReport(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only", "--detail", "full")
	require.NoError(t, err)
	require.Contains(t, out, "Produce a detailed analysis report")
	require.Contains(t, out, "A timeline of important visible events")
	require.Contains(t, out, "Follow-up recommendations")
}

func TestRecordAnalyzePromptOnlyUsesExplicitLanguage(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only", "--language", "FR")
	require.NoError(t, err)
	require.Contains(t, out, "Output language:")
	require.Contains(t, out, "language code: fr")
}

func TestRecordAnalyzePromptOnlyUsesLocaleLanguage(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "fr_FR.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only")
	require.NoError(t, err)
	require.Contains(t, out, "language code: fr")
}

func TestRecordAnalyzePromptOnlyOmitsLanguageForPosixLocale(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_MESSAGES", "fr_FR.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only")
	require.NoError(t, err)
	require.NotContains(t, out, "Output language:")
}

func TestRecordAnalyzeRejectsInvalidLanguage(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only", "--language", "fr-FR")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported analysis language")
	require.Contains(t, err.Error(), "[a-zA-Z]{2,8}")
}

func TestBuildRecordAnalysisPromptUsesEmbeddedTemplate(t *testing.T) {
	prompt, err := buildRecordAnalysisPrompt(recordAnalysisPromptOptions{
		Lab:           "demo",
		RecordingsDir: "/tmp/recordings",
		Sessions: []recording.Session{
			analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
		},
		DetailLevel: analysisDetailSummary,
	})
	require.NoError(t, err)

	require.Contains(t, prompt, "taxiway record analysis prompt")
	require.NotContains(t, prompt, "{{")
	require.Contains(t, prompt, "Lab: demo")
	require.Contains(t, prompt, "Recordings directory: /tmp/recordings")
	require.Contains(t, prompt, "- Name: walkthrough")
	require.Contains(t, prompt, "How to analyze these recordings")
	require.Contains(t, prompt, "Produce an executive summary")
	require.NotContains(t, prompt, "Produce a detailed analysis report")
}

func TestResolveAnalysisLanguageUsesExplicitValueBeforeLocale(t *testing.T) {
	language, err := resolveAnalysisLanguage("ENG", func(string) string {
		return "fr_FR.UTF-8"
	})
	require.NoError(t, err)
	require.Equal(t, "eng", language)
}

func TestResolveAnalysisLanguageRejectsMalformedExplicitValue(t *testing.T) {
	_, err := resolveAnalysisLanguage("fr-FR", os.Getenv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported analysis language")
}

func TestResolveAnalysisLanguageExtractsLocaleLanguage(t *testing.T) {
	env := map[string]string{
		"LC_MESSAGES": "ja_JP.UTF-8",
		"LANG":        "fr_FR.UTF-8",
	}
	language, err := resolveAnalysisLanguage("", func(name string) string {
		return env[name]
	})
	require.NoError(t, err)
	require.Equal(t, "ja", language)
}

func TestResolveAnalysisLanguageIgnoresPosixLocale(t *testing.T) {
	language, err := resolveAnalysisLanguage("", func(name string) string {
		if name == "LC_ALL" {
			return "posix.UTF-8"
		}
		return "fr_FR.UTF-8"
	})
	require.NoError(t, err)
	require.Empty(t, language)
}

func TestResolveAnalysisLanguageIgnoresMalformedLocale(t *testing.T) {
	language, err := resolveAnalysisLanguage("", func(name string) string {
		switch name {
		case "LC_ALL":
			return ""
		case "LC_MESSAGES":
			return "1.UTF-8"
		case "LANG":
			return "fr_FR.UTF-8"
		default:
			return ""
		}
	})
	require.NoError(t, err)
	require.Equal(t, "fr", language)
}

func TestRecordAnalyzeRejectsUnknownDetailLevel(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--detail", "verbose")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported analysis detail level")
	require.Contains(t, err.Error(), "summary")
	require.Contains(t, err.Error(), "full")
}

func TestRecordAnalyzePromptOnlyRecordSelectsOneStoppedRecording(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "one", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
		analysisSession("demo", "two", recording.StateStopped, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)),
	)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--record", "two", "--prompt-only")
	require.NoError(t, err)
	require.Contains(t, out, "two")
	require.NotContains(t, out, "Name: one")
}

func TestRecordAnalyzeErrorsWhenRecordingIsUnknown(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--record", "missing", "--prompt-only")
	require.Error(t, err)
	require.Contains(t, err.Error(), "recording \"missing\" not found for lab \"demo\"")
}

func TestRecordAnalyzeErrorsWhenNoStoppedRecordingsAreSelected(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "active", recording.StateRecording, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)),
	)

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no stopped recordings found for lab \"demo\"")
}

func TestRecordAnalyzeRunsExplicitRunner(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex"))

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex")
	require.NoError(t, err)
	require.Equal(t, "analysis from codex\n", out)
	require.Empty(t, errOut)
	require.Len(t, *calls, 1)
	require.Equal(t, "codex", (*calls)[0].Command)
	require.Contains(t, (*calls)[0].Stdin, "walkthrough")
}

func TestRecordAnalyzeUsesEnvironmentRunnerWhenFlagAbsent(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "claude"))
	t.Setenv("TAXIWAY_ANALYZE_RUNNER", "claude-code")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo")
	require.NoError(t, err)
	require.Equal(t, "analysis from claude\n", out)
	require.Len(t, *calls, 1)
	require.Equal(t, "claude", (*calls)[0].Command)
}

func TestRecordAnalyzeRunnerFlagOverridesEnvironment(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex", "claude"))
	t.Setenv("TAXIWAY_ANALYZE_RUNNER", "claude-code")

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--runner", "codex")
	require.NoError(t, err)
	require.Len(t, *calls, 1)
	require.Equal(t, "codex", (*calls)[0].Command)
}

func TestRecordAnalyzePromptOnlyDoesNotRequireRunner(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("TAXIWAY_ANALYZE_RUNNER", "unknown")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo", "--prompt-only")
	require.NoError(t, err)
	require.Contains(t, out, "taxiway record analysis prompt")
	require.Empty(t, *calls)
}

func TestRecordAnalyzeErrorsWhenNoRunnerCanBeSelected(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	t.Setenv("PATH", t.TempDir())

	_, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no supported analysis runner detected")
	require.Contains(t, err.Error(), "--runner")
	require.Contains(t, err.Error(), "TAXIWAY_ANALYZE_RUNNER")
}

func TestRecordAnalyzePromptsForRunnerWhenInteractiveAndMultipleDetected(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	writeRecordAnalysisSessions(t, state, "demo",
		analysisSession("demo", "walkthrough", recording.StateStopped, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	)
	calls := captureAnalyzeRunnerExecution(t)
	t.Setenv("PATH", testPathWithCommands(t, "codex", "claude"))
	root.SetIn(bytes.NewBufferString("2\n"))
	analyzeRunnerInteractiveInput = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { analyzeRunnerInteractiveInput = defaultAnalyzeRunnerInteractiveInput })

	out, _, err := execRoot(t, root, stdout, stderr, "record", "analyze", "demo")
	require.NoError(t, err)
	require.Contains(t, out, "Choose analysis runner")
	require.Contains(t, out, "analysis from claude")
	require.Len(t, *calls, 1)
	require.Equal(t, "claude", (*calls)[0].Command)
}

func TestSelectAnalyzeRunnerUsesFlagBeforeEnvironment(t *testing.T) {
	runner, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Explicit: "codex",
		Env:      "claude-code",
		Lookup:   fakeAnalyzeRunnerLookup("codex", "claude"),
	})
	require.NoError(t, err)
	require.Equal(t, "codex", runner.Name)
	require.Equal(t, "codex", runner.Command)
	require.Equal(t, []string{"exec", "-"}, runner.Args)
}

func TestSelectAnalyzeRunnerRejectsUnknownRunner(t *testing.T) {
	_, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Explicit: "other",
		Lookup:   fakeAnalyzeRunnerLookup("codex", "claude"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported analysis runner \"other\"")
	require.Contains(t, err.Error(), "codex")
	require.Contains(t, err.Error(), "claude-code")
}

func TestSelectAnalyzeRunnerRejectsUnavailableExplicitRunner(t *testing.T) {
	_, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Explicit: "claude-code",
		Lookup:   fakeAnalyzeRunnerLookup("codex"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analysis runner \"claude-code\" requires command \"claude\"")
}

func TestSelectAnalyzeRunnerAutoDetectsSingleRunner(t *testing.T) {
	runner, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Lookup: fakeAnalyzeRunnerLookup("codex"),
	})
	require.NoError(t, err)
	require.Equal(t, "codex", runner.Name)
}

func TestSelectAnalyzeRunnerRejectsMultipleDetectedRunnersWhenNonInteractive(t *testing.T) {
	_, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Lookup: fakeAnalyzeRunnerLookup("codex", "claude"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiple analysis runners detected")
	require.Contains(t, err.Error(), "--runner")
	require.Contains(t, err.Error(), "TAXIWAY_ANALYZE_RUNNER")
}

func TestSelectAnalyzeRunnerPromptsWhenInteractive(t *testing.T) {
	var prompt bytes.Buffer
	runner, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Lookup:      fakeAnalyzeRunnerLookup("codex", "claude"),
		Interactive: true,
		In:          bytes.NewBufferString("2\n"),
		Out:         &prompt,
	})
	require.NoError(t, err)
	require.Equal(t, "claude-code", runner.Name)
	require.Contains(t, prompt.String(), "Choose analysis runner")
}

func TestSelectAnalyzeRunnerClearsPromptWhenRequested(t *testing.T) {
	var prompt bytes.Buffer
	runner, err := selectAnalyzeRunner(analyzeRunnerSelectionOptions{
		Lookup:      fakeAnalyzeRunnerLookup("codex", "claude"),
		Interactive: true,
		ClearPrompt: true,
		In:          bytes.NewBufferString("1\n"),
		Out:         &prompt,
	})
	require.NoError(t, err)
	require.Equal(t, "codex", runner.Name)
	require.Contains(t, prompt.String(), "Choose analysis runner")
	require.Contains(t, prompt.String(), "\x1b[4A\x1b[J")
}

func fakeAnalyzeRunnerLookup(available ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, name := range available {
		set[name] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", fmt.Errorf("%w: %s", os.ErrNotExist, name)
	}
}

func captureAnalyzeRunnerExecution(t *testing.T) *[]analyzeRunnerExecution {
	t.Helper()
	var calls []analyzeRunnerExecution
	original := defaultAnalyzeRunnerExec
	defaultAnalyzeRunnerExec = func(exec analyzeRunnerExecution) error {
		calls = append(calls, exec)
		_, err := exec.Stdout.Write([]byte("analysis from " + exec.Command + "\n"))
		return err
	}
	t.Cleanup(func() {
		defaultAnalyzeRunnerExec = original
	})
	return &calls
}

func testPathWithCommands(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	return dir
}

func TestRunAnalyzeRunnerSendsPromptOnStdin(t *testing.T) {
	var got analyzeRunnerExecution
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
		Prompt: "analyze this",
		Out:    &bytes.Buffer{},
		Exec: func(exec analyzeRunnerExecution) error {
			got = exec
			_, err := exec.Stdout.Write([]byte("analysis\n"))
			return err
		},
	})
	require.NoError(t, err)
	require.Equal(t, "codex", got.Command)
	require.Len(t, got.Args, 4)
	require.Equal(t, "exec", got.Args[0])
	require.Equal(t, "-o", got.Args[1])
	require.NotEmpty(t, got.Args[2])
	require.Equal(t, "-", got.Args[3])
	require.Equal(t, "analyze this", got.Stdin)
}

func TestRunAnalyzeRunnerPrintsOnlyFinalOutputFile(t *testing.T) {
	var out bytes.Buffer
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
		Prompt: "analyze this",
		Out:    &out,
		Exec: func(exec analyzeRunnerExecution) error {
			_, _ = exec.Stdout.Write([]byte("runner log on stdout\n"))
			_, _ = exec.Stderr.Write([]byte("runner log on stderr\n"))
			return os.WriteFile(exec.OutputPath, []byte("final analysis\n"), 0o644)
		},
	})
	require.NoError(t, err)
	require.Equal(t, "final analysis\n", out.String())
}

func TestRunAnalyzeRunnerAppendsMissingFinalNewline(t *testing.T) {
	var out bytes.Buffer
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
		Prompt: "analyze this",
		Out:    &out,
		Exec: func(exec analyzeRunnerExecution) error {
			return os.WriteFile(exec.OutputPath, []byte("final analysis without newline"), 0o644)
		},
	})
	require.NoError(t, err)
	require.Equal(t, "final analysis without newline\n", out.String())
}

func TestRunAnalyzeRunnerErrorsWhenFinalOutputIsEmpty(t *testing.T) {
	var out bytes.Buffer
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
		Prompt: "analyze this",
		Out:    &out,
		Exec: func(exec analyzeRunnerExecution) error {
			_, _ = exec.Stdout.Write([]byte("runner log on stdout\n"))
			return os.WriteFile(exec.OutputPath, []byte(" \n\t"), 0o644)
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analysis runner \"codex\" produced no analysis output")
	require.Empty(t, out.String())
}

func TestRunAnalyzeRunnerPrettyRendersFinalMarkdown(t *testing.T) {
	var out bytes.Buffer
	original := renderAnalysisMarkdown
	renderAnalysisMarkdown = func(markdown string) (string, error) {
		return "pretty:" + markdown, nil
	}
	t.Cleanup(func() { renderAnalysisMarkdown = original })

	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}, OutputLastMessage: true},
		Prompt: "analyze this",
		Out:    &out,
		Pretty: true,
		Exec: func(exec analyzeRunnerExecution) error {
			return os.WriteFile(exec.OutputPath, []byte("# Analysis\n"), 0o644)
		},
	})
	require.NoError(t, err)
	require.Equal(t, "pretty:# Analysis\n", out.String())
}

func TestRunAnalyzeRunnerInteractiveAppendsPromptArgument(t *testing.T) {
	var got analyzeRunnerInteractiveExecution
	err := runAnalyzeRunnerInteractive(runAnalyzeRunnerInteractiveOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", InteractiveArgs: []string{"--no-alt-screen"}},
		Prompt: "analyze this",
		Exec: func(exec analyzeRunnerInteractiveExecution) error {
			got = exec
			return nil
		},
		In:  strings.NewReader(""),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Equal(t, "codex", got.Command)
	require.Equal(t, []string{"--no-alt-screen", "analyze this"}, got.Args)
}

func TestRunAnalyzeRunnerInteractiveWrapsExecutionErrors(t *testing.T) {
	err := runAnalyzeRunnerInteractive(runAnalyzeRunnerInteractiveOptions{
		Runner: analyzeRunner{Name: "claude-code", Command: "claude"},
		Prompt: "analyze this",
		Exec: func(analyzeRunnerInteractiveExecution) error {
			return errors.New("boom")
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "interactive analysis runner \"claude-code\" failed")
}

func TestRunAnalyzeRunnerSuppressesStderrForStdoutRunners(t *testing.T) {
	var out bytes.Buffer
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "claude-code", Command: "claude", Args: []string{"-p"}},
		Prompt: "analyze this",
		Out:    &out,
		Exec: func(exec analyzeRunnerExecution) error {
			_, _ = exec.Stdout.Write([]byte("final analysis\n"))
			_, _ = exec.Stderr.Write([]byte("runner log on stderr\n"))
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, "final analysis\n", out.String())
}

func TestRunAnalyzeRunnerWrapsExecutionErrors(t *testing.T) {
	err := runAnalyzeRunner(runAnalyzeRunnerOptions{
		Runner: analyzeRunner{Name: "codex", Command: "codex", Args: []string{"exec", "-"}},
		Prompt: "analyze this",
		Out:    &bytes.Buffer{},
		Exec: func(analyzeRunnerExecution) error {
			return errors.New("boom")
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analysis runner \"codex\" failed")
}

func TestExecAnalyzeRunnerDoesNotInheritProcessStderr(t *testing.T) {
	var out bytes.Buffer
	err := execAnalyzeRunner(analyzeRunnerExecution{
		Command: "sh",
		Args:    []string{"-c", "printf final; printf log >&2"},
		Stdin:   "",
		Stdout:  &out,
		Stderr:  io.Discard,
	})
	require.NoError(t, err)
	require.Equal(t, "final", out.String())
}
