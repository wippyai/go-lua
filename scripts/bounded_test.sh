#!/usr/bin/env bash
# Run one test command inside a separately killable session with both a Go
# heap target and an external process-tree RSS/time ceiling.  The RSS watchdog
# is authoritative: it also catches native allocations, subprocesses, and a
# non-converging test that keeps allocating despite Go's soft memory limit.

set -euo pipefail

if (( $# == 0 )); then
    echo "usage: scripts/bounded_test.sh <command> [args...]" >&2
    exit 2
fi

rss_limit_mib=${TEST_RSS_LIMIT_MIB:-20480}
time_limit_seconds=${TEST_TIMEOUT_SECONDS:-120}
go_memory_limit=${TEST_GO_MEMORY_LIMIT:-18GiB}
poll_seconds=${TEST_POLL_SECONDS:-0.20}
go_cache=${TEST_GOCACHE:-/tmp/go-lua-bounded-go-build}

case $rss_limit_mib in
    ''|*[!0-9]*) echo "TEST_RSS_LIMIT_MIB must be a positive integer" >&2; exit 2 ;;
esac
case $time_limit_seconds in
    ''|*[!0-9]*) echo "TEST_TIMEOUT_SECONDS must be a positive integer" >&2; exit 2 ;;
esac
if (( rss_limit_mib == 0 || time_limit_seconds == 0 )); then
    echo "memory and time limits must be non-zero" >&2
    exit 2
fi

for required in setsid timeout ps awk; do
    if ! command -v "$required" >/dev/null 2>&1; then
        echo "required safeguard command is unavailable: $required" >&2
        exit 2
    fi
done

# A bounded invocation may itself run a semantic gate through this script.
# A second `setsid` would put that gate outside the parent watchdog's process
# tree, so an active parent is a cooperative inheritance contract: it supplies
# both this marker and an unlinked descriptor whose recorded session must equal
# this process's actual session.  An active marker with invalid capability is a
# safety failure, never permission to create a detached session.  This is not
# hostile-process containment: same-UID hostile containment needs a kernel
# cgroup/supervisor boundary and is intentionally outside this script.
bounded_parent_session() {
    local descriptor=${WIPPY_BOUNDED_CAPABILITY_FD:-}
    if [[ "$descriptor" != "9" || ! -r "/proc/self/fd/${descriptor}" ]]; then
        return 1
    fi
    local marker
    if ! IFS= read -r marker < "/proc/self/fd/${descriptor}"; then
        return 1
    fi
    if [[ ! "$marker" =~ ^wippy-bounded-v1:([1-9][0-9]*)$ ]]; then
        return 1
    fi
    local session
    session=$(ps -o sid= -p "$$" | awk '{$1=$1; print}')
    [[ "$session" == "${BASH_REMATCH[1]}" ]]
}

if [[ -n ${WIPPY_BOUNDED_ACTIVE:-} ]]; then
    if [[ ${WIPPY_BOUNDED_ACTIVE} != "wippy-bounded-v1" ]] || ! bounded_parent_session; then
        echo "bounded test aborted: invalid inherited bounded-runner capability" >&2
        exit 125
    fi
    # The parent runner still owns the RSS and wall-clock limits for this
    # session.  Replacing this wrapper leaves the gate in that killable tree.
    exec "$@"
fi

# Keep Go's writable build cache inside the sandbox-owned temporary area. This
# lets callers use the plain bounded command without per-run environment
# assignments (and their corresponding approval prompts).
mkdir -p -- "$go_cache"

rss_limit_kib=$((rss_limit_mib * 1024))
started=$SECONDS
reason=
peak_rss_kib=0

# `timeout` is a second, independent wall-clock guard.  `setsid` gives the
# watchdog an exact process-tree boundary that cannot include the invoking
# shell or another developer's tests.  The session leader writes its identity
# into an unlinked inherited descriptor; only a descendant of that session can
# use it to avoid creating a detached nested session.
capability_file=$(mktemp "${TMPDIR:-/tmp}/wippy-bounded-capability.XXXXXX")
cleanup_capability() {
    rm -f -- "$capability_file"
}
trap cleanup_capability EXIT
setsid bash -c '
    capability_file=$1
    time_limit_seconds=$2
    go_memory_limit=$3
    go_cache=$4
    shift 4
    printf "wippy-bounded-v1:%s\\n" "$$" > "$capability_file"
    exec 9<"$capability_file"
    rm -f -- "$capability_file"
    exec env WIPPY_BOUNDED_ACTIVE=wippy-bounded-v1 WIPPY_BOUNDED_CAPABILITY_FD=9 GOMEMLIMIT="$go_memory_limit" GOCACHE="$go_cache" timeout --signal=TERM --kill-after=3s "${time_limit_seconds}s" "$@"
' bash "$capability_file" "$time_limit_seconds" "$go_memory_limit" "$go_cache" "$@" &
leader=$!

session_rss_kib() {
    ps -e -o sid=,rss= | awk -v wanted="$leader" '$1 == wanted { total += $2 } END { print total + 0 }'
}

session_pids() {
    ps -e -o pid=,sid= | awk -v wanted="$leader" '$2 == wanted { print $1 }'
}

terminate_session() {
    local signal=$1
    local pid
    while read -r pid; do
        if [[ -n $pid ]]; then
            kill "-$signal" "$pid" 2>/dev/null || true
        fi
    done < <(session_pids)
}

while kill -0 "$leader" 2>/dev/null; do
    rss_kib=$(session_rss_kib)
    if (( rss_kib > peak_rss_kib )); then
        peak_rss_kib=$rss_kib
    fi
    if (( rss_kib > rss_limit_kib )); then
        reason="RSS ceiling exceeded: $((rss_kib / 1024)) MiB > ${rss_limit_mib} MiB"
        terminate_session TERM
        sleep 0.25
        terminate_session KILL
        break
    fi
    if (( SECONDS - started >= time_limit_seconds )); then
        reason="wall-clock ceiling exceeded: ${time_limit_seconds}s"
        terminate_session TERM
        sleep 0.25
        terminate_session KILL
        break
    fi
    sleep "$poll_seconds"
done

set +e
wait "$leader"
status=$?
set -e

echo "bounded test usage: peak process-tree RSS $((peak_rss_kib / 1024)) MiB; elapsed $((SECONDS - started))s" >&2

if [[ -n $reason ]]; then
    echo "bounded test aborted: $reason" >&2
    exit 125
fi
if (( status == 124 || status == 137 )); then
    echo "bounded test aborted by timeout/memory safeguard (status $status)" >&2
fi
exit "$status"
