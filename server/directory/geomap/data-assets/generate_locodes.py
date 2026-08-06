#!/usr/bin/env python3
"""Parse code-list-improved.csv into a LOCODE -> [lat, lon] JSON map.

The output artifact:

    locodes.json  ->  {"ADALV": [42.5, 1.5167], ...}

Key  = uppercase "<country><location>" (e.g. "AD" + "ALV" -> "ADALV")
Value = [latitude, longitude], both in decimal degrees, sourced from the
CoordinatesDecimal column (format: "lat,lon"). Rows without valid decimal
coordinates fall back to parsing the Coordinates column (DDMM N DDDMM E).
Rows with no usable coordinates are skipped.
"""

import csv
import json
import os
import re

HERE = os.path.dirname(os.path.abspath(__file__))
RAW = os.path.join(HERE, "raw")
CSV_PATH = os.path.join(RAW, "code-list-improved.csv")
LOCODES_PATH = os.path.join(HERE, "generated", "locodes.json")

COORD_RE = re.compile(r"^(\d{2})(\d{2})([NS])\s+(\d{3})(\d{2})([EW])$")


def parse_dms_coord(raw: str):
    if not raw:
        return None
    m = COORD_RE.match(raw.strip())
    if not m:
        return None
    lat = int(m.group(1)) + int(m.group(2)) / 60.0
    lon = int(m.group(4)) + int(m.group(5)) / 60.0
    if m.group(3) == "S":
        lat = -lat
    if m.group(6) == "W":
        lon = -lon
    return [round(lat, 4), round(lon, 4)]


def parse_decimal_coord(raw: str):
    if not raw:
        return None
    parts = raw.strip().split(",")
    if len(parts) != 2:
        return None
    try:
        lat = float(parts[0])
        lon = float(parts[1])
    except ValueError:
        return None
    return [round(lat, 4), round(lon, 4)]


def main():
    if not os.path.exists(CSV_PATH):
        print(f"CSV not found: {CSV_PATH}", file=os.sys.stderr)
        return 1

    locodes: dict[str, list] = {}
    skipped = 0

    with open(CSV_PATH, newline="", encoding="utf-8") as f:
        reader = csv.reader(f)
        header = next(reader)
        for row in reader:
            if len(row) < 13:
                continue
            country = row[1].strip().upper()
            place = row[2].strip().upper()
            if not country or not place:
                continue

            coord = parse_decimal_coord(row[12])
            if coord is None:
                coord = parse_dms_coord(row[10])
            if coord is None:
                if row[10].strip() or row[12].strip():
                    skipped += 1
                continue

            locodes[country + place] = coord

    with open(LOCODES_PATH, "w", encoding="utf-8") as f:
        json.dump(locodes, f, ensure_ascii=False, separators=(",", ":"))

    print(f"wrote {len(locodes)} locodes -> {LOCODES_PATH}")
    if skipped:
        print(f"skipped {skipped} row(s) with unparseable coordinates")


if __name__ == "__main__":
    main()
