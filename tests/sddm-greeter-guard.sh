#!/usr/bin/env bash
# The login screen guard restarts SDDM only when the daemon runs with no helper
# at all (no greeter, no session) past the grace period, a bounded number of
# times per boot, and says so in the system log. A fake /proc tree stands in
# for the process table, a stub ryoku-host records the restart, a stub logger
# collects the reports; the host's services are never touched.
set -euo pipefail
repo=${RYOKU_PATH:-$(cd "$(dirname "$0")/.." && pwd)}
guard=$repo/ryoku/lockscreen/sddm/ryoku-greeter-guard
manifest=$repo/system/services/manifest.tsv
payload=$repo/release/packages/ryoku-desktop/payload.sh
tmp=$(mktemp -d)
guard_pid=
monitor_pid=
cleanup() {
  [ -z "$guard_pid" ] || kill "$guard_pid" 2>/dev/null || true
  [ -z "$monitor_pid" ] || kill "$monitor_pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT
fail() { printf 'sddm-greeter-guard: %s\n' "$*" >&2; exit 1; }

# delivery: one manifest row on both inits, shipped by the desktop payload.
awk -F '\t' '$1 == "greeter-guard" && $2 == "system" && $3 == "boot" &&
  $4 == "ryoku-greeter-guard.service" && $5 == "ryoku-greeter-guard" { found = 1 } END { exit !found }' "$manifest" ||
  fail "manifest lacks the greeter-guard boot row for both inits"
grep -qF 'lockscreen/sddm/ryoku-greeter-guard"' "$payload" || fail "the desktop payload does not ship the guard"
grep -qF 'ryoku-greeter-guard.service' "$payload" || fail "the desktop payload does not ship the guard's unit"
[ -x "$repo/void/init/system/ryoku-greeter-guard/run" ] || fail "no runit service for the guard"

bin=$tmp/bin
mkdir -p "$bin"
cat >"$bin/ryoku-host" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$RYOKU_TEST_HOST_LOG"
STUB
cat >"$bin/logger" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$RYOKU_TEST_LOGGER_LOG"
STUB
chmod +x "$bin/ryoku-host" "$bin/logger"

# the fake process table: one directory per pid with the kernel's comm file.
reset_proc() { rm -rf "$tmp/proc"; mkdir -p "$tmp/proc"; }
add_proc() { mkdir -p "$tmp/proc/$1"; printf '%s\n' "$2" >"$tmp/proc/$1/comm"; }

once() {
  RYOKU_GREETER_GUARD_PROC="$tmp/proc" \
    RYOKU_GREETER_GUARD_STATE="$tmp/state" \
    RYOKU_GREETER_GUARD_HOST="$bin/ryoku-host" \
    RYOKU_GREETER_GUARD_GRACE="${1:-0}" \
    RYOKU_GREETER_GUARD_LIMIT="${2:-3}" \
    RYOKU_TEST_HOST_LOG="$tmp/host" \
    RYOKU_TEST_LOGGER_LOG="$tmp/logger" \
    PATH="$bin:$PATH" \
    "$guard" --once && echo 0 || echo $?
}
restarts() { grep -c 'svc --system restart sddm' "$tmp/host" 2>/dev/null || true; }
clean() { rm -rf "$tmp/state"; : >"$tmp/host"; : >"$tmp/logger"; }
wait_for_restart() {
  for _ in {1..100}; do
    [ "$(restarts)" -ge 1 ] && return 0
    sleep 0.05
  done
  return 1
}

# --- the greeter is up: nothing happens, no state is kept.
clean; reset_proc; add_proc 600 sddm; add_proc 745 sddm-helper; add_proc 837 sddm-greeter-qt
[ "$(once)" = 0 ] || fail "a running greeter made the guard act"
[ "$(restarts)" = 0 ] || fail "a running greeter was restarted"
[ ! -e "$tmp/state/missing-since" ] || fail "a running greeter started the grace clock"

# --- a user is logged in and the greeter is gone (the normal state after a
# login): the session helper counts, nothing happens.
clean; reset_proc; add_proc 600 sddm; add_proc 970 sddm-helper; add_proc 1246 niri
[ "$(once)" = 0 ] || fail "a logged-in session made the guard act"
[ "$(restarts)" = 0 ] || fail "a logged-in session's display manager was restarted"

# --- no display manager at all (boot, a box without SDDM): nothing happens.
clean; reset_proc; add_proc 1 runit; add_proc 623 agetty
[ "$(once)" = 0 ] || fail "a box without sddm made the guard act"
[ "$(restarts)" = 0 ] || fail "a box without sddm was restarted"

# --- sddm alone: the grace clock starts on the first look, and the restart
# comes only once the grace period has passed.
clean; reset_proc; add_proc 600 sddm; add_proc 623 agetty
[ "$(once 3600)" = 0 ] || fail "the first look at a bare sddm acted at once"
[ -f "$tmp/state/missing-since" ] || fail "the grace clock did not start"
[ "$(once 3600)" = 0 ] || fail "a bare sddm inside the grace period was acted on"
[ "$(restarts)" = 0 ] || fail "sddm was restarted inside the grace period"
printf '%s\n' "$(( $(date +%s) - 100 ))" >"$tmp/state/missing-since"
[ "$(once 20)" = 3 ] || fail "a bare sddm past the grace period was not restarted"
[ "$(restarts)" = 1 ] || fail "expected one restart, got $(restarts)"
grep -q -- '-t ryoku-greeter-guard -p daemon.err' "$tmp/logger" || fail "the restart was not reported to the system log"
grep -q 'no login screen and runs no session for 20s; restarting the display manager (1 of 3 this boot)' "$tmp/logger" ||
  fail "the report does not say what happened: $(cat "$tmp/logger")"
[ ! -e "$tmp/state/missing-since" ] || fail "the grace clock was not reset after the restart"

# --- the greeter came back after the restart: the clock stays off and the
# restart count stays.
add_proc 745 sddm-helper
[ "$(once 0)" = 0 ] || fail "a recovered greeter made the guard act"
[ ! -e "$tmp/state/missing-since" ] || fail "a recovered greeter started the grace clock"
[ "$(cat "$tmp/state/restarts")" = 1 ] || fail "the restart count was lost"

# --- it keeps dying: the cap holds and the guard says it gave up, once.
rm -rf "$tmp/proc/745"
[ "$(once 0)" = 0 ] || fail "the second disappearance acted before its grace clock"
[ "$(once 0)" = 3 ] || fail "the second restart was not issued"
[ "$(once 0)" = 0 ] || fail "the third disappearance acted before its grace clock"
[ "$(once 0)" = 3 ] || fail "the third restart was not issued"
[ "$(restarts)" = 3 ] || fail "expected three restarts, got $(restarts)"
[ "$(once 0)" = 0 ] || fail "the fourth disappearance acted before its grace clock"
[ "$(once 0)" = 0 ] || fail "the guard restarted past its cap"
[ "$(restarts)" = 3 ] || fail "the cap did not hold: $(restarts) restarts"
[ "$(grep -c 'after 3 restarts this boot; leaving it' "$tmp/logger")" = 1 ] ||
  fail "giving up was not reported exactly once: $(cat "$tmp/logger")"
grep -q 'sddm-helper-start-wayland/sddm.log' "$tmp/logger" || fail "the give-up report does not say where to look"

# --- session signals trigger an immediate re-check, and the monitor is reaped
# with the guard.
cat >"$bin/dbus-monitor" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$BASHPID" >"$RYOKU_TEST_DBUS_PID"
exec 3<>"$RYOKU_TEST_DBUS_EVENTS"
while IFS= read -r event <&3; do
  printf '%s\n' 'signal interface=org.freedesktop.login1.Manager; member=SessionRemoved'
done
STUB
chmod +x "$bin/dbus-monitor"
clean; reset_proc; add_proc 600 sddm; add_proc 745 sddm-helper
mkfifo "$tmp/dbus-events"
RYOKU_GREETER_GUARD_PROC="$tmp/proc" \
  RYOKU_GREETER_GUARD_STATE="$tmp/state" \
  RYOKU_GREETER_GUARD_HOST="$bin/ryoku-host" \
  RYOKU_GREETER_GUARD_DBUS_MONITOR="$bin/dbus-monitor" \
  RYOKU_GREETER_GUARD_GRACE=0 \
  RYOKU_GREETER_GUARD_INTERVAL=0.05 \
  RYOKU_GREETER_GUARD_FALLBACK=30 \
  RYOKU_TEST_DBUS_EVENTS="$tmp/dbus-events" \
  RYOKU_TEST_DBUS_PID="$tmp/dbus-pid" \
  RYOKU_TEST_HOST_LOG="$tmp/host" \
  RYOKU_TEST_LOGGER_LOG="$tmp/logger" \
  PATH="$bin:$PATH" \
  "$guard" >"$tmp/event-guard.out" 2>&1 &
guard_pid=$!
for _ in {1..40}; do [ -r "$tmp/dbus-pid" ] && break; sleep 0.05; done
[ -r "$tmp/dbus-pid" ] || fail "the login1 event monitor did not start"
monitor_pid=$(cat "$tmp/dbus-pid")
rm -rf "$tmp/proc/745"
printf 'removed\nremoved\n' >"$tmp/dbus-events"
wait_for_restart || fail "a login1 session event did not wake the guard"
kill "$guard_pid"
wait "$guard_pid" 2>/dev/null || true
guard_pid=
kill -0 "$monitor_pid" 2>/dev/null && fail "the login1 event monitor leaked after guard exit"
monitor_pid=

# --- after a restart, keep the short cadence long enough to catch a new
# greeter that disappears before login1 can publish a session signal.
clean; reset_proc; add_proc 600 sddm; add_proc 745 sddm-helper
mkdir -p "$tmp/state"
printf '%s\n' "$(date +%s)" >"$tmp/state/restarted-at"
rm -f "$tmp/dbus-pid"
RYOKU_GREETER_GUARD_PROC="$tmp/proc" \
  RYOKU_GREETER_GUARD_STATE="$tmp/state" \
  RYOKU_GREETER_GUARD_HOST="$bin/ryoku-host" \
  RYOKU_GREETER_GUARD_DBUS_MONITOR="$bin/dbus-monitor" \
  RYOKU_GREETER_GUARD_GRACE=1 \
  RYOKU_GREETER_GUARD_INTERVAL=0.05 \
  RYOKU_GREETER_GUARD_FALLBACK=30 \
  RYOKU_TEST_DBUS_EVENTS="$tmp/dbus-events" \
  RYOKU_TEST_DBUS_PID="$tmp/dbus-pid" \
  RYOKU_TEST_HOST_LOG="$tmp/host" \
  RYOKU_TEST_LOGGER_LOG="$tmp/logger" \
  PATH="$bin:$PATH" \
  "$guard" >"$tmp/restart-grace.out" 2>&1 &
guard_pid=$!
for _ in {1..40}; do [ -r "$tmp/dbus-pid" ] && break; sleep 0.05; done
[ -r "$tmp/dbus-pid" ] || fail "the post-restart login1 monitor did not start"
monitor_pid=$(cat "$tmp/dbus-pid")
rm -rf "$tmp/proc/745"
wait_for_restart || fail "post-restart grace fell back to the long check interval"
kill "$guard_pid"
wait "$guard_pid" 2>/dev/null || true
guard_pid=
kill -0 "$monitor_pid" 2>/dev/null && fail "the post-restart login1 monitor leaked"
monitor_pid=

# --- without dbus-monitor the original interval poll remains the fallback.
clean; reset_proc; add_proc 600 sddm; add_proc 745 sddm-helper
RYOKU_GREETER_GUARD_PROC="$tmp/proc" \
  RYOKU_GREETER_GUARD_STATE="$tmp/state" \
  RYOKU_GREETER_GUARD_HOST="$bin/ryoku-host" \
  RYOKU_GREETER_GUARD_DBUS_MONITOR="$tmp/no-dbus-monitor" \
  RYOKU_GREETER_GUARD_GRACE=0 \
  RYOKU_GREETER_GUARD_INTERVAL=0.05 \
  RYOKU_TEST_HOST_LOG="$tmp/host" \
  RYOKU_TEST_LOGGER_LOG="$tmp/logger" \
  PATH="$bin:$PATH" \
  "$guard" >"$tmp/poll-guard.out" 2>&1 &
guard_pid=$!
rm -rf "$tmp/proc/745"
wait_for_restart || fail "the no-dbus-monitor fallback stopped polling"
kill "$guard_pid"
wait "$guard_pid" 2>/dev/null || true
guard_pid=

printf 'sddm-greeter-guard: PASS\n'
