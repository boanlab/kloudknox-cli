#!/usr/bin/env bash
# kkctl bash completion

_kkctl_completions() {
    local cur prev words cword
    _init_completion || return

    local commands="probe install uninstall upgrade status apply delete get describe label inject policy stream sysdump completion version help"
    local global_flags="--env --kubeconfig --kube-context --namespace --server -o"

    if [[ $cword -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$commands $global_flags" -- "$cur") )
        return
    fi

    local verb="${words[1]}"
    # skip global flags to find the real verb
    for word in "${words[@]:1}"; do
        [[ "$word" == --* ]] && continue
        verb="$word"
        break
    done

    case "$verb" in
        install)
            COMPREPLY=( $(compgen -W "--env --image --wait --skip-apparmor-webhook" -- "$cur") )
            ;;
        upgrade)
            COMPREPLY=( $(compgen -W "--env --image --dry-run" -- "$cur") )
            ;;
        uninstall)
            COMPREPLY=( $(compgen -W "--env --purge-policies" -- "$cur") )
            ;;
        apply)
            case "$prev" in
                -f) COMPREPLY=( $(compgen -f -- "$cur") ); return ;;
            esac
            COMPREPLY=( $(compgen -W "-f --dry-run --policy-dir" -- "$cur") )
            ;;
        delete)
            COMPREPLY=( $(compgen -W "policy -f --ignore-not-found --policy-dir" -- "$cur") )
            ;;
        get)
            COMPREPLY=( $(compgen -W "policies nodes -A --policy-dir -o" -- "$cur") )
            ;;
        describe)
            COMPREPLY=( $(compgen -W "policy container node" -- "$cur") )
            ;;
        label)
            COMPREPLY=( $(compgen -W "container pod" -- "$cur") )
            ;;
        inject)
            case "$prev" in
                -f) COMPREPLY=( $(compgen -f -- "$cur") ); return ;;
            esac
            COMPREPLY=( $(compgen -W "-f --target" -- "$cur") )
            ;;
        policy)
            COMPREPLY=( $(compgen -W "validate" -- "$cur") )
            ;;
        stream)
            COMPREPLY=( $(compgen -W "events alerts logs --eventName --source --category --operation --resource --data --nodeName --namespaceName --podName --containerName --labels" -- "$cur") )
            ;;
        status)
            COMPREPLY=( $(compgen -W "-o" -- "$cur") )
            ;;
        sysdump)
            COMPREPLY=( $(compgen -W "-o" -- "$cur") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") )
            ;;
        probe)
            COMPREPLY=( $(compgen -W "--env" -- "$cur") )
            ;;
        -o)
            COMPREPLY=( $(compgen -W "table wide json yaml" -- "$cur") )
            ;;
        --env)
            COMPREPLY=( $(compgen -W "k8s docker" -- "$cur") )
            ;;
    esac
}

complete -F _kkctl_completions kkctl
