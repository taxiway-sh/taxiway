#!/usr/bin/env bash
# Translate the Codex orchestrator's autonomous guest contract.
codex_autonomous_args=(--dangerously-bypass-approvals-and-sandbox)
codex_autonomous_config() {
    printf 'approval_policy = "never"\nsandbox_mode = "danger-full-access"\n'
}
