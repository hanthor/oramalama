#!/usr/bin/env bash
# Bash completion for oramalama
_oramalama_complete() {
    local cur prev words cword
    _init_completion || return

    local subcommands="run serve launch list ls pull ps stop show rm search help"
    local global_flags="--remote --model --dry-run --help"

    # If no subcommand yet, complete subcommands + global flags
    local subcommand=""
    for word in "${words[@]:1:$((cword-1))}"; do
        case "$word" in
            run|serve|launch|list|ls|pull|ps|stop|show|rm|search) subcommand="$word"; break ;;
        esac
    done

    if [ -z "$subcommand" ]; then
        COMPREPLY=($(compgen -W "$subcommands $global_flags" -- "$cur"))
        return
    fi

    # Model completion for subcommands that take a model
    case "$subcommand" in
        run|serve|pull|rm|show|stop)
            if [[ "$prev" == "$subcommand" || "$prev" == "--model" ]]; then
                local models
                models=$(ramalama list --json 2>/dev/null | jq -r '.[].name' 2>/dev/null)
                COMPREPLY=($(compgen -W "$models" -- "$cur"))
            fi
            ;;
        launch)
            case "$prev" in
                --tool) COMPREPLY=($(compgen -W "opencode goose server" -- "$cur")) ;;
                *)      COMPREPLY=($(compgen -W "--tool --model" -- "$cur")) ;;
            esac
            ;;
    esac
}

complete -F _oramalama_complete oramalama
complete -F _oramalama_complete opencode-rl
