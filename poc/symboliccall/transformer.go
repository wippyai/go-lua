package symboliccall

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// Definition is one symbolic equation before direct calls are composed.
type Definition struct {
	ID       FunctionID
	Params   int
	Returns  []Expr
	Uses     []state.LaneID
	Fallback string
}

// Transformer is a solved, call-free function value transformer. Contextual
// means the experiment must use the complete production analysis instead.
type Transformer struct {
	params     int
	returns    []Expr
	valid      bool
	contextual bool
	reason     string
}

// Contextual reports whether all-or-nothing fallback is required.
func (t Transformer) Contextual() bool { return t.contextual }

// Reason reports the deterministic fallback reason.
func (t Transformer) Reason() string { return t.reason }

// Instantiate applies a solved transformer to concrete production values.
func (t Transformer) Instantiate(reg *axis.Registry, params []product.Value) ([]product.Value, error) {
	if !t.valid || t.contextual {
		return nil, fmt.Errorf("symboliccall: contextual transformer: %s", t.reason)
	}
	if len(params) != t.params {
		return nil, fmt.Errorf("symboliccall: got %d parameters, want %d", len(params), t.params)
	}
	out := make([]product.Value, len(t.returns))
	for i, expr := range t.returns {
		var err error
		out[i], err = eval(reg, expr, params)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func contextual(params int, reason string) Transformer {
	return Transformer{params: params, valid: true, contextual: true, reason: reason}
}

func transformerEqual(a, b Transformer) bool {
	if a.valid != b.valid || a.contextual != b.contextual {
		return false
	}
	if !a.valid {
		return true
	}
	return a.params == b.params && a.reason == b.reason && exprSliceEqual(a.returns, b.returns)
}

func joinTransformer(a, b Transformer) Transformer {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}
	if a.contextual {
		return a
	}
	if b.contextual {
		return b
	}
	if a.params != b.params || len(a.returns) != len(b.returns) {
		return contextual(max(a.params, b.params), "incompatible transformer shape")
	}
	out := Transformer{params: a.params, valid: true, returns: make([]Expr, len(a.returns))}
	for i := range out.returns {
		out.returns[i] = Join(a.returns[i], b.returns[i])
	}
	return out
}

func resolveDefinition(def Definition, read func(FunctionID) Transformer) Transformer {
	if def.Fallback != "" {
		return contextual(def.Params, def.Fallback)
	}
	for _, lane := range def.Uses {
		if lane != state.LaneValues {
			return contextual(def.Params, "unsupported state lane: "+string(lane))
		}
	}
	out := Transformer{params: def.Params, valid: true, returns: make([]Expr, len(def.Returns))}
	for i, expr := range def.Returns {
		resolved, reason := resolveCalls(expr, read)
		if reason != "" {
			return contextual(def.Params, reason)
		}
		out.returns[i] = resolved
	}
	return out
}

func resolveCalls(expr Expr, read func(FunctionID) Transformer) (Expr, string) {
	if expr.n == nil {
		return Expr{}, ""
	}
	switch expr.n.op {
	case opBottom, opParam, opConst:
		return expr, ""
	case opJoin:
		args := make([]Expr, len(expr.n.args))
		for i, arg := range expr.n.args {
			var reason string
			args[i], reason = resolveCalls(arg, read)
			if reason != "" {
				return Expr{}, reason
			}
		}
		return Join(args...), ""
	case opCall:
		callee := read(expr.n.callee)
		if !callee.valid {
			// Bottom is the least solution during SCC ascent.
			return Expr{}, ""
		}
		if callee.contextual {
			return Expr{}, "callee requires contextual analysis: " + string(expr.n.callee)
		}
		if expr.n.slot >= len(callee.returns) || len(expr.n.args) != callee.params {
			return Expr{}, "call shape mismatch: " + string(expr.n.callee)
		}
		args := make([]Expr, len(expr.n.args))
		for i, arg := range expr.n.args {
			var reason string
			args[i], reason = resolveCalls(arg, read)
			if reason != "" {
				return Expr{}, reason
			}
		}
		got, ok := substitute(callee.returns[expr.n.slot], args)
		if !ok {
			return Expr{}, "callee result is not closed: " + string(expr.n.callee)
		}
		return got, ""
	default:
		return Expr{}, "invalid expression"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
