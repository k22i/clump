#!/bin/sh
# mkfixture.sh DIR [N]: build a scratch git repo for tests and the e2e run.
# It has: plain commits, a rename, a copy, a binary file, a mode change, a
# file without a trailing newline, a quoted unicode path, a path with spaces,
# a deleted file, a merge commit, a feature branch, a stash entry, and N
# (default 40) filler commits so lazygit has something to scroll through.
set -eu
dir=$1
n=${2:-40}
rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q -b main
git config user.name "Fixture Bot"
git config user.email "fixture@example.invalid"
git config commit.gpgsign false
export GIT_AUTHOR_DATE="2026-01-01T00:00:00Z" GIT_COMMITTER_DATE="2026-01-01T00:00:00Z"
c() { git add -A && git commit -q -m "$1" ${2:+-m "$2"}; }

mkdir -p src docs
printf 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hello")\n}\n' > src/main.go
printf '# Fixture\n\nA scratch repo.\n' > README.md
printf 'old content\n' > docs/old.txt
c "Initial commit" "Seed the fixture repo."

printf 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hello, world")\n}\n\nfunc add(a, b int) int { return a + b }\n' > src/main.go
printf '# Fixture\n\nA scratch repo for clump tests.\n' > README.md
c "Mixed: greet the world, add helper, readme tweak"

git mv docs/old.txt docs/new.txt
printf 'appended\n' >> docs/new.txt
c "Rename old.txt to new.txt and append"

printf '\211PNG\r\n\032\n\000\000\000\rIHDR' > logo.bin
printf 'no newline at end' > nonl.txt
chmod +x src/main.go
c "Binary, mode change, no-newline file"

printf 'caf\303\251\n' > "caf$(printf '\303\251').txt"
mkdir -p "dir a"
printf 'spaced\n' > "dir a/f b.txt"
c "Unicode and spaced paths"

cp src/main.go src/main_copy.go
printf '\n// copy\n' >> src/main_copy.go
c "Copy main.go"

git rm -q nonl.txt
c "Delete nonl.txt"

git checkout -q -b feature
printf 'package main\n\nfunc feature() string { return "f" }\n' > src/feature.go
c "Add feature"
printf 'package main\n\nfunc feature() string { return "feature" }\n\nfunc feature2() int { return 2 }\n' > src/feature.go
c "Extend feature"
git checkout -q main
printf '# Fixture\n\nA scratch repo for clump tests.\n\nSee docs/new.txt.\n' > README.md
c "Point README at docs"
git merge -q --no-ff --no-edit feature -m "Merge branch 'feature'"

i=1
while [ "$i" -le "$n" ]; do
	printf 'line %d\n' "$i" >> docs/log.txt
	if [ $((i % 3)) -eq 0 ]; then printf 'func f%d() int { return %d }\n' "$i" "$i" >> src/fill.go; fi
	c "Filler commit $i"
	i=$((i + 1))
done

git checkout -q -b feature2
printf 'package main\n\n// Feature two.\nfunc two() int { return 2 }\n' > src/two.go
c "Start feature two"
git checkout -q main

printf 'stashed change\n' >> README.md
git stash push -q -m "fixture stash"
echo "fixture repo at $dir: $(git rev-list --count HEAD) commits on main"
