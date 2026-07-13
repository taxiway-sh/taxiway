# Recordings

Taxiway can record a lab session so you can replay or analyze what happened
during an orchestrator run.

Recordings use the same session target as `taxiway shell <lab>`. Cast files and
their index are stored on the host under the lab state directory and mounted
inside the lab at `/lab/recordings`.

## Prerequisites

Before starting a recording:

- the lab must exist;
- the lab session target must be running;
- `asciinema` and `tmux` must be available inside the lab runtime.

If recording dependencies are missing, recreate or update the lab with the
current adapter so its bootstrap phase installs them.

## Start A Recording

Start a recording for a lab:

```bash
taxiway record start mylab
```

Give the recording a stable name when you want to refer to it later:

```bash
taxiway record start mylab --name delivery-run
```

Only one recording can be active for a lab at a time. Stop the active recording
before starting another one.

Taxiway starts recordings with a stable terminal size and aligns the target tmux
session to that size before attaching in read-only mode, so your terminal size
does not disrupt the recording. When you
later open the lab shell, Taxiway resizes the live shell and active recordings
to match your terminal.

## Stop A Recording

Stop the latest active recording:

```bash
taxiway record stop mylab
```

Omit `--name` to stop the latest active recording.

Or stop a recording by name:

```bash
taxiway record stop mylab --name delivery-run
```

Stopped recordings can be listed, replayed in the browser player, analyzed, or
removed.

## List Recordings

List recordings for one lab:

```bash
taxiway record list mylab
```

List recordings across all labs:

```bash
taxiway record list
```

The output shows the recording name, state, start time, stop time, and cast file.

## Replay Recordings

Serve the browser player for a lab:

```bash
taxiway record player mylab
```

By default Taxiway writes `index.html` into the lab recordings directory, starts
a local HTTP server, and opens the player in the browser. The default player
port is allocated per lab and stored in lab state, so multiple lab players can
run side by side.

Useful options:

| Option | Description |
|---|---|
| `--port <port>` | Use a specific local port |
| `--no-open` | Print the URL without opening the browser |
| `--write-only` | Write `index.html` without starting the HTTP server |

The player reads `recordings.json` and local `.cast` files from the recordings
directory.

## Analyze Recordings

Analyze stopped recordings with a local agent runner:

```bash
taxiway record analyze mylab
```

Analyze one named recording:

```bash
taxiway record analyze mylab --record delivery-run
```

Taxiway generates an analysis prompt and runs it with the configured local
runner.

Use interactive mode when you want to keep working with the agent after the
initial analysis:

```bash
taxiway record analyze mylab --record delivery-run --interactive
```

In this mode, Taxiway opens the selected runner in its native interactive UI
with the recording analysis prompt already loaded. After the first answer, you
can ask follow-up questions, request more detail about a suspicious step,
compare the terminal timeline with the expected workflow, or ask the agent to
focus on setup, tooling, or orchestration issues visible in the recording.

Interactive mode is useful when the first analysis points to several possible
causes and you want to investigate them without manually rebuilding the prompt
or copying cast file references.

Supported runners:

| Runner | CLI |
|---|---|
| `codex` | local Codex CLI |
| `claude-code` | local Claude Code CLI |

Useful options:

| Option | Description |
|---|---|
| `--runner <name>` | Select `codex` or `claude-code` for this run |
| `--prompt-only` | Print the generated prompt and recording references |
| `--interactive` | Open the selected runner in its native interactive UI |
| `--detail summary\|full` | Control analysis detail |
| `--language <code>` | Request an output language, such as `en` or `fr` |
| `--pretty` | Render the final Markdown analysis for the terminal |

Use `TAXIWAY_ANALYZE_RUNNER` to set the default runner when `--runner` is not
provided.

## Remove A Recording

Remove a stopped recording and its cast file:

```bash
taxiway record rm mylab delivery-run
```

Active recordings are protected by default. Use `--force` only when you want
Taxiway to stop the active recorder process and remove the recording in one
operation:

```bash
taxiway record rm mylab delivery-run --force
```

## Troubleshooting

If recording fails because `/lab/recordings` is missing, recreate or update the
lab so the driver mounts the recordings state directory.

If analysis reports that no stopped recordings exist, stop the active recording
first or select a stopped recording with `--record`.
