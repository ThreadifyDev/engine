#!/usr/bin/env python3
"""Fail release packaging when CI has not supplied a complete static dashboard."""
from pathlib import Path
import re

root = Path(__file__).resolve().parents[1] / "internal/dashboard/dist"
index = root / "index.html"
if not index.is_file():
    raise SystemExit("Missing dashboard: build web/ and copy build/client/ to internal/dashboard/dist/ before release.")
html = index.read_text()
assets = re.findall(r'(?:src|href)="(/assets/[^"?#]+)', html)
if not any(asset.endswith(".js") for asset in assets):
    raise SystemExit("Dashboard has no JavaScript entry point")
for asset in assets:
    if not (root / asset.lstrip("/")).is_file():
        raise SystemExit(f"Missing dashboard asset: {asset}")
print("Static dashboard verified")
