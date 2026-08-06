#!/usr/bin/env python3
"""Download raw upstream assets into the raw/ directory."""

import os
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
RAW_DIR = os.path.join(HERE, "raw")
os.makedirs(RAW_DIR, exist_ok=True)

ASSETS = {
    "code-list-improved.csv":
        "https://github.com/cristan/improved-un-locodes/raw/refs/heads/main/data/code-list-improved.csv",
    "countries.geo.json":
        "https://raw.githubusercontent.com/johan/world.geo.json/master/countries.geo.json",
    "all.csv":
        "https://raw.githubusercontent.com/lukes/ISO-3166-Countries-with-Regional-Codes/master/all/all.csv",
}


def download(name: str, url: str) -> None:
    dest = os.path.join(RAW_DIR, name)
    if os.path.exists(dest):
        print(f"skip {name}: already exists")
        return
    print(f"downloading {url} -> {dest}")
    with urllib.request.urlopen(url, timeout=30) as r, open(dest, "wb") as f:
        f.write(r.read())


def main() -> None:
    for name, url in ASSETS.items():
        download(name, url)
    print("completed")


if __name__ == "__main__":
    main()
