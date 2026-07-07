package body

import "github.com/wippyai/go-lua/analysis/lua/semantics"

// Semantic fact aliases define the check/body boundary for facts produced by
// lua/semantics. Consumers above body should depend on these names instead of
// importing the lower semantic extraction package directly.
type BranchKind = semantics.BranchKind
type BranchConditionFact = semantics.BranchConditionFact
type LocalAssignmentFact = semantics.LocalAssignmentFact
type LocalAssignmentFactView = semantics.LocalAssignmentFactView
type OrdinaryAssignmentFact = semantics.OrdinaryAssignmentFact
type OrdinaryAssignmentFactView = semantics.OrdinaryAssignmentFactView
type ReturnFact = semantics.ReturnFact
type ObjectLiteralFact = semantics.ObjectLiteralFact
type ObjectEntryFact = semantics.ObjectEntryFact
type CallFact = semantics.CallFact
type CallFactView = semantics.CallFactView
type SourceSpan = semantics.SourceSpan

const (
	BranchUnknown      = semantics.BranchUnknown
	BranchIf           = semantics.BranchIf
	BranchWhile        = semantics.BranchWhile
	BranchRepeat       = semantics.BranchRepeat
	BranchShortCircuit = semantics.BranchShortCircuit
)
