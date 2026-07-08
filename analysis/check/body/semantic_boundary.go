package body

import (
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
)

// Source fact aliases define the check/body boundary for facts produced by
// cfgbuild. Consumers above body should depend on these names instead of
// importing the lower CFG construction package directly.
type LocalAssignmentFact = cfgbuild.LocalAssignment
type OrdinaryAssignmentFact = cfgbuild.OrdinaryAssignment
type ReturnFact = cfgbuild.Return
type CallFact = cfgbuild.Call
type SourceSpan = cfgbuild.SourceSpan
