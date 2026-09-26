#!/usr/bin/env python3
"""Three-way merge of one i18n catalog, key by key.

  merge3.py BASE OURS THEIRS OUT

A catalog is a flat JSON object of English source -> translation. Line-based
git merges conflict on every neighbouring key, so this merges entries instead:
a key changed on one side takes that side, a key changed on both takes THEIRS
(the upstream translation run is newer), and a key deleted on one side and
untouched on the other stays deleted. A missing BASE is read as empty.
The output uses sync.py's layout so a later sync run writes no diff.
"""
import json
import sys


def load(path):
    try:
        with open(path, encoding="utf-8") as fh:
            return json.load(fh)
    except FileNotFoundError:
        return {}


def merge(base, ours, theirs):
    out = {}
    for key in base.keys() | ours.keys() | theirs.keys():
        b, o, t = base.get(key), ours.get(key), theirs.get(key)
        if o == t:
            value = o
        elif o == b:
            value = t
        elif t == b:
            value = o
        else:
            value = t if t is not None else o
        if value is not None:
            out[key] = value
    return out


def main():
    if len(sys.argv) != 5:
        sys.exit("usage: merge3.py BASE OURS THEIRS OUT")
    base, ours, theirs, out = sys.argv[1:]
    merged = merge(load(base), load(ours), load(theirs))
    with open(out, "w", encoding="utf-8") as fh:
        json.dump(dict(sorted(merged.items())), fh, ensure_ascii=False, indent=2)
        fh.write("\n")


if __name__ == "__main__":
    main()
