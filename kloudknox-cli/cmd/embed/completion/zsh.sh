#compdef kkctl
# kkctl zsh completion

_kkctl() {
    local state line
    typeset -A opt_args

    local -a commands
    commands=(
        'probe:Pre-flight environment check'
        'install:Deploy KloudKnox'
        'uninstall:Remove KloudKnox'
        'upgrade:Upgrade KloudKnox image'
        'status:Show KloudKnox status'
        'apply:Upsert policies from YAML'
        'delete:Delete a policy'
        'get:List resources'
        'describe:Show resource detail'
        'label:Inject labels into container or pod'
        'inject:Inject policy selector labels'
        'policy:Policy subcommands'
        'stream:Stream events, alerts, or logs over gRPC'
        'sysdump:Collect debug bundle'
        'completion:Print shell completion script'
        'version:Print kkctl version'
    )

    local -a global_flags
    global_flags=(
        '--env[Target environment]:env:(k8s docker)'
        '--kubeconfig[Path to kubeconfig]:file:_files'
        '--kube-context[kubeconfig context name]:context:'
        '--namespace[Kubernetes namespace]:namespace:'
        '--server[gRPC server addr]:addr:'
        '-o[Output format]:format:(table wide json yaml)'
    )

    _arguments -C \
        $global_flags \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            _describe 'commands' commands
            ;;
        args)
            case $words[1] in
                apply)
                    _arguments \
                        '-f[YAML file]:file:_files' \
                        '--dry-run[Dry run]' \
                        '--policy-dir[Policy directory]:dir:_files -/'
                    ;;
                delete)
                    _arguments \
                        '-f[YAML file]:file:_files' \
                        '--ignore-not-found[Suppress not-found errors]' \
                        '--policy-dir[Policy directory]:dir:_files -/' \
                        '*:name:'
                    ;;
                get)
                    _arguments \
                        '-A[All namespaces]' \
                        '--policy-dir[Policy directory]:dir:_files -/' \
                        '1:resource:(policies nodes)'
                    ;;
                describe)
                    _arguments '1:resource:(policy container node)' '2:name:'
                    ;;
                install)
                    _arguments \
                        '--image[Container image]:image:' \
                        '--wait[Wait for rollout]' \
                        '--skip-apparmor-webhook[Skip AppArmor webhook (BPF-LSM-only clusters)]'
                    ;;
                upgrade)
                    _arguments \
                        '--image[Container image]:image:' \
                        '--dry-run[Dry run]'
                    ;;
                uninstall)
                    _arguments '--purge-policies[Also delete policies]'
                    ;;
                label)
                    _arguments '1:resource:(container pod)' '2:name:'
                    ;;
                inject)
                    _arguments \
                        '-f[Policy YAML]:file:_files' \
                        '--target[Target resource]:target:(container pod)'
                    ;;
                policy)
                    _arguments '1:subcommand:(validate)'
                    ;;
                stream)
                    _arguments \
                        '1:target:(events alerts logs)' \
                        '--eventName[Filter by event name]:name:' \
                        '--source[Filter by source]:source:' \
                        '--category[Filter by category]:category:' \
                        '--operation[Filter by operation]:operation:' \
                        '--resource[Filter by resource]:resource:' \
                        '--data[Filter by data substring]:data:' \
                        '--nodeName[Filter by node]:name:' \
                        '--namespaceName[Filter by namespace]:ns:' \
                        '--podName[Filter by pod]:name:' \
                        '--containerName[Filter by container]:name:' \
                        '--labels[Filter by labels]:labels:' \
                        '--level[Filter log level (logs only)]:level:(INFO WARN ERROR)' \
                        '--server[gRPC server addr]:addr:'
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh)'
                    ;;
                sysdump)
                    _arguments '-o[Output file]:file:_files'
                    ;;
            esac
            ;;
    esac
}

_kkctl "$@"
