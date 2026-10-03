#!/usr/bin/env bash
# Install an agent distributed as a global npm package.
#
# The requested version is either "latest" or an exact release (X.Y.Z with an
# optional prerelease suffix). Ranges and other npm tags are rejected so a pin
# never resolves to a moving target.

# npm_agent_install <agent> <setting> <package> <requested> <version-cmd...>
#
# <version-cmd> prints the version of the installed executable, or nothing
# when it is missing.
npm_agent_install() {
  local agent="$1" setting="$2" pkg="$3" requested="$4"
  shift 4

  if [[ "$requested" != "latest" && ! "$requested" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    printf '%s version %q is not supported: use latest or an exact release such as 1.2.3\n' "$agent" "$requested" >&2
    printf '  Change it with: taxiway install %s --set %s=<version>\n' "${TAXIWAY_LAB:-<lab>}" "$setting" >&2
    return 1
  fi

  local current
  current="$("$@")"
  if [[ -n "$current" && ( "$requested" == "latest" || "$current" == "$requested" ) ]]; then
    log "$agent already installed (version: $current) - skipping"
    return 0
  fi

  log "Installing ${pkg}@${requested}"
  local npm_err
  npm_err="$(mktemp -t npm-err.XXXXXX)"
  if ! npm install -g "${pkg}@${requested}" 2>"$npm_err"; then
    if grep -qi 'EACCES\|permission denied' "$npm_err"; then
      log "Retrying with sudo"
      rm -f "$npm_err"
      sudo npm install -g "${pkg}@${requested}" || return 1
    else
      cat "$npm_err" >&2
      rm -f "$npm_err"
      printf '  Check the release exists with: npm view %s versions\n' "$pkg" >&2
      printf '  Change it with: taxiway install %s --set %s=<version>\n' "${TAXIWAY_LAB:-<lab>}" "$setting" >&2
      return 1
    fi
  fi
  rm -f "$npm_err"

  local installed
  installed="$("$@")"
  [[ -n "$installed" ]] || { printf '%s not found after install\n' "$agent" >&2; return 1; }
  log "Installed: $agent $installed"
}
