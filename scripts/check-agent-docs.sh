#!/usr/bin/env bash
# Check agent docs and docs/ pages: skill frontmatter and discovery symlinks,
# scheduled agent declarations, autonomy and heartbeat schema,
# relative Markdown links (pages under docs/ reach other files through blob
# URLs), and normative phrases that must stay in their owner.
# Usage: scripts/check-agent-docs.sh [repository-root]
set -euo pipefail
root="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
exec python3 -I - "$root" <<'PY'
import json
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1]).resolve()
# Pages rendered by the site link to repository files through this URL.
BLOB = "https://github.com/taxiway-sh/taxiway/blob/main/"

# Normative phrases and the only file allowed to contain them. Each rule has
# one owner; other files link to it. Matching ignores case and line wrapping.
OWNED_PHRASES = [
    ("`TestE2E_` entry point", "AGENTS.md"),
    ("parallel suite", "AGENTS.md"),
    ("three lifecycle E2E scenarios", "AGENTS.md"),
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


def strip_code(text):
    """Drop fenced blocks (3+ backticks or tildes) and blank inline code."""
    kept, fence = [], None
    for line in text.splitlines():
        match = re.match(r" {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence is None:
            if match:
                fence = match.group(1)
            else:
                kept.append(re.sub(r"`[^`\n]*`", "``", line))
        elif (match and match.group(1)[0] == fence[0]
              and len(match.group(1)) >= len(fence) and not match.group(2).strip()):
            fence = None
    return "\n".join(kept)


def slug(heading):
    """GitHub-style heading anchor."""
    heading = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", heading.strip().lower())
    heading = re.sub(r"[`*]|(?<!\w)_|_(?!\w)", "", heading)
    return re.sub(r"[^\w\- ]", "", heading).replace(" ", "-")


def anchors(path):
    text = path.read_text()
    result = set(re.findall(r"<a\s+(?:id|name)=[\"']([^\"']+)[\"']", text))
    seen = {}
    for line in strip_code(text).splitlines():
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


AGENT_FIELDS = {"id", "title", "schedule", "model", "repositories", "workdir", "skills", "outputs", "autonomy"}
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
APPROVAL_FIELDS = {"class", "proposal", "answer", "date", "link"}
APPROVAL_ANSWERS = ["ok", "refused", "corrected"]


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
    if "approvals" in required:
        errors.append(f"{rel(schema)}: approvals must stay optional")
    props = data.get("properties")
    approvals = props.get("approvals") if isinstance(props, dict) else None
    if not isinstance(approvals, dict) or approvals.get("type") != "array":
        errors.append(f"{rel(schema)}: approvals must be an array")
        return
    items = approvals.get("items")
    if not isinstance(items, dict) or items.get("additionalProperties") is not False:
        errors.append(f"{rel(schema)}: approvals items must be objects without additional properties")
        return
    if not isinstance(items.get("required"), list) or set(items["required"]) != APPROVAL_FIELDS:
        errors.append(f"{rel(schema)}: approvals items must require exactly {sorted(APPROVAL_FIELDS)}")
    fields = items.get("properties")
    answer = fields.get("answer") if isinstance(fields, dict) else None
    if not isinstance(answer, dict) or answer.get("enum") != APPROVAL_ANSWERS:
        errors.append(f"{rel(schema)}: approvals answer must be one of {APPROVAL_ANSWERS}")


def check_agents():
    """Scheduled agent declarations: .agents/agents/<id>.json + <id>.prompt.md."""
    agents = root / ".agents/agents"
    schedules = {}
    schema = agents / HEARTBEAT_SCHEMA
    classes = {}
    if agents.is_dir():
        try:
            check_heartbeat_schema(schema)
        except (OSError, ValueError) as err:
            errors.append(f"{rel(schema)}: unreadable heartbeat schema: {err}")
        classes = action_classes()
    decls = [p for p in agents.glob("*.json") if p.name != HEARTBEAT_SCHEMA] if agents.is_dir() else []
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
        extra = data.keys() - AGENT_FIELDS
        if missing:
            errors.append(f"{name}: missing fields {sorted(missing)}")
        if extra:
            errors.append(f"{name}: unknown fields {sorted(extra)}")
        if data.get("id") != decl.stem:
            errors.append(f"{name}: id must be {decl.stem!r}")
        if not (agents / f"{decl.stem}.prompt.md").is_file():
            errors.append(f"{name}: missing {decl.stem}.prompt.md")
        schedule = data.get("schedule")
        if not isinstance(schedule, str) or len(schedule.split()) != 5:
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
