#!/usr/bin/env python3
"""Prepare country-boundary polygons for the UN/LOCODE map.

Raw inputs are read from the local raw/ directory.
The UN/LOCODE release only carries point coordinates, not country shapes,
so the polygon geometry is sourced from another dataset.
That dataset keys features by ISO-3166 alpha-3, while the server uses
alpha-2 country codes, so we rewrite each feature's `id` to alpha-2 using an
ISO 3166 mapping.
"""

import csv
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
RAW = os.path.join(HERE, "raw")
OUT = os.path.join(HERE, "generated", "countries.geo.json")

GEO_PATH = os.path.join(RAW, "countries.geo.json")
ISO_PATH = os.path.join(RAW, "all.csv")


def read_text(path: str) -> str:
    with open(path, encoding="utf-8") as f:
        return f.read()


def main():
    geo = json.loads(read_text(GEO_PATH))

    a3toa2 = {}
    for row in csv.DictReader(read_text(ISO_PATH).splitlines()):
        a3 = row.get("alpha-3", "").strip().upper()
        a2 = row.get("alpha-2", "").strip().upper()
        if a3 and a2:
            a3toa2[a3] = a2

    matched = 0
    for f in geo.get("features", []):
        a3 = str(f.get("id", "")).strip().upper()
        a2 = a3toa2.get(a3)
        if a2:
            f["id"] = a2
            matched += 1

    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(geo, fh, ensure_ascii=False, separators=(",", ":"))

    print(f"wrote {OUT}: {len(geo.get('features', []))} features, {matched} re-keyed to alpha-2")


if __name__ == "__main__":
    main()
