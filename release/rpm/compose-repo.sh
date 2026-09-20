#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
out=${1:?COPR output directory}
key=${2:?COPR public key file}
: "${RYOKU_COPR_FINGERPRINT:?set the COPR public-key fingerprint}"
: "${RYOKU_RPM_SIGNING_KEY:?set the repository metadata signing-key fingerprint}"
python3 "$here/verify-rpms.py" "$out" "$key" "$RYOKU_COPR_FINGERPRINT"
mkdir -p "$out/keys"
cp "$key" "$out/keys/copr.asc"
gpg --batch --armor --export "$RYOKU_RPM_SIGNING_KEY" > "$out/keys/metadata.asc"
python3 - "$here" "$out/keys/metadata.asc" "$RYOKU_RPM_SIGNING_KEY" <<'PY'
import importlib.machinery, sys
verify = importlib.machinery.SourceFileLoader('verify', sys.argv[1] + '/verify-rpms.py').load_module()
verify.verify_key(sys.argv[2], sys.argv[3])
PY
release=$(python3 - "$out/release.json" <<'PY'
import json, re, sys
release = json.load(open(sys.argv[1]))['release']
if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?', release):
    sys.exit('invalid frozen release name')
print(release)
PY
)
# Cached channel metadata must still fetch its RPMs after the channel moves.
createrepo_c --location-prefix "../../../../releases/$release/44/x86_64/" "$out"
gpg --batch --yes --local-user "$RYOKU_RPM_SIGNING_KEY" --armor --detach-sign "$out/repodata/repomd.xml"
gpg --verify "$out/repodata/repomd.xml.asc" "$out/repodata/repomd.xml"
