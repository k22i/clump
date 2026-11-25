// Package engine runs the grouping pipeline shared by one-shot mode, watch
// mode and the --json/--plain/--dry-run outputs: cache lookup, Build,
// estimate, provider calls (tier 1 or tier 2), Validate, cache write and
// calibration. Owned by the coordinator; see DESIGN.md "Engine".
package engine
