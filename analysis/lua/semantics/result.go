package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

type Result struct {
	function *ast.FunctionExpr

	calls map[cfg.Point]*CallFact
}

func newResult(fn *ast.FunctionExpr) *Result {
	return &Result{
		function: fn,
		calls:    make(map[cfg.Point]*CallFact),
	}
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return r.function
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

func (r *Result) setCall(point cfg.Point, fact CallFact) {
	if r == nil || point == 0 {
		return
	}
	r.calls[point] = &fact
}
