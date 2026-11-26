# Distribution: one static binary, prebuilt releases, self-update

Clump ships as a single static binary per platform (`CGO_ENABLED=0`), built by the release workflow
for every tag, with checksums that both `scripts/install.sh` and `clump update` verify before
replacing anything. The dependency set is closed and small on purpose — adding one is a decision,
not a detail (ADR-0008 is what happened last time one was proposed).

`clump update` writes the new binary to a temp file in the same directory and renames it over the
running executable, so the running process keeps its inode and a failed install leaves the old
binary in place.

Distribution: a release workflow (.github/workflows/release.yml) publishes, per tag `vX.Y.Z`,
`clump_<GOOS>_<GOARCH>.tar.gz` (macOS and Linux, arm64 and amd64; one binary at the archive root,
its version stamped with `-X main.version`) plus `checksums.txt`. scripts/install.sh downloads
the latest release and verifies it against those checksums; `clump update` (alias `upgrade`)
does the same through internal/update — the GitHub API anonymously, or with $GITHUB_TOKEN, else
the gh CLI's token — and renames the new binary over the running one (a temp file in the same
dir, then rename, so a running clump keeps its inode and a failed install changes nothing).

Dependencies (the whole list): `charm.land/bubbletea/v2` v2.1.2, `charm.land/bubbles/v2` v2.2.2,
`charm.land/lipgloss/v2` v2.1.1 (the current stable major of each), `github.com/alecthomas/chroma/v2`,
`github.com/BurntSushi/toml`, `golang.org/x/sys`. The Anthropic and OpenAI-compatible APIs are
spoken over raw `net/http`. `CGO_ENABLED=0`; one static binary; `go install
github.com/k22i/clump/cmd/clump@latest`.
