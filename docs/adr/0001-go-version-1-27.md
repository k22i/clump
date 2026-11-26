# Go 1.27 in `go.mod`

The task brief said Go 1.25; the user asked for 1.27 during Phase 0, and the current stable
charm v2 releases require Go ≥ 1.26. `go.mod` therefore pins 1.27.

The cost of reversing this is a downgrade of every charm dependency, so it is recorded instead
of being re-argued. `go-version-file` in `release.yml` reads the same pin.
