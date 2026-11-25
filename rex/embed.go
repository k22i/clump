// Package rexlua embeds clump's Rex Lua module (init.lua) so
// `clump rex install` can write it out from a `go install`ed binary.
package rexlua

import _ "embed"

// Module is the contents of rex/init.lua.
//
//go:embed init.lua
var Module []byte
