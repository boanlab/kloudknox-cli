#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 BoanLab @ Dankook University
#
# Narrow integration test for kloudknox-cli (kkctl).
# Self  : built locally — host binary (kloudknox-cli/bin/kkctl) + image +
#         in-cluster pod deployment (deployments/kloudknox-cli.yaml).
# Others: KloudKnox core (CRD + namespace + operator + daemonset) pulled via
#         kubectl apply -f <GitHub raw URL> (branch/tag = $KK_REF, default main).

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMP_ROOT="$(cd "${E2E_ROOT}/.." && pwd)"
ART_DIR="${E2E_ROOT}/artifacts"
FIXTURES_DIR="${E2E_ROOT}/fixtures"

# --- knobs ---
KK_REF="${KK_REF:-main}"
KL_NS="${KL_NS:-kloudknox}"
KL_KKCTL_MODE="${KL_KKCTL_MODE:-both}"                 # local | pod | both
KL_KKCTL_POD_DEPLOY="${KL_KKCTL_POD_DEPLOY:-kloudknox-cli}"

SELF_MODULE_DIR="${COMP_ROOT}/kloudknox-cli"
SELF_CLI_YAML="${COMP_ROOT}/deployments/kloudknox-cli.yaml"
SELF_IMAGE="boanlab/kloudknox-cli"
SELF_TAG="${SELF_TAG:-v0.1.3}"
KKCTL_BIN="${SELF_MODULE_DIR}/bin/kkctl"

# Cross-component (KloudKnox core) manifests from GitHub
KK_RAW="https://raw.githubusercontent.com/boanlab/KloudKnox/${KK_REF}/deployments"
KK_MANIFESTS=(
    "00_kloudknox_namespace.yaml"
    "01_kloudknoxpolicy.yaml"
    "02_operator-controller.yaml"
    "04_kloudknox.yaml"
)

PASS=0
FAIL=0
REPORT="${ART_DIR}/report.md"

_ok()   { echo "  [PASS] $1${2:+ — $2}"; PASS=$((PASS+1)); echo "- PASS: $1${2:+ — $2}" >>"${REPORT}"; }
_fail() { echo "  [FAIL] $1${2:+ — $2}" >&2; FAIL=$((FAIL+1)); echo "- FAIL: $1${2:+ — $2}" >>"${REPORT}"; }

_preflight() {
    for bin in kubectl docker jq go; do
        command -v "${bin}" >/dev/null 2>&1 || { echo "missing: ${bin}" >&2; exit 1; }
    done
    if ! kubectl cluster-info >/dev/null 2>&1; then
        echo "kubectl cluster-info failed — cluster not reachable" >&2
        exit 1
    fi
    [[ -d "${SELF_MODULE_DIR}" ]] || { echo "missing ${SELF_MODULE_DIR}" >&2; exit 1; }
    [[ -f "${SELF_CLI_YAML}" ]] || { echo "missing ${SELF_CLI_YAML}" >&2; exit 1; }
}

cmd_up() {
    _preflight
    mkdir -p "${ART_DIR}"

    echo "[up] applying KloudKnox core from GitHub (ref=${KK_REF})"
    for m in "${KK_MANIFESTS[@]}"; do
        echo "  apply ${KK_RAW}/${m}"
        kubectl apply -f "${KK_RAW}/${m}" >/dev/null
    done
    kubectl -n "${KL_NS}" rollout status deploy/kloudknox-operator --timeout=180s
    kubectl -n "${KL_NS}" rollout status daemonset/kloudknox --timeout=180s

    echo "[up] build self host binary + image (${SELF_IMAGE}:${SELF_TAG})"
    make -C "${SELF_MODULE_DIR}" build
    make -C "${SELF_MODULE_DIR}" build-image TAG="${SELF_TAG}"
    docker save "${SELF_IMAGE}:${SELF_TAG}" \
        | sudo ctr -n k8s.io images import -

    echo "[up] apply self (${SELF_CLI_YAML})"
    kubectl apply -f "${SELF_CLI_YAML}"
    kubectl -n "${KL_NS}" rollout status "deploy/${KL_KKCTL_POD_DEPLOY}" --timeout=180s

    echo "[up] done"
}

