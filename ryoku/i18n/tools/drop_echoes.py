#!/usr/bin/env python3
"""Undo translations that only echo the English source, changed since BASE.

  drop_echoes.py [BASE]    (BASE defaults to main)

A translation run sometimes answers with the English string itself
("OPEN SHIMA SETTINGS": "OPEN SHIMA SETTINGS"). For every catalog in the
working tree, each entry whose value equals its key and differs from BASE is
undone: a key BASE translated gets BASE's translation back, a key BASE lacked
is removed so the next run translates it again. Echoes BASE already had are
left alone. The output uses sync.py's layout so a later sync run writes no diff.
"""
import glob
import json
import os
import subprocess
import sys

CATALOG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "catalog")


def load_base(ref, path):
    rel = os.path.relpath(path, toplevel())
    shown = subprocess.run(["git", "show", f"{ref}:{rel}"],
                           capture_output=True, text=True)
    return json.loads(shown.stdout) if shown.returncode == 0 else {}


def toplevel():
    return subprocess.run(["git", "rev-parse", "--show-toplevel"], check=True,
                          capture_output=True, text=True, cwd=CATALOG).stdout.strip()


def drop(base, cur):
    restored = removed = 0
    for key, value in list(cur.items()):
        if value != key or base.get(key) == value:
            continue
        if key in base:
            cur[key] = base[key]
            restored += 1
        else:
            del cur[key]
            removed += 1
    return restored, removed


def main():
    ref = sys.argv[1] if len(sys.argv) > 1 else "main"
    # An unknown ref would read every base as empty and drop every echo.
    if subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}"],
                      capture_output=True, cwd=CATALOG).returncode:
        sys.exit(f"drop_echoes: unknown ref {ref}")
    for path in sorted(glob.glob(os.path.join(CATALOG, "*.json"))):
        if os.path.basename(path) == "en.json":
            continue
        with open(path, encoding="utf-8") as fh:
            cur = json.load(fh)
        restored, removed = drop(load_base(ref, path), cur)
        if not restored + removed:
            continue
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(dict(sorted(cur.items())), fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        print(f"{os.path.basename(path)}: {restored} restored, {removed} removed")


if __name__ == "__main__":
    main()
