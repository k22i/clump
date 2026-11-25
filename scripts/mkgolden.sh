#!/bin/sh
# mkgolden.sh FIXTURE_REPO: regenerate internal/gitdiff/testdata/git from a
# repo built by mkfixture.sh. Each commit is captured twice: as clump's
# loader runs git (*.show.txt) and as lazygit 0.66 runs it with
# colorArg: always (*.lazygit.txt). Both must parse to the same Diff.Hash.
set -eu
FX=$1
T=$(dirname "$0")/../internal/gitdiff/testdata/git
mkdir -p "$T"
G() { git -C "$FX" -c core.quotepath=off -c diff.noprefix=false -c diff.mnemonicPrefix=false -c diff.srcPrefix=a/ -c diff.dstPrefix=b/ -c color.ui=never "$@"; }
LG() { git -C "$FX" -c diff.dstPrefix=b/ -c diff.srcPrefix=a/ -c diff.noprefix=false "$@"; }
sha() { git -C "$FX" log --format=%H -F --grep="$1" main | head -1; }
printf 'name\tsource\tdescription\n' > "$T/INDEX.tsv"
for pair in "mixed:Mixed: greet the world, add helper, readme tweak" "rename:Rename old.txt to new.txt and append" \
	"binary-mode:Binary, mode change, no-newline file" "unicode-spaces:Unicode and spaced paths" \
	"delete:Delete nonl.txt" "merge:Merge branch 'feature'"; do
	n=${pair%%:*}; subj=${pair#*:}; s=$(sha "$subj")
	G show --format= --no-color --no-ext-diff --unified=3 --find-renames --submodule --diff-merges=first-parent "$s" > "$T/$n.show.txt"
	LG show --no-ext-diff --unified=3 --find-renames=50% --submodule --color=always --stat --decorate -p "$s" -- > "$T/$n.lazygit.txt"
	printf '%s\t%s\t%s\n' "$n" "$s" "$subj" >> "$T/INDEX.tsv"
done
s=$(sha "Copy main.go")
G show --format= --no-color --no-ext-diff --unified=3 --find-renames --find-copies-harder --submodule "$s" > "$T/copy.show.txt"
printf 'copy\t%s\tCopy main.go, shown with --find-copies-harder\n' "$s" >> "$T/INDEX.tsv"
G diff --no-color --no-ext-diff --unified=3 --find-renames --submodule main...feature2 > "$T/range-main-feature2.show.txt"
printf 'range-main-feature2\tmain...feature2\tgit diff main...feature2\n' >> "$T/INDEX.tsv"
LG show --no-ext-diff --unified=3 --submodule --color=always -c --stat --decorate -p "$(sha "Merge branch 'feature'")" -- > "$T/merge-cc.lazygit.txt" || true
printf 'merge-cc\t(merge)\tgit show -c of the merge: combined diff (may be empty for a clean merge)\n' >> "$T/INDEX.tsv"