cmd_down() {
    kubectl delete kloudknoxpolicies.security.boanlab.com --all \
        --ignore-not-found --wait=false >/dev/null 2>&1 || true
    if [[ -f "${SELF_CLI_YAML}" ]]; then
        echo "[down] deleting ${SELF_CLI_YAML}"
        kubectl delete -f "${SELF_CLI_YAML}" --ignore-not-found
    fi
    echo "[down] deleting KloudKnox core from GitHub (ref=${KK_REF})"
    # Reverse order to avoid leaving orphans: daemonset → operator → CRD → ns.
    for m in "04_kloudknox.yaml" "02_operator-controller.yaml" \
             "01_kloudknoxpolicy.yaml" "00_kloudknox_namespace.yaml"; do
        kubectl delete -f "${KK_RAW}/${m}" --ignore-not-found >/dev/null 2>&1 || true
    done
    echo "[down] done"
}

_init_report() {
    mkdir -p "${ART_DIR}"
    {
        echo "# kloudknox-cli (kkctl) test report"
        echo
        echo "- Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "- KK_REF: ${KK_REF}"
        echo "- Modes: ${KL_KKCTL_MODE}"
        echo
    } >"${REPORT}"
}

_kkctl_modes() {
    case "${KL_KKCTL_MODE}" in
        local) echo "local" ;;
        pod)   echo "pod" ;;
        both)  echo "local pod" ;;
        *)     echo "local" ;;
    esac
}

_kkctl_available() {
    local mode="$1"
    case "${mode}" in
        local) [[ -x "${KKCTL_BIN}" ]] ;;
        pod)
            local ready
            ready="$(kubectl -n "${KL_NS}" get deployment "${KL_KKCTL_POD_DEPLOY}" \
                -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)"
            [[ "${ready:-0}" -ge 1 ]]
            ;;
        *) return 1 ;;
    esac
}

