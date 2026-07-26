package engine

import "github.com/wippyai/go-lua/analysis/check/fixpoint/front"

// Local aliases keep the engine's domain-oriented helper signatures compact
// without declaring a second copy of the front-owned wire schemas.
type branchPredicateWire = front.BranchPredicateWire
type branchDiffWire = front.BranchDiffWire
