package body

import "github.com/wippyai/go-lua/analysis/lua/cfgbuild"

// Source fact aliases define the check/body boundary for facts still produced
// by cfgbuild. Consumers above body should depend on these names instead of
// importing the lower CFG construction package directly.
type SourceSpan = cfgbuild.SourceSpan