# Invoke kkctl in "local" or "pod" mode. For pod mode, "-f <path>" becomes
# "-f -" piped via kubectl exec stdin.
_kkctl() {
    local mode="$1"; shift
    case "${mode}" in
        local)
            "${KKCTL_BIN}" "$@"
            ;;
        pod)
            local args=() file=""
            while (( $# )); do
                if [[ "$1" == "-f" && -n "${2:-}" && "$2" != "-" ]]; then
                    file="$2"
                    args+=("-f" "-")
                    shift 2
                else
                    args+=("$1")
                    shift
                fi
            done
            if [[ -n "${file}" ]]; then
                kubectl -n "${KL_NS}" exec -i \
                    "deployment/${KL_KKCTL_POD_DEPLOY}" -- \
                    kkctl "${args[@]}" <"${file}"
            else
                kubectl -n "${KL_NS}" exec \
                    "deployment/${KL_KKCTL_POD_DEPLOY}" -- \
                    kkctl "${args[@]}"
            fi
            ;;
    esac
}

_check_mode() {
    local mode="$1" tag="[$1]"

    local ver_out
    if ver_out="$(_kkctl "${mode}" version 2>&1)"; then
        _ok "kkctl ${tag} version" "${ver_out}"
    else
        _fail "kkctl ${tag} version" "${ver_out}"
    fi

    local status_out
    if status_out="$(_kkctl "${mode}" --env k8s \
            --namespace "${KL_NS}" status 2>&1)"; then
        _ok "kkctl ${tag} status" \
            "$(echo "${status_out}" | grep -i 'daemonset\|ready' | head -1)"
    else
        _fail "kkctl ${tag} status" "$(echo "${status_out}" | head -1)"
    fi

    if _kkctl "${mode}" policy validate \
            -f "${FIXTURES_DIR}/valid-policy.yaml" >/dev/null 2>&1; then
        _ok "kkctl ${tag} policy validate (valid)" ""
    else
        _fail "kkctl ${tag} policy validate (valid)" "unexpectedly failed"
    fi

    if ! _kkctl "${mode}" policy validate \
            -f "${FIXTURES_DIR}/invalid-no-selector.yaml" >/dev/null 2>&1; then
        _ok "kkctl ${tag} policy validate (invalid)" "correctly rejected"
    else
        _fail "kkctl ${tag} policy validate (invalid)" "unexpectedly accepted"
    fi
}

# Apply the valid fixture via `kubectl apply` and wait for the operator to set
# status=Active. This exercises kkctl's fixture end-to-end without relying on
# kkctl apply itself (that path may need extra auth setup in some envs).
_check_operator_integration() {
    local name="clitest-valid"
    kubectl delete kloudknoxpolicies.security.boanlab.com "${name}" \
        --ignore-not-found >/dev/null 2>&1 || true

    if ! kubectl apply -f "${FIXTURES_DIR}/valid-policy.yaml" >/dev/null 2>&1; then
        _fail "policy apply → operator" "kubectl apply failed"
        return
    fi

    local deadline=$(( SECONDS + 30 )) got=""
    while (( SECONDS < deadline )); do
        got="$(kubectl get kloudknoxpolicies.security.boanlab.com "${name}" \
            -o jsonpath='{.status.status}' 2>/dev/null || echo "")"
        [[ "${got}" == "Active" ]] && break
        sleep 1
    done
    if [[ "${got}" == "Active" ]]; then
        _ok "policy apply → operator Active" ""
    else
        _fail "policy apply → operator Active" "status=${got:-<empty>}"
    fi
    kubectl delete kloudknoxpolicies.security.boanlab.com "${name}" \
        --ignore-not-found >/dev/null 2>&1 || true
}

cmd_check() {
    _preflight
    _init_report

    local ran_any=0
    for mode in $(_kkctl_modes); do
        if _kkctl_available "${mode}"; then
            echo "[check] mode=${mode}"
            _check_mode "${mode}"
            ran_any=1
        else
            _fail "kkctl ${mode} mode" "not available (binary missing or pod not ready)"
        fi
    done

    if (( ran_any )); then
        echo "[check] operator integration (kubectl apply → Active)"
        _check_operator_integration
    fi

    _summary
}

_summary() {
    echo
    echo "[summary] passed: ${PASS}  failed: ${FAIL}"
    echo "          report: ${REPORT}"
    {
        echo
        echo "## summary"
        echo "- passed: ${PASS}"
        echo "- failed: ${FAIL}"
    } >>"${REPORT}"
    (( FAIL == 0 ))
}

cmd_all() {
    cmd_up
    cmd_check
}

usage() {
    cat <<'EOF'
Usage: test.sh <command> [args]

Commands:
  up       apply KloudKnox core (GitHub, KK_REF), build kkctl host binary +
           image, apply deployments/kloudknox-cli.yaml, wait for rollouts
  check    run kkctl version/status/validate in configured modes + operator
           apply→Active integration (default target)
  down     reverse of up — delete self + KloudKnox core
  all      up + check

Environment:
  KK_REF                   GitHub ref for cross-component manifests  (default: main)
  KL_NS                    KloudKnox namespace                       (default: kloudknox)
  KL_KKCTL_MODE            local | pod | both                        (default: both)
  KL_KKCTL_POD_DEPLOY      deployment name for pod mode              (default: kloudknox-cli)
  SELF_TAG                 self image tag to build                   (default: v0.1.3)
EOF
}

main() {
    local cmd="${1:-check}"
    shift || true
    case "${cmd}" in
        up)     cmd_up "$@" ;;
        check)  cmd_check "$@" ;;
        down)   cmd_down "$@" ;;
        all)    cmd_all "$@" ;;
        ""|-h|--help|help) usage ;;
        *) echo "unknown command: ${cmd}" >&2; usage >&2; exit 2 ;;
    esac
}

main "$@"
