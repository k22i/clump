#!/bin/sh
# Re-run the spike B matrix: 3 diffs x 3 providers on a public repo (lazygit).
# usage: run.sh <lazygit-clone>
repo="$1"; out="$(cd "$(dirname "$0")" && pwd)"; G="$out/../bin/groupcli"
for spec in 7ffcbb47 c7c514fa '54cc6af1^1...54cc6af1^2'; do
  for p in claude:haiku claude:sonnet opencode:opencode/deepseek-v4.1-flash-free; do
    name=$(printf '%s__%s' "$spec" "$p" | tr '^./:' '-_--')
    echo "== $spec $p"
    "$G" --repo "$repo" --provider "$p" --stats "$out/stats.jsonl" "$spec" > "$out/$name.md" 2> "$out/$name.log"
    tail -1 "$out/$name.log"
  done
done
