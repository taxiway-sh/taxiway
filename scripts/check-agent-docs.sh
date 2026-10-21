#!/usr/bin/env bash
# Check agent docs and docs/ pages: skill frontmatter and discovery symlinks,
# scheduled agent declarations and titles, autonomy, heartbeat schema
# (approval log and frictions) and machine queue schema,
# relative Markdown links (pages under docs/ reach other files through blob
# URLs), and normative phrases that must stay in their owner.
# Usage: scripts/check-agent-docs.sh [repository-root]
set -euo pipefail
root="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
exec python3 -I - "$root" <<'PY'
import html
import json
import pathlib
import re
import sys
import unicodedata

root = pathlib.Path(sys.argv[1]).resolve()
# Pages rendered by the site link to repository files through this URL.
BLOB = "https://github.com/taxiway-sh/taxiway/blob/main/"

# Normative phrases and the only file allowed to contain them. Each rule has
# one owner; other files link to it. Matching ignores case and line wrapping.
TESTING = ".agents/skills/taxiway-testing/SKILL.md"
OWNED_PHRASES = [
    ("`TestE2E_` entry point", TESTING),
    ("parallel suite", TESTING),
    ("three lifecycle E2E scenarios", TESTING),
    ("adds its own three scenarios", TESTING),
    ("Each rule has one owner", "AGENTS.md"),
]

errors = []


def docs():
    files = [root / "AGENTS.md", root / "CONTRIBUTING.md", root / "tests/live/AGENTS.md"]
    files += sorted((root / ".agents").rglob("*.md"))
    files += sorted((root / "docs").rglob("*.md"))
    return [f for f in files if f.is_file()]


def rel(path):
    return path.relative_to(root).as_posix()


