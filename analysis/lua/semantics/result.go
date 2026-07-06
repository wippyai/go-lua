package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

type Result struct {
	function *ast.FunctionExpr

	localDeclarationFacts map[cfg.Point]*LocalAssignmentFact
	assignmentFacts       map[cfg.Point]*OrdinaryAssignmentFact
	calls                 map[cfg.Point]*CallFact
	returns               map[cfg.Point]*ReturnFact
	objectLiterals        map[ast.Expr]ObjectLiteralFact
	branches              map[cfg.Point]BranchConditionFact
}

func newResult(fn *ast.FunctionExpr) *Result {
	return &Result{
		function:              fn,
		localDeclarationFacts: make(map[cfg.Point]*LocalAssignmentFact),
		assignmentFacts:       make(map[cfg.Point]*OrdinaryAssignmentFact),
		calls:                 make(map[cfg.Point]*CallFact),
		returns:               make(map[cfg.Point]*ReturnFact),
		objectLiterals:        make(map[ast.Expr]ObjectLiteralFact),
		branches:              make(map[cfg.Point]BranchConditionFact),
	}
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return r.function
}

func (r *Result) LocalAssignment(point cfg.Point) (LocalAssignmentFact, bool) {
	if r == nil {
		return LocalAssignmentFact{}, false
	}
	fact, ok := r.localDeclarationFacts[point]
	if !ok || fact == nil {
		return LocalAssignmentFact{}, false
	}
	return copyLocalAssignmentFact(*fact), true
}

// LocalAssignmentFactView is a read-only borrowed view of a local-assignment
// fact owned by Result. Borrowed values share slice backing storage with the
// Result and must not be mutated or retained after the immediate operation.
type LocalAssignmentFactView struct {
	fact *LocalAssignmentFact
}

// Borrowed returns the local-assignment fact without defensive copies. Callers
// that need ownership must use Result.LocalAssignment.
func (v LocalAssignmentFactView) Borrowed() (LocalAssignmentFact, bool) {
	if v.fact == nil {
		return LocalAssignmentFact{}, false
	}
	return *v.fact, true
}

// LocalAssignmentView returns a borrowed read-only local-assignment fact view at point.
func (r *Result) LocalAssignmentView(point cfg.Point) (LocalAssignmentFactView, bool) {
	if r == nil {
		return LocalAssignmentFactView{}, false
	}
	fact, ok := r.localDeclarationFacts[point]
	if !ok || fact == nil {
		return LocalAssignmentFactView{}, false
	}
	return LocalAssignmentFactView{fact: fact}, true
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (OrdinaryAssignmentFact, bool) {
	if r == nil {
		return OrdinaryAssignmentFact{}, false
	}
	fact, ok := r.assignmentFacts[point]
	if !ok || fact == nil {
		return OrdinaryAssignmentFact{}, false
	}
	return copyOrdinaryAssignmentFact(*fact), true
}

// OrdinaryAssignmentFactView is a read-only borrowed view of an ordinary
// assignment fact owned by Result.
type OrdinaryAssignmentFactView struct {
	fact *OrdinaryAssignmentFact
}

// Borrowed returns the ordinary assignment fact without defensive copies.
// Callers that need ownership must use Result.OrdinaryAssignment.
func (v OrdinaryAssignmentFactView) Borrowed() (OrdinaryAssignmentFact, bool) {
	if v.fact == nil {
		return OrdinaryAssignmentFact{}, false
	}
	return *v.fact, true
}

// OrdinaryAssignmentView returns a borrowed read-only ordinary assignment fact view at point.
func (r *Result) OrdinaryAssignmentView(point cfg.Point) (OrdinaryAssignmentFactView, bool) {
	if r == nil {
		return OrdinaryAssignmentFactView{}, false
	}
	fact, ok := r.assignmentFacts[point]
	if !ok || fact == nil {
		return OrdinaryAssignmentFactView{}, false
	}
	return OrdinaryAssignmentFactView{fact: fact}, true
}

func (r *Result) Call(point cfg.Point) (CallFact, bool) {
	if r == nil {
		return CallFact{}, false
	}
	fact, ok := r.calls[point]
	if !ok || fact == nil {
		return CallFact{}, false
	}
	return copyCallFact(*fact), true
}

// CallFactView is a read-only borrowed view of a call fact owned by Result.
// Borrowed values share slice and path backing storage with the Result and must
// not be mutated or retained after the immediate operation.
type CallFactView struct {
	fact *CallFact
}

// Borrowed returns the call fact without defensive copies. The returned fact is
// for immediate read-only use; callers that need ownership must use Result.Call.
func (v CallFactView) Borrowed() (CallFact, bool) {
	if v.fact == nil {
		return CallFact{}, false
	}
	return *v.fact, true
}

// CallView returns a borrowed read-only call fact view at point.
func (r *Result) CallView(point cfg.Point) (CallFactView, bool) {
	if r == nil {
		return CallFactView{}, false
	}
	fact, ok := r.calls[point]
	if !ok || fact == nil {
		return CallFactView{}, false
	}
	return CallFactView{fact: fact}, true
}

func (r *Result) Return(point cfg.Point) (ReturnFact, bool) {
	if r == nil {
		return ReturnFact{}, false
	}
	fact, ok := r.returns[point]
	if !ok || fact == nil {
		return ReturnFact{}, false
	}
	return copyReturnFact(*fact), true
}

// ReturnFactView is a read-only borrowed view of a return fact owned by Result.
type ReturnFactView struct {
	fact *ReturnFact
}

// Borrowed returns the return fact without defensive copies. Callers that need
// ownership must use Result.Return.
func (v ReturnFactView) Borrowed() (ReturnFact, bool) {
	if v.fact == nil {
		return ReturnFact{}, false
	}
	return *v.fact, true
}

// ReturnView returns a borrowed read-only return fact view at point.
func (r *Result) ReturnView(point cfg.Point) (ReturnFactView, bool) {
	if r == nil {
		return ReturnFactView{}, false
	}
	fact, ok := r.returns[point]
	if !ok || fact == nil {
		return ReturnFactView{}, false
	}
	return ReturnFactView{fact: fact}, true
}

func (r *Result) ObjectLiteral(expr ast.Expr) (ObjectLiteralFact, bool) {
	if r == nil || expr == nil {
		return ObjectLiteralFact{}, false
	}
	fact, ok := r.objectLiterals[expr]
	if !ok {
		return ObjectLiteralFact{}, false
	}
	return copyObjectLiteralFact(fact), true
}

func (r *Result) setCall(point cfg.Point, fact CallFact) {
	if r == nil || point == 0 {
		return
	}
	r.calls[point] = &fact
}

func (r *Result) setLocalAssignment(point cfg.Point, fact LocalAssignmentFact) {
	if r == nil || point == 0 {
		return
	}
	r.localDeclarationFacts[point] = &fact
}

func (r *Result) setOrdinaryAssignment(point cfg.Point, fact OrdinaryAssignmentFact) {
	if r == nil || point == 0 {
		return
	}
	r.assignmentFacts[point] = &fact
}

func (r *Result) setReturn(point cfg.Point, fact ReturnFact) {
	if r == nil || point == 0 {
		return
	}
	r.returns[point] = &fact
}

func (r *Result) BranchCondition(point cfg.Point) (BranchConditionFact, bool) {
	if r == nil {
		return BranchConditionFact{}, false
	}
	fact, ok := r.branches[point]
	if !ok {
		return BranchConditionFact{}, false
	}
	return copyBranchConditionFact(fact), true
}
