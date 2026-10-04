#!/usr/bin/env bash
# The orchestrator resolves its settings and owns ~/.codex/config.toml; this
# helper only provides the Codex agent policy to merge into it.

# codex_update_policy_config <codex-version>
# A pinned Codex must not offer to replace itself with another release.
codex_update_policy_config() {
    if [[ -n "${1:-}" && "$1" != "latest" ]]; then
        printf 'check_for_update_on_startup = false\n'
    fi
}
