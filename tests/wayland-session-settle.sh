#!/usr/bin/env bash
# hermetic test for ryoku-wayland-session, the SDDM [Wayland] SessionCommand
# wrapper that waits for the user's config bootstrap, and for the greeter
# (weston) to release the GPU, before the session compositor starts (issue #174:
# a hybrid-GPU laptop otherwise misses its iGPU and lands on a black headless
# dGPU; a first login otherwise races ryoku-bootstrap).
#
# Nothing here starts weston, a compositor or a user unit. A fake pgrep on PATH
# reports weston "present" for a controlled number of polls, a fake systemctl
# records the bootstrap start (and can hang or fail on cue), and
# RYOKU_WAYLAND_SESSION_DEFAULT points at a fake session that records that it
# ran and with which arguments.
#
# The load-bearing assertions: the wrapper does NOT hand off while weston is
# still present, always hands off eventually (login never hangs), and passes the
# session arguments through untouched.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
wrapper="$here/../ryoku/lockscreen/sddm/ryoku-wayland-session"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fail() { echo "FAIL: $1" >&2; exit 1; }

bin="$tmp/bin"; mkdir -p "$bin"

# fake pgrep: reports weston present until it has been polled "$tmp/pglimit"
# times, then gone. Every call bumps "$tmp/pgcount" so the test can see whether
# the wrapper actually waited.
cat >"$bin/pgrep" <<EOF
#!/bin/sh
c=\$(cat "$tmp/pgcount" 2>/dev/null || echo 0); c=\$((c + 1)); echo "\$c" >"$tmp/pgcount"
limit=\$(cat "$tmp/pglimit" 2>/dev/null || echo 0)
[ "\$c" -le "\$limit" ] && exit 0   # weston present
exit 1                              # weston gone
EOF
chmod +x "$bin/pgrep"

# fake systemctl: logs its arguments and the order of events, then sleeps
# "$tmp/sc-sleep" seconds and exits "$tmp/sc-rc", so a test can make the
# bootstrap slow or failing without any real user manager.
cat >"$bin/systemctl" <<EOF
#!/bin/sh
printf '%s\n' "\$*" >>"$tmp/systemctl-args"
echo systemctl >>"$tmp/order"
sleep "\$(cat "$tmp/sc-sleep" 2>/dev/null || echo 0)"
exit "\$(cat "$tmp/sc-rc" 2>/dev/null || echo 0)"
EOF
chmod +x "$bin/systemctl"
export PATH="$bin:$PATH"

# fake session: records the arguments it was handed and that it ran at all.
cat >"$bin/fake-session" <<EOF
#!/bin/sh
printf '%s' "\$*" >"$tmp/session-args"
: >"$tmp/session-ran"
echo session >>"$tmp/order"
EOF
chmod +x "$bin/fake-session"
export RYOKU_WAYLAND_SESSION_DEFAULT="$bin/fake-session"

reset() { rm -f "$tmp/pgcount" "$tmp/session-ran" "$tmp/session-args" "$tmp/systemctl-args" "$tmp/order" "$tmp/sc-sleep" "$tmp/sc-rc"; }
polls() { cat "$tmp/pgcount" 2>/dev/null || echo 0; }

# ---- greeter still up: wait for it, then hand off with args intact -----------
reset
echo 3 >"$tmp/pglimit"   # weston present for the first three polls
RYOKU_SESSION_SETTLE_DEADLINE=10 "$wrapper" Hyprland --foo
[[ -e "$tmp/session-ran" ]] || fail "wrapper never handed off to the session"
[[ "$(cat "$tmp/session-args")" == "Hyprland --foo" ]] \
  || fail "session arguments were not passed through: '$(cat "$tmp/session-args")'"
(( "$(polls)" >= 2 )) || fail "wrapper handed off without waiting for weston (polled $(polls)x)"

# ---- no greeter: hand off at once, no wasted polling -------------------------
reset
echo 0 >"$tmp/pglimit"   # weston already gone
"$wrapper" Hyprland
[[ -e "$tmp/session-ran" ]] || fail "wrapper did not hand off when weston was absent"
(( "$(polls)" == 1 )) || fail "wrapper polled $(polls)x when weston was already gone; expected 1"

# ---- greeter never exits: the deadline caps the wait so login never hangs ----
reset
echo 100000 >"$tmp/pglimit"   # weston "present" forever
start=$SECONDS
RYOKU_SESSION_SETTLE_DEADLINE=1 "$wrapper" Hyprland
[[ -e "$tmp/session-ran" ]] || fail "wrapper hung on a greeter that never exits (must cap and hand off)"
(( SECONDS - start <= 4 )) || fail "wrapper waited past its deadline"

# ---- config bootstrap: started before the hand-off, user scope, by name ------
reset
echo 0 >"$tmp/pglimit"
"$wrapper" Hyprland
[[ "$(cat "$tmp/systemctl-args" 2>/dev/null)" == "--user start ryoku-bootstrap.service" ]] \
  || fail "wrapper did not start the config bootstrap: '$(cat "$tmp/systemctl-args" 2>/dev/null)'"
[[ "$(tr '\n' ' ' <"$tmp/order")" == "systemctl session " ]] \
  || fail "the bootstrap must finish before the hand-off, got order: $(tr '\n' ' ' <"$tmp/order")"

# ---- a bootstrap that never finishes: the deadline caps it, login goes on -----
reset
echo 0 >"$tmp/pglimit"; echo 30 >"$tmp/sc-sleep"
start=$SECONDS
RYOKU_SESSION_BOOTSTRAP_DEADLINE=1 "$wrapper" Hyprland
[[ -e "$tmp/session-ran" ]] || fail "wrapper hung on a bootstrap that never finishes"
(( SECONDS - start <= 4 )) || fail "wrapper waited past its bootstrap deadline"

# ---- a failing bootstrap or user manager never blocks the login ---------------
reset
echo 0 >"$tmp/pglimit"; echo 1 >"$tmp/sc-rc"
"$wrapper" Hyprland --bar
[[ "$(cat "$tmp/session-args" 2>/dev/null)" == "Hyprland --bar" ]] \
  || fail "a failed bootstrap stopped the hand-off"

echo "wayland-session-settle: all checks passed"
