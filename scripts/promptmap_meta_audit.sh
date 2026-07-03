#!/usr/bin/env bash
set -euo pipefail

ROOT="${ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
PROMPTMAP="${PROMPTMAP:-/home/wolfy-j/wippy/promptmap/promptmap}"
MATRIX="${1:-$ROOT/analysis/architecture/promptmap_meta_audit.csv}"
SELECTOR="${2:-all}"
OUTDIR="${OUTDIR:-$ROOT/.promptmap-meta}"

mkdir -p "$OUTDIR"

python3 - "$ROOT" "$PROMPTMAP" "$MATRIX" "$SELECTOR" "$OUTDIR" <<'PY'
import csv
import os
import shlex
import subprocess
import sys
from pathlib import Path

root, promptmap, matrix, selector, outdir = sys.argv[1:6]
root = str(Path(root).resolve())
outdir_path = Path(outdir)
outdir_path.mkdir(parents=True, exist_ok=True)

model = os.environ.get("MODEL", "gemma-4-26b")
leaf_model = os.environ.get("LEAF_MODEL", model)
reduce_model = os.environ.get("REDUCE_MODEL", model)
conc = os.environ.get("CONC", "12")
deep_tokens = os.environ.get("DEEP_TOKENS", "900")
max_tokens = os.environ.get("MAX_TOKENS", "1200")
max_bytes = os.environ.get("MAX_BYTES", "120000")
steps = os.environ.get("STEPS", "8")
dry_run = os.environ.get("DRY_RUN", "") != ""

def selected(row):
    if selector == "all":
        return True
    wanted = {part.strip() for part in selector.split(",") if part.strip()}
    return row["id"] in wanted or row["mode"] in wanted

def common_policy(row):
    return (
        "You are a skeptical architecture auditor. promptmap is only a noisy index; "
        "do not invent abstractions. Require concrete symbols, local evidence, and "
        "a clear owner boundary. Treat this CSV row as the audit spec.\n\n"
        f"RULE: {row['id']} - {row['title']}\n"
        f"EXPECTED OWNER: {row['expected_owner']}\n"
        f"ALLOWED SURFACES: {row['allowed_surfaces']}\n"
        f"BANNED SURFACES: {row['banned_surfaces']}\n"
        f"DETERMINISTIC PROBE: {row['deterministic_probe']}\n\n"
    )

def run(cmd, err_path):
    print("+", " ".join(shlex.quote(x) for x in cmd), flush=True)
    if dry_run:
        return
    with open(err_path, "w", encoding="utf-8") as err:
        subprocess.run(cmd, cwd=root, stderr=err, check=True)

with open(matrix, newline="", encoding="utf-8") as f:
    rows = [r for r in csv.DictReader(f) if selected(r)]

if not rows:
    raise SystemExit(f"no matching rows for selector {selector!r}")

for row in rows:
    row_id = row["id"]
    scope = str(Path(root, row["scope_dir"]))
    out_csv = str(outdir_path / f"{row_id}.csv")
    err_log = str(outdir_path / f"{row_id}.err")
    mode = row["mode"]

    cmd = [
        promptmap,
        "-mode", mode,
        "-dir", scope,
        "-ext", row["ext"] or "go",
        "-c", conc,
        "-model", model,
        "-leaf-model", leaf_model,
        "-reduce-model", reduce_model,
        "-max-bytes", max_bytes,
        "-out", out_csv,
    ]
    if row["match"]:
        cmd += ["-match", row["match"]]
    if row["exclude"]:
        cmd += ["-exclude", row["exclude"]]

    policy = common_policy(row)
    if mode == "refine":
        cmd += [
            "-deep-tokens", deep_tokens,
            "-filter", policy + row["filter_prompt"],
            "-deep", policy + row["deep_prompt"],
        ]
    elif mode == "agentscan":
        qfile = outdir_path / f"{row_id}.queries"
        qfile.write_text(f"# {row_id}\n{policy}{row['agent_prompt']}\n", encoding="utf-8")
        cmd += ["-queries", str(qfile), "-steps", steps, "-max-tokens", max_tokens]
    else:
        raise SystemExit(f"unsupported mode {mode!r} in row {row_id}")

    run(cmd, err_log)
PY
