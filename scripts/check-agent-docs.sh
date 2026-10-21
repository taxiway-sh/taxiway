#!/usr/bin/env bash
# Check agent docs and docs/ pages: skill frontmatter and discovery symlinks,
# scheduled agent declarations and heartbeat schema,
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
AUTONOMY = {"ask"}
HEARTBEAT_SCHEMA = "status.schema.json"


def check_agents():
    """Scheduled agent declarations: .agents/agents/<id>.json + <id>.prompt.md."""
    agents = root / ".agents/agents"
    schedules = {}
    schema = agents / HEARTBEAT_SCHEMA
    if agents.is_dir():
        try:
            if "promptVersion" not in json.loads(schema.read_text()).get("required", []):
                errors.append(f"{rel(schema)}: must require promptVersion")
        except (OSError, ValueError) as err:
            errors.append(f"{rel(schema)}: unreadable heartbeat schema: {err}")
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
        elif repos.get("taxiway-sh/taxiway") != "read":
            errors.append(f"{name}: taxiway-sh/taxiway access must be read")
        for field in ("title", "workdir", "model"):
            if not isinstance(data.get(field), str) or not data.get(field):
                errors.append(f"{name}: {field} must be a non-empty string")
        for field in ("skills", "outputs"):
            if not isinstance(data.get(field), list) or not all(isinstance(v, str) for v in data.get(field)):
                errors.append(f"{name}: {field} must be a list of strings")
        for skill in data.get("skills") if isinstance(data.get("skills"), list) else []:
            if not (root / ".agents/skills" / str(skill) / "SKILL.md").is_file():
                errors.append(f"{name}: unknown skill {skill!r}")
        if not isinstance(data.get("autonomy"), str) or data["autonomy"] not in AUTONOMY:
            errors.append(f"{name}: autonomy must be one of {sorted(AUTONOMY)}")
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
