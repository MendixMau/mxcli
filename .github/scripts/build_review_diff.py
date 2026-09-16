#!/usr/bin/env python3
"""Build a review diff from the per-file API payload, within a byte budget.

The diff endpoint caps at 20,000 lines (HTTP 406); the files endpoint does not,
so the diff is reassembled from per-file patches. Selection is file-aware: whole
patches are taken in priority order until the budget is spent, so the model sees
complete hunks of the files that matter instead of a byte-sliced prefix of
whatever sorted first.
"""
import json, os, sys

BUDGET = int(os.environ.get("DIFF_BUDGET", "80000"))
# No single file may take more than this, or two big ones crowd out everything
# else: on PR #1107, size-ordered with no cap, three files filled the budget.
PER_FILE = max(6000, BUDGET // 10)

# Paths that are noise in a review: generated, vendored, fixtures, append-only logs.
# Matched against "/" + filename, so every pattern starts with "/" and therefore
# matches on a path-segment boundary. The repo has BOTH a root testdata/ and
# per-package ones; matching the bare word instead would also have caught
# docs/notes-testdata/, which is not fixtures.
SKIP = (
    "/mdl/grammar/parser/",
    "/.claude/skills/fix-issue/findings/",
    "/testdata/",
    "/vscode-mdl/vscode-mdl-",
)
SKIP_SUFFIX = (".bson", ".mpr", ".vsix", ".lockb", ".sum", ".png", ".jpg", ".gif", ".pdf")

def rank(f):
    n = f["filename"]
    if n.endswith((".go", ".g4")):                       return 0   # the code
    if n.startswith(".github/") or n == "Makefile":      return 1   # the build
    if n.endswith((".mdl", ".json", ".yaml", ".yml")):   return 2   # examples, config
    if n.endswith(".md"):                                return 3   # docs
    return 4

def noise(n):
    return n.endswith(SKIP_SUFFIX) or any(s in "/" + n for s in SKIP)

def main():
    files = json.load(open(sys.argv[1]))
    out, manifest = [], []
    included = skipped_noise = omitted_budget = 0
    used = 0

    for f in files:
        manifest.append("  %-6s +%-5d -%-5d %s" % (f["status"][:6], f["additions"],
                                                   f["deletions"], f["filename"]))

    for f in sorted(files, key=lambda f: (rank(f), -(f["additions"] + f["deletions"]), f["filename"])):
        n, patch = f["filename"], f.get("patch")
        if patch is None:                       # binary, or too large for a patch
            continue
        if noise(n):
            skipped_noise += 1
            continue
        clipped = ""
        if len(patch) > PER_FILE:
            patch = patch[:patch.rfind("\n", 0, PER_FILE) + 1]
            clipped = "... [patch clipped at %d bytes; %d/%d lines changed]\n" % (
                PER_FILE, f["additions"] + f["deletions"], f["changes"])
        block = "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n%s\n%s" % (n, n, n, n, patch, clipped)
        if used + len(block) > BUDGET:
            omitted_budget += 1
            continue                            # keep trying: a later file may fit
        out.append(block); used += len(block); included += 1

    with open(os.environ.get("DIFF_OUT", "/tmp/pr.diff"), "w") as fh:
        fh.write("".join(out))
    with open(os.environ.get("MANIFEST_OUT", "/tmp/pr-manifest.txt"), "w") as fh:
        fh.write("\n".join(manifest) + "\n")

    print("files=%d included=%d noise_skipped=%d over_budget=%d bytes=%d"
          % (len(files), included, skipped_noise, omitted_budget, used))
    # for the workflow's truncation note
    with open(os.environ.get("SUMMARY_OUT", "/tmp/diff-summary.txt"), "w") as fh:
        fh.write("%d %d %d %d" % (len(files), included, skipped_noise, omitted_budget))

main()
