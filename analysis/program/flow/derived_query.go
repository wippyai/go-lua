package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Outcome is the public copy of one sealed Outcome row.
type Outcome struct {
	Body   keyspace.Term
	Kind   kind.OutcomeKind
	Target keyspace.Term
}

type Outcomes struct{ result *outcome.Result }

func (view Outcomes) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.Count()
}
func (view Outcomes) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.At(index)
}
func (view Outcomes) Get(term keyspace.Term) (Outcome, bool) {
	if view.result == nil {
		return Outcome{}, false
	}
	body, outcomeKind, target, ok := view.result.Get(term)
	return Outcome{Body: body, Kind: outcomeKind, Target: target}, ok
}
func (view Outcomes) BodyRange(body keyspace.Term) (int, int, bool) {
	if view.result == nil {
		return 0, 0, false
	}
	return view.result.BodyRange(body)
}
func (view Outcomes) BodyExit(body keyspace.Term, outcomeKind kind.OutcomeKind) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.BodyExit(body, outcomeKind)
}
func (view Outcomes) Propagation(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Propagation(term)
}
func (view Outcomes) ReturnExit(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.ReturnExit(term)
}
func (view Outcomes) BreakExit(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.BreakExit(term)
}
func (view Outcomes) GotoExit(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.GotoExit(term)
}

type Ports struct{ result *evaluation.Ports }

func (view Ports) Entry(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Entry(term)
}
func (view Ports) Finish(term keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Finish(term)
}

// termCount returns the canonical denominator bound carried by Flow's sealed
// evaluation ports. It remains private to Flow's owner-local transitive
// queries; callers cannot use it to reconstruct a term directory.
func (view Ports) termCount() uint32 {
	if view.result == nil {
		return 0
	}
	return view.result.TermCount()
}

type Pending struct{ result *evaluation.Pending }

func (view Pending) Count(subject keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Count(subject)
}
func (view Pending) At(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.At(subject, index)
}

type Executable struct {
	result *executable.Result
}

func (view Executable) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.Count()
}
func (view Executable) FamilyCount(family keyspace.Family) int {
	if view.result == nil {
		return 0
	}
	return view.result.FamilyCount(family)
}
func (view Executable) Contains(term keyspace.Term) bool {
	return view.result != nil && view.result.Executable(term)
}

// RootCount returns Flow's sealed dense executable-root denominator for one
// Body. The rows were issued during Flow assembly from the exact Source root
// order, executable membership, and semantic-path certificate; unavailable
// Bodies fail closed separately from valid empty root sets.
func (view Executable) RootCount(body keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.RootCount(body)
}

// RootAt returns one already-issued executable-root identity and authored
// family in dense Source order. It never reopens Source or rebuilds a join.
func (view Executable) RootAt(body keyspace.Term, index int) (identity.ContentID, keyspace.Family, bool) {
	if view.result == nil {
		return identity.ContentID{}, keyspace.FamilyInvalid, false
	}
	return view.result.RootAt(body, index)
}

type DirectFunctions struct{ result *directfunction.Result }

func (view DirectFunctions) For(value keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.DirectFunction(value)
}
func (view DirectFunctions) Read(read keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.ReadFunction(read)
}
func (view DirectFunctions) Call(call keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.CallFunction(call)
}
func (view DirectFunctions) GenericLoop(loop keyspace.Term) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.GenericLoopFunction(loop)
}

type Candidates struct{ result *candidates.Result }

func (view Candidates) Unary() UnaryCandidates   { return UnaryCandidates(view) }
func (view Candidates) Binary() BinaryCandidates { return BinaryCandidates(view) }
func (view Candidates) Access() AccessCandidates { return AccessCandidates(view) }

type UnaryCandidates struct{ result *candidates.Result }

func (view UnaryCandidates) NumericCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.UnaryNumeric().Count()
}
func (view UnaryCandidates) NumericAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.UnaryNumeric().At(index)
}
func (view UnaryCandidates) LengthCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Length().Count()
}
func (view UnaryCandidates) LengthAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Length().At(index)
}

type BinaryCandidates struct{ result *candidates.Result }

func (view BinaryCandidates) ArithmeticCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Arithmetic().Count()
}
func (view BinaryCandidates) ArithmeticAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Arithmetic().At(index)
}
func (view BinaryCandidates) BitwiseCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Bitwise().Count()
}
func (view BinaryCandidates) BitwiseAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Bitwise().At(index)
}
func (view BinaryCandidates) ConcatCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Concat().Count()
}
func (view BinaryCandidates) ConcatAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Concat().At(index)
}
func (view BinaryCandidates) EqualityCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Equality().Count()
}
func (view BinaryCandidates) EqualityAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Equality().At(index)
}
func (view BinaryCandidates) OrderCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Order().Count()
}
func (view BinaryCandidates) OrderAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Order().At(index)
}

type AccessCandidates struct{ result *candidates.Result }

func (view AccessCandidates) GetCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.IndexGet().Count()
}
func (view AccessCandidates) GetAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.IndexGet().At(index)
}
func (view AccessCandidates) SetCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.IndexSet().Count()
}
func (view AccessCandidates) SetAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.IndexSet().At(index)
}
