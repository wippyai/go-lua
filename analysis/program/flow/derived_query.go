package flow

import (
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/directbinding"
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

type Executable struct{ result *executable.Result }

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

func (view Candidates) Unary() UnaryCandidates   { return UnaryCandidates{result: view.result} }
func (view Candidates) Binary() BinaryCandidates { return BinaryCandidates{result: view.result} }
func (view Candidates) Access() AccessCandidates { return AccessCandidates{result: view.result} }
func (view Candidates) Loops() LoopCandidates    { return LoopCandidates{result: view.result} }

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

type LoopCandidates struct{ result *candidates.Result }

func (view LoopCandidates) GenericCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.GenericLoop().Count()
}
func (view LoopCandidates) GenericAt(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.GenericLoop().At(index)
}

type DirectBindings struct{ result *directbinding.Result }

func (view DirectBindings) Selection(read keyspace.Term) (keyspace.Term, int, bool) {
	if view.result == nil {
		return 0, 0, false
	}
	return view.result.BindingSelections().Get(read)
}

// SelectionPath opens one immutable leaf-to-root cursor over a sealed exact
// binding path. Each Segment advances one existing parent-chain edge in O(1)
// without materializing or restarting the path.
func (view DirectBindings) SelectionPath(read keyspace.Term) (BindingPath, bool) {
	if view.result == nil {
		return BindingPath{}, false
	}
	path, ok := view.result.BindingSelections().PathCursor(read)
	return BindingPath{path: path}, ok
}

type BindingPath struct{ path directbinding.BindingPath }

func (path BindingPath) Segment() (keyspace.Key, BindingPath, bool) {
	key, next, ok := path.path.Segment()
	if !ok {
		return 0, BindingPath{}, false
	}
	return key, BindingPath{path: next}, true
}

func (view DirectBindings) Publication(publication keyspace.Term) (root, owner keyspace.Term, depth int, ok bool) {
	if view.result == nil {
		return 0, 0, 0, false
	}
	return view.result.PublicationPaths().Get(publication)
}

// PublicationPath opens one immutable leaf-to-root cursor over the sealed
// exact Static publication path. Segment is O(1) and allocation-free.
func (view DirectBindings) PublicationPath(publication keyspace.Term) (PublicationPath, bool) {
	if view.result == nil {
		return PublicationPath{}, false
	}
	path, ok := view.result.PublicationPaths().PathCursor(publication)
	return PublicationPath{path: path}, ok
}

type PublicationPath struct{ path directbinding.PublicationPath }

func (path PublicationPath) Segment() (keyspace.Key, PublicationPath, bool) {
	key, next, ok := path.path.Segment()
	if !ok {
		return 0, PublicationPath{}, false
	}
	return key, PublicationPath{path: next}, true
}
func (view DirectBindings) Call(call keyspace.Term) (keyspace.Term, CallForm, bool) {
	if view.result == nil {
		return 0, 0, false
	}
	read, form, ok := view.result.DirectCalls().Get(call)
	if !ok {
		return 0, 0, false
	}
	return read, publicCallForm(form), true
}

// CallForm is the closed direct-call syntax disposition.
type CallForm uint8

const (
	CallFormPlain  CallForm = 1
	CallFormMethod CallForm = 2
)

func publicCallForm(form directbinding.CallForm) CallForm {
	switch form {
	case directbinding.CallFormPlain:
		return CallFormPlain
	case directbinding.CallFormMethod:
		return CallFormMethod
	default:
		return 0
	}
}
