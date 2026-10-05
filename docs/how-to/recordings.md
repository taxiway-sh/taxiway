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

Omit `--name` to stop the latest active recording. Taxiway detaches only the
recorder’s client, independently of tmux key bindings, and waits for the recorder
to end before marking the entry stopped. A timeout leaves it active for retry.

Or stop a recording by name:

```bash
taxiway record stop mylab --name delivery-run
```

If the recorder session has disappeared or the lab is stopped, `record stop`
reconciles the index and preserves the cast file. `record rm --force` can also
recover these entries, but removes the cast. Inspection or transport failures
leave the entry active so they can be retried safely.

Recording entries are saved before the recorder starts. If a start fails, use
`record stop` to recover the entry before starting another recording.

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

The output shows the recording name, state, start time, stop time, cast file,
and ID. Global listing warns by lab name when an index cannot be read and
continues with the healthy labs.

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
directory. Its pinned asciinema-player bundle is served locally with its license;
playback needs no CDN or internet connection.

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

Taxiway reads casts within the lab's recordings directory before handing them to
an agent. Local runners receive private snapshots which are removed when the
analysis ends; `--prompt-only` includes the captured contents in its output.
Both modes reject recordings outside that directory and symbolic links. Missing
casts must be recovered or removed before analysis.

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
| `--prompt-only` | Print the generated prompt with captured recording contents |
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

Each start gets a unique ID, including repeated starts with the same name in
one second. Names must be unique for removal. If a name appears more than once, select
the ID shown by `record list`:

```bash
taxiway record rm mylab --id '<ID-from-record-list>'
```

Taxiway saves the index before deleting the cast. A failed index save preserves
the cast; a failed cast deletion prints a warning with the remaining path.

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
