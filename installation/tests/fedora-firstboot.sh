#!/usr/bin/env bash
set -euo pipefail
[[ ${RYOKU_TEST_DISPOSABLE:-0} == 1 && $EUID == 0 && -f /etc/fedora-release ]] || {
  echo 'requires an explicitly disposable Fedora 44 container' >&2
  exit 1
}
[[ -f /run/.containerenv || -f /.dockerenv ]] || {
  echo 'refusing to change accounts outside a disposable container' >&2
  exit 1
}
root=$(cd "$(dirname "$0")/../.." && pwd)
python3 -m unittest discover -s "$root/installation/fedora/tests" -v
install -Dm755 "$root/installation/fedora/firstboot.py" /usr/libexec/ryoku-firstboot
install -Dm644 "$root/installation/fedora/ryoku-firstboot.service" /etc/systemd/system/ryoku-firstboot.service
install -Dm644 "$root/installation/fedora/firstboot-gate.conf" /etc/systemd/system/sddm.service.d/10-firstboot.conf
# The prompt test needs no graphical packages; verify ordering against a placeholder DM.
cat > /etc/systemd/system/sddm.service <<'EOF'
[Unit]
After=systemd-user-sessions.service
[Service]
ExecStart=/usr/bin/true
EOF
systemd-analyze verify ryoku-firstboot.service sddm.service
python3 "$root/installation/fedora/tests/firstboot-fedora.py"
