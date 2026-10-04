#!/usr/bin/env bash
# Translate the orchestrator's autonomous guest contract to native CLI flags.
claude_code_exec_autonomous() {
    local command="$1"
    shift
    if [[ "${command##*/}" == claude ]]; then
        # The one-time bypass warning is separate from authentication/onboarding.
        exec "$command" --dangerously-skip-permissions \
            --settings '{"skipDangerousModePermissionPrompt":true}' "$@"
    fi
    exec "$command" "$@"
}