def strip_code(text, inline=True):
    """Drop fenced blocks (3+ backticks or tildes) and, unless inline is
    False, blank inline code."""
    kept, fence = [], None
    for line in text.splitlines():
        match = re.match(r" {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence is None:
            if match:
                fence = match.group(1)
            else:
                kept.append(re.sub(r"`[^`\n]*`", "``", line) if inline else line)
        elif (match and match.group(1)[0] == fence[0]
              and len(match.group(1)) >= len(fence) and not match.group(2).strip()):
            fence = None
    return "\n".join(kept)


def slug(heading):
    """Heading anchor, an approximation of github-slugger, which GitHub and the
    site (rehype-slug) both use: the heading's rendered text, lowercased, keeps
    letters, digits, `_`, `-` and spaces, and each space becomes `-`. Inline
    code keeps its text verbatim; outside it, links and images keep their text,
    HTML tags, escapes, entities and emphasis markers are dropped. Not handled:
    setext headings and a duplicate suffix that collides with another heading.
    The site's docs link test checks docs/ pages with the real algorithm."""
    parts = re.split(r"(`+)(.+?)\1", heading.strip())
    text = ""
    for index in range(0, len(parts), 3):
        prose = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", parts[index])
        prose = html.unescape(re.sub(r"\\(\W)", r"\1", re.sub(r"<[^>]*>", "", prose)))
        text += re.sub(r"\*|(?<!\w)_|_(?!\w)", "", prose)
        if index + 2 < len(parts):
            text += parts[index + 2].strip() or parts[index + 2]
    return re.sub(r"[^\w\- ]", "", text.lower()).replace(" ", "-")


def anchors(path):
    text = path.read_text()
    result = set(re.findall(r"<a\s+(?:id|name)=[\"']([^\"']+)[\"']", text))
    seen = {}
    for line in strip_code(text, inline=False).splitlines():
        match = re.match(r"#{1,6}\s+(.*?)\s*#*\s*$", line)
        if not match:
            continue
        base = slug(match.group(1))
        count = seen.get(base, 0)
        seen[base] = count + 1
        result.add(base if count == 0 else f"{base}-{count}")
    return result


def frontmatter(lines):
    """Top-level fields of a simple YAML frontmatter; block scalars are joined."""
    end = [l.strip() for l in lines[1:]].index("---") + 1
    fields, key = {}, None
    for line in lines[1:end]:
        if line.startswith((" ", "\t")):
            if key:
                fields[key] = (fields[key] + " " + line.strip()).strip()
            continue
        name, sep, value = line.partition(":")
        key = name.strip() if sep else None
        if key:
            value = value.strip()
            block = re.fullmatch(r"[>|][+-]?\d*", value)
            fields[key] = "" if block else value.strip("'\"")
    return fields


def check_skills():
    skills = root / ".agents/skills"
    for skill in sorted(p for p in skills.iterdir() if p.is_dir()) if skills.is_dir() else []:
        name = skill.name
        manifest = skill / "SKILL.md"
        if not manifest.is_file():
            errors.append(f"{rel(skill)}: missing SKILL.md")
            continue
        lines = manifest.read_text().splitlines()
        if not lines or lines[0].strip() != "---" or "---" not in [l.strip() for l in lines[1:]]:
            errors.append(f"{rel(manifest)}: missing YAML frontmatter")
        else:
            fields = frontmatter(lines)
            if fields.get("name") != name:
                errors.append(f"{rel(manifest)}: frontmatter name must be {name!r}")
            if not fields.get("description"):
                errors.append(f"{rel(manifest)}: frontmatter description is empty")
        link = root / ".claude/skills" / name
        if not link.is_symlink() or link.resolve() != skill.resolve():
            errors.append(f".claude/skills/{name}: must be a symlink to {rel(skill)}")
    discovery = root / ".claude/skills"
    for entry in sorted(discovery.iterdir()) if discovery.is_dir() else []:
        if entry.name.startswith("."):
            continue
        if not (skills / entry.name).is_dir():
            errors.append(f".claude/skills/{entry.name}: no matching .agents/skills/{entry.name}")


AGENT_FIELDS = {"id", "title", "model", "repositories", "workdir", "skills", "outputs", "autonomy"}
# Optional fields: runs (where the agent runs, cloud by default) and schedule,
# required in the cloud and absent on the machine, where runs are event-driven.
AGENT_OPTIONAL = {"runs", "schedule"}
RUNS = ["cloud", "machine"]
LEVELS = ["propose", "ask", "auto"]
# The fixed never list: the agents skill must list exactly these actions, and
# no declaration may use one as an action class.
NEVER = {
    "push-main", "force-push-foreign", "publish-release", "publish-upstream",
    "handle-secrets", "skip-tests", "change-own-instructions",
    "change-approvals", "change-never-list",
}
# Classes pinned at highest level ask: they change the agentic system, or
# AGENTS.md reserves them to the maintainer.
NEVER_AUTO = {"change-agent-system", "merge-pr"}
# Classes that need write access to taxiway-sh/taxiway, and only they grant it.
PUSH_CLASSES = {"push-pr-branch", "change-agent-system"}
AGENTS_SKILL = ".agents/skills/taxiway-agents/SKILL.md"
HEARTBEAT_SCHEMA = "status.schema.json"
QUEUE_SCHEMA = "queue.schema.json"
QUEUE_REQUIRED = {"id", "link", "commit", "checks", "maxMinutes", "expires", "requester", "created", "status"}
QUEUE_STATUSES = ["pending", "running", "done", "failed", "expired", "rejected"]
QUEUE_DATES = ["expires", "created", "started", "finished"]
ONE_LINE = "^[^\\n]*$"
UTC_DATE = "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
# A machine agent reads Taxiway, writes only the state repository, and takes
# no action class: its queue and heartbeat writes are its run contract.
MACHINE_REPOSITORIES = {"taxiway-sh/taxiway": "read", "taxiway-sh/agents": "write"}
APPROVAL_FIELDS = {"class", "proposal", "answer", "date", "link"}
APPROVAL_ANSWERS = ["ok", "refused", "corrected"]
# A declaration's title names its session and scheduled task (en dash); the
# scope is printable ASCII without parentheses.
TITLE = re.compile(r"Taxiway \u2013 [A-Za-z][A-Za-z0-9-]*(?: [A-Za-z0-9-]+)* agent"
                   r" \([!-'*-~]+(?: [!-'*-~]+)*\)")
WATCHDOG_TITLE = "Taxiway \u2013 agents watchdog"
FRICTION_FIELDS = {"kind", "summary"}
FRICTION_KINDS = ["missing-rule", "ambiguous-rule", "contradictory-rule",
                  "workaround", "check-not-run", "tool-refused"]
FRICTIONS_MAX = 20
FRICTION_SUMMARY_MAX = 200
# Heartbeat links stay on the project's own GitHub items.
PROJECT_LINK = "^https://github\\.com/taxiway-sh/"


def skill_table(text, heading):
    """Rows of the table under a heading: {first-cell code: last-cell code}."""
    section = re.split(rf"^###? {re.escape(heading)}\s*$", text, maxsplit=1, flags=re.M)
    if len(section) < 2:
        return {}
    rows = {}
    for line in re.split(r"^#", section[1], maxsplit=1, flags=re.M)[0].splitlines():
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        first = re.fullmatch(r"`([a-z0-9-]+)`", cells[0]) if line.startswith("|") else None
        if first:
            if first.group(1) in rows:
                errors.append(f"{AGENTS_SKILL}: duplicate {heading} row {first.group(1)!r}")
            last = re.fullmatch(r"`([a-z]+)`", cells[-1])
            rows[first.group(1)] = last.group(1) if last else None
    return rows


def action_classes():
    """Action classes and their highest level, and the never list, from the skill."""
    skill = root / AGENTS_SKILL
    if not skill.is_file():
        errors.append(f"{AGENTS_SKILL}: missing; it defines the agent action classes")
        return {}
    text = skill.read_text()
    classes = skill_table(text, "Action classes")
    if not classes:
        errors.append(f"{AGENTS_SKILL}: no Action classes table")
    for name, highest in classes.items():
        if highest not in ("ask", "auto"):
            errors.append(f"{AGENTS_SKILL}: action class {name!r} highest level must be ask or auto")
            classes[name] = "ask"
    for name in sorted(NEVER_AUTO):
        if classes.get(name) != "ask":
            errors.append(f"{AGENTS_SKILL}: action class {name!r} must have highest level ask")
            classes[name] = "ask"
    never = set(skill_table(text, "Never"))
    if never != NEVER:
        errors.append(f"{AGENTS_SKILL}: Never list must match the checker:"
                      f" missing {sorted(NEVER - never)}, extra {sorted(never - NEVER)}")
    for name in classes.keys() & NEVER:
        errors.append(f"{AGENTS_SKILL}: {name!r} is on the never list, not an action class")
    return classes


def check_autonomy(name, autonomy, classes):
    if not isinstance(autonomy, dict):
        errors.append(f"{name}: autonomy must map action classes to {LEVELS}")
        return
    for action, level in autonomy.items():
        if action in NEVER:
            errors.append(f"{name}: autonomy {action!r} is on the never list")
        elif action not in classes:
            errors.append(f"{name}: autonomy {action!r} is not an action class")
        elif level not in LEVELS:
            errors.append(f"{name}: autonomy {action!r} must be one of {LEVELS}")
        elif LEVELS.index(level) > LEVELS.index(classes[action]):
            errors.append(f"{name}: autonomy {action!r} can never be {level}")


def check_heartbeat_list(schema, props, name, fields, enum_field, values):
    """An optional heartbeat list of objects with exactly the required fields."""
    node = props.get(name) if isinstance(props, dict) else None
    if not isinstance(node, dict) or node.get("type") != "array":
        errors.append(f"{rel(schema)}: {name} must be an array")
        return None
    items = node.get("items")
    if not isinstance(items, dict) or items.get("additionalProperties") is not False:
        errors.append(f"{rel(schema)}: {name} items must be objects without additional properties")
        return None
    if not isinstance(items.get("required"), list) or set(items["required"]) != fields:
        errors.append(f"{rel(schema)}: {name} items must require exactly {sorted(fields)}")
    item_props = items.get("properties")
    item_props = item_props if isinstance(item_props, dict) else {}
    enum = item_props.get(enum_field)
    if not isinstance(enum, dict) or enum.get("enum") != values:
        errors.append(f"{rel(schema)}: {name} {enum_field} must be one of {values}")
    return item_props


def check_heartbeat_schema(schema):
    data = json.loads(schema.read_text())
    if not isinstance(data, dict):
        errors.append(f"{rel(schema)}: must be a JSON object")
        return
    required = data.get("required", [])
    if not isinstance(required, list):
        errors.append(f"{rel(schema)}: required must be a list")
        required = []
    if "promptVersion" not in required:
        errors.append(f"{rel(schema)}: must require promptVersion")
    for name in ("approvals", "frictions"):
        if name in required:
            errors.append(f"{rel(schema)}: {name} must stay optional")
    props = data.get("properties")
    check_heartbeat_list(schema, props, "approvals", APPROVAL_FIELDS, "answer", APPROVAL_ANSWERS)
    frictions = check_heartbeat_list(schema, props, "frictions", FRICTION_FIELDS, "kind", FRICTION_KINDS)
    if frictions is not None:
        if set(frictions) != FRICTION_FIELDS | {"link"}:
            errors.append(f"{rel(schema)}: frictions items must have exactly"
                          f" {sorted(FRICTION_FIELDS | {'link'})}")
        link = frictions.get("link")
        if not isinstance(link, dict) or link.get("pattern") != PROJECT_LINK:
            errors.append(f"{rel(schema)}: frictions link must match {PROJECT_LINK}")
        summary = frictions.get("summary")
        if (not isinstance(summary, dict) or summary.get("type") != "string"
                or summary.get("minLength") != 1
                or summary.get("maxLength") != FRICTION_SUMMARY_MAX):
            errors.append(f"{rel(schema)}: frictions summary must be a non-empty string of at"
                          f" most {FRICTION_SUMMARY_MAX} characters")
        if props["frictions"].get("maxItems") != FRICTIONS_MAX:
            errors.append(f"{rel(schema)}: frictions must hold at most {FRICTIONS_MAX} items")


def check_queue_schema(schema):
    """Shape of the machine queue item schema; samples are validated by its tests."""
    data = json.loads(schema.read_text())
    if not isinstance(data, dict):
        errors.append(f"{rel(schema)}: must be a JSON object")
        return
    if data.get("additionalProperties") is not False:
        errors.append(f"{rel(schema)}: queue items must not allow additional properties")
    required = data.get("required")
    if not isinstance(required, list) or set(required) != QUEUE_REQUIRED:
        errors.append(f"{rel(schema)}: queue items must require exactly {sorted(QUEUE_REQUIRED)}")
    props = data.get("properties") if isinstance(data.get("properties"), dict) else {}
    status = props.get("status")
    if not isinstance(status, dict) or status.get("enum") != QUEUE_STATUSES:
        errors.append(f"{rel(schema)}: queue status must be one of {QUEUE_STATUSES}")
    commit = props.get("commit")
    if not isinstance(commit, dict) or commit.get("pattern") != "^[0-9a-f]{40}$":
        errors.append(f"{rel(schema)}: queue commit must be a full SHA")
    checks = props.get("checks") if isinstance(props.get("checks"), dict) else {}
    items = checks.get("items") if isinstance(checks.get("items"), dict) else {}
    fields = items.get("properties") if isinstance(items.get("properties"), dict) else {}
    scope = fields.get("scope")
    if not isinstance(scope, dict) or scope.get("pattern") != ONE_LINE:
        errors.append(f"{rel(schema)}: queue check scope must be one line ({ONE_LINE})")
    for field in QUEUE_DATES:
        date = props.get(field)
        if not isinstance(date, dict) or date.get("pattern") != UTC_DATE:
            errors.append(f"{rel(schema)}: queue {field} must be a UTC date ({UTC_DATE})")


def check_agents():
    """Scheduled agent declarations: .agents/agents/<id>.json + <id>.prompt.md."""
    agents = root / ".agents/agents"
    schedules, titles = {}, {}
    schema = agents / HEARTBEAT_SCHEMA
    classes = {}
    if agents.is_dir():
        try:
            check_heartbeat_schema(schema)
        except (OSError, ValueError) as err:
            errors.append(f"{rel(schema)}: unreadable heartbeat schema: {err}")
        queue = agents / QUEUE_SCHEMA
        try:
            check_queue_schema(queue)
        except (OSError, ValueError) as err:
            errors.append(f"{rel(queue)}: unreadable queue schema: {err}")
        classes = action_classes()
    decls = [p for p in agents.glob("*.json") if not p.name.endswith(".schema.json")] if agents.is_dir() else []
    for decl in sorted(decls):
        name = rel(decl)
        try:
            data = json.loads(decl.read_text())
        except ValueError as err:
            errors.append(f"{name}: invalid JSON: {err}")
            continue
        if not isinstance(data, dict):
            errors.append(f"{name}: must be a JSON object")
            continue
        missing = AGENT_FIELDS - data.keys()
        extra = data.keys() - AGENT_FIELDS - AGENT_OPTIONAL
        if missing:
            errors.append(f"{name}: missing fields {sorted(missing)}")
        if extra:
            errors.append(f"{name}: unknown fields {sorted(extra)}")
        if data.get("id") != decl.stem:
            errors.append(f"{name}: id must be {decl.stem!r}")
        if not (agents / f"{decl.stem}.prompt.md").is_file():
            errors.append(f"{name}: missing {decl.stem}.prompt.md")
        runs = data.get("runs", "cloud")
        if runs not in RUNS:
            errors.append(f"{name}: runs must be one of {RUNS}")
        schedule = data.get("schedule")
        if runs == "machine":
            if "schedule" in data:
                errors.append(f"{name}: a machine agent is event-driven and has no schedule")
            if data.get("repositories") != MACHINE_REPOSITORIES:
                errors.append(f"{name}: a machine agent's repositories must be exactly {MACHINE_REPOSITORIES}")
            if data.get("autonomy") != {}:
                errors.append(f"{name}: a machine agent takes no action class; autonomy must be empty")
        elif not isinstance(schedule, str) or len(schedule.split()) != 5:
            errors.append(f"{name}: schedule must be five cron fields")
        elif schedule in schedules:
            errors.append(f"{name}: schedule {schedule!r} already used by {schedules[schedule]}")
        else:
            schedules[schedule] = decl.stem
        repos = data.get("repositories")
        if not isinstance(repos, dict) or any(v not in ("read", "write") for v in repos.values()):
            errors.append(f"{name}: repositories must map each repository to read or write")
        elif repos.get("taxiway-sh/taxiway") not in ("read", "write"):
            errors.append(f"{name}: taxiway-sh/taxiway access must be declared")
        else:
            autonomy = data.get("autonomy") if isinstance(data.get("autonomy"), dict) else {}
            pushes = sorted(c for c in PUSH_CLASSES if autonomy.get(c) in ("ask", "auto"))
            if repos["taxiway-sh/taxiway"] == "write" and not pushes:
                errors.append(f"{name}: write on taxiway-sh/taxiway requires one of"
                              f" {sorted(PUSH_CLASSES)} at ask or auto")
            elif repos["taxiway-sh/taxiway"] == "read" and pushes:
                errors.append(f"{name}: {pushes} at ask or auto requires write on taxiway-sh/taxiway")
        for field in ("title", "workdir", "model"):
            if not isinstance(data.get(field), str) or not data.get(field):
                errors.append(f"{name}: {field} must be a non-empty string")
        title = data.get("title")
        if isinstance(title, str) and title:
            if any(unicodedata.category(c) == "Cf" for c in title):
                errors.append(f"{name}: title must not contain invisible format characters")
            elif decl.stem == "watchdog" and title != WATCHDOG_TITLE:
                errors.append(f"{name}: title must be {WATCHDOG_TITLE!r}")
            elif decl.stem != "watchdog" and not TITLE.fullmatch(title):
                errors.append(f"{name}: title must be 'Taxiway \u2013 <role> agent (<scope>)'")
            elif (key := unicodedata.normalize("NFKC", title).casefold()) in titles:
                errors.append(f"{name}: title {title!r} already used by {titles[key]}")
            else:
                titles[key] = decl.stem
        for field in ("skills", "outputs"):
            if not isinstance(data.get(field), list) or not all(isinstance(v, str) for v in data.get(field)):
                errors.append(f"{name}: {field} must be a list of strings")
        for skill in data.get("skills") if isinstance(data.get("skills"), list) else []:
            if not (root / ".agents/skills" / str(skill) / "SKILL.md").is_file():
                errors.append(f"{name}: unknown skill {skill!r}")
        check_autonomy(name, data.get("autonomy"), classes)
    for prompt in sorted(agents.glob("*.prompt.md")) if agents.is_dir() else []:
        if not (agents / (prompt.name[: -len(".prompt.md")] + ".json")).is_file():
            errors.append(f"{rel(prompt)}: no matching declaration")


def in_docs(path):
    return path.resolve().is_relative_to(root / "docs")


def check_links(path):
    text = strip_code(path.read_text())
    for target in re.findall(r"\]\(\s*<?([^)\s>]+)>?(?:\s+\"[^\"]*\")?\s*\)", text):
        if target.startswith(BLOB):
            base, local = root, target[len(BLOB):]
        elif re.match(r"^[a-z][a-z0-9+.-]*:", target, re.I):
            continue
        else:
            base, local = path.parent, target
        file_part, _, anchor = local.partition("#")
        if not file_part and not anchor:
            continue
        dest = path if not file_part else (base / file_part).resolve()
        if base is not root and in_docs(path) and not in_docs(dest):
            errors.append(f"{rel(path)}: relative link {target} leaves docs/; use {BLOB}<path>")
        elif not dest.exists():
            errors.append(f"{rel(path)}: dead link {target}")
        elif anchor and dest.suffix == ".md" and anchor not in anchors(dest):
            errors.append(f"{rel(path)}: missing anchor {target}")


def check_owned_phrases(files):
    for phrase, owner in OWNED_PHRASES:
        pattern = re.compile(r"\s+".join(map(re.escape, phrase.split())), re.I)
        for path in files:
            if rel(path) != owner and pattern.search(path.read_text()):
                errors.append(f"{rel(path)}: restates {phrase!r}; link to {owner} instead")


files = docs()
check_skills()
check_agents()
for path in files:
    check_links(path)
check_owned_phrases(files)
for error in errors:
    print(f"FAIL: {error}", file=sys.stderr)
if errors:
    sys.exit(1)
print(f"check-agent-docs: OK ({len(files)} files)")
PY
