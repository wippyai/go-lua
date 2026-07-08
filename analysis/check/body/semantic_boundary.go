package body

import (
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
)

// Semantic fact aliases define the check/body boundary for facts produced by
// lua/semantics. Consumers above body should depend on these names instead of
// importing the lower semantic extraction package directly.
type LocalAssignmentFact = cfgbuild.LocalAssignment
type OrdinaryAssignmentFact = cfgbuild.OrdinaryAssignment
type ReturnFact = cfgbuild.Return
type CallFact = cfgbuild.Call
type CallFactView = cfgbuild.CallView
type SourceSpan = cfgbuild.SourceSpan
