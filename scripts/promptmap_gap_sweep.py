#!/usr/bin/env python3
"""Drive promptmap-mcp over stdio to sweep domain packages for generic-engine
vocabulary gaps. Credentials are parsed from ~/.codex/config.toml at runtime.

Usage:
  promptmap_gap_sweep.py schema            # print refine/query input schemas
  promptmap_gap_sweep.py launch            # start the gap-sweep refine job
  promptmap_gap_sweep.py poll <job_id>     # poll job to completion, dump YES rows
"""
import json
import os
import re
import subprocess
import sys
import time

ROOT = "/home/wolfy-j/wippy/go-lua"
CACHE = os.path.join(ROOT, ".promptmap-cache")
OUT = os.path.join(CACHE, "gap_sweep")

VERBS = (
    "typed Factor carriers (lattice, sparse keywise Default, fingerprint, per-key AdmitAt admission); "
    "exact typed Unit reads; summary reads; ordered selector reads (SelectRead with declared candidates, "
    "non-poisoning noncurrent candidates); multi-read Query[R] product with FactorExact/FactorSummary tokens; "
    "direct exact writes; SelectWrite ordered candidate writes; RouteWrite route-preserving staged writes over "
    "authenticated dynamic routes tied to a presealed Factor target universe; Product staged-or-no-candidate "
    "totality (callback false is protocol failure); scoped edge Reindex recurrence transport (never WTO-region Mu); "
    "Point join equations X_p = Init_p JOIN C_g with localized widening on back contributions only; "
    "Composition-owned support completion and prune; activation of sealed candidate relations; "
    "ClosedRefs opaque boundary refs; epoch-local cached Group contributions; exact ChangeSet deltas plus "
    "whole-Factor Carry invalidation; demand-driven queries with FactorRegion evidence"
)

FILTER = (
    "This repository has a generic abstract-interpretation engine whose complete capability vocabulary is: "
    + VERBS + ". "
    "Does THIS file contain domain transfer, judgment, write, scheduling, or cross-domain semantics that "
    "CANNOT be expressed with that vocabulary and would require a NEW generic engine capability (a new verb), "
    "not merely a declarative use of existing verbs? Ignore legacy plumbing that the canonical migration deletes. "
    "YES or NO only."
)

DEEP = (
    "Name the missing generic engine capability as a domain-agnostic verb (like RouteWrite or Reindex), "
    "state the exact semantic shape that requires it (e.g. multi-route staged write, temporal protocol ordering, "
    "callee summary instantiation, negative/pruning write, cross-scope demand), and cite the concrete construct "
    "in this file that needs it. Do not propose code. Be terse."
)


def creds():
    text = open(os.path.expanduser("~/.codex/config.toml")).read()
    block = text.split("[mcp_servers.promptmap.env]", 1)[1]
    env = dict(os.environ)
    for key in ("PROMPTMAP_BASE", "PROMPTMAP_KEY", "PROMPTMAP_MODEL"):
        m = re.search(rf'{key}\s*=\s*"([^"]+)"', block)
        env[key] = m.group(1)
    return env


class Client:
    def __init__(self):
        self.proc = subprocess.Popen(
            ["/home/wolfy-j/.local/bin/promptmap-mcp"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            env=creds(), text=True,
        )
        self.seq = 0
        self.rpc("initialize", {
            "protocolVersion": "2024-11-05", "capabilities": {},
            "clientInfo": {"name": "gap-sweep", "version": "1.0"},
        })
        self.notify("notifications/initialized")

    def send(self, obj):
        self.proc.stdin.write(json.dumps(obj) + "\n")
        self.proc.stdin.flush()

    def notify(self, method, params=None):
        msg = {"jsonrpc": "2.0", "method": method}
        if params:
            msg["params"] = params
        self.send(msg)

    def rpc(self, method, params):
        self.seq += 1
        self.send({"jsonrpc": "2.0", "id": self.seq, "method": method, "params": params})
        while True:
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError("server closed stdout")
            msg = json.loads(line)
            if msg.get("id") == self.seq:
                if "error" in msg:
                    raise RuntimeError(json.dumps(msg["error"]))
                return msg["result"]

    def call(self, tool, args):
        result = self.rpc("tools/call", {"name": tool, "arguments": args})
        content = result.get("content", [])
        if content and content[0].get("type") == "text":
            try:
                return json.loads(content[0]["text"])
            except json.JSONDecodeError:
                return content[0]["text"]
        return result


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "schema"
    client = Client()

    if mode == "schema":
        tools = client.rpc("tools/list", {})["tools"]
        for tool in tools:
            if tool["name"] in ("refine", "query", "job_status", "job_result"):
                print("==", tool["name"], "==")
                print(json.dumps(tool.get("inputSchema", {}), indent=1)[:1200])
        return

    if mode == "launch":
        result = client.call("refine", {
            "dir": os.path.join(ROOT, "analysis/domain"),
            "ext": "go",
            "exclude": "_test",
            "filter": FILTER,
            "deep": DEEP,
            "deep_tokens": 700,
            "max_bytes": 100000,
        })
        print(json.dumps(result))
        return

    if mode == "run":
        result = client.call("refine", {
            "dir": os.path.join(ROOT, "analysis/domain"),
            "ext": "go",
            "exclude": "_test",
            "filter": FILTER,
            "deep": DEEP,
            "deep_tokens": 700,
            "max_bytes": 100000,
        })
        print(json.dumps(result), flush=True)
        job_id = result["job_id"]
        while True:
            status = client.call("job_status", {"job_id": job_id})
            print(json.dumps(status), flush=True)
            if status.get("state") in ("done", "error", "cancelled"):
                break
            time.sleep(60)
        result = client.call("job_result", {"job_id": job_id, "filter": "YES", "offset": 0, "limit": 500})
        with open(OUT + "_latest.json", "w") as fh:
            json.dump(result, fh, indent=1)
        print("matched_rows:", result.get("matched_rows"), "->", OUT + "_latest.json")
        return

    if mode == "poll":
        job_id = sys.argv[2]
        while True:
            status = client.call("job_status", {"job_id": job_id})
            print(json.dumps(status), flush=True)
            if status.get("state") in ("done", "error", "cancelled"):
                break
            time.sleep(60)
        result = client.call("job_result", {"job_id": job_id, "filter": "YES", "offset": 0, "limit": 500})
        with open(OUT + "_" + job_id + ".json", "w") as fh:
            json.dump(result, fh, indent=1)
        rows = result.get("matched_rows")
        print("matched_rows:", rows, "->", OUT + "_" + job_id + ".json")


if __name__ == "__main__":
    main()
