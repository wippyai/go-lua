// Package visibilityfacts adapts Lua WIR structure into generic visibility tables.
package visibilityfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DefinitionsFromWIR extracts the path definitions needed by point-local
// visibility directly from WIR structure. It is intentionally structural:
// lowering records paths, assignment origins, and select result targets; this
// adapter only turns those identities into visibility definitions.
func DefinitionsFromWIR(bindings *bind.Result, graph cfg.Graph, body *wir.Body) []visibility.Definition {
	if graph == nil || body == nil {
		return nil
	}
	assigned := make(map[symbol.ID]struct{})
	seen := make(map[definitionKey]struct{})
	var defs []visibility.Definition

	add := func(point cfg.Point, sym symbol.ID) {
		if sym == 0 {
			return
		}
		key := definitionKey{point: point, sym: sym}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		defs = append(defs, visibility.Definition{
			Point:  point,
			Symbol: sym,
			Root:   bindings.Name(sym),
		})
	}

	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Assign != wir.AssignNone {
				p := wirOperandPath(body, inst.Dst)
				if p.Symbol != 0 && len(p.Segments) == 0 {
					assigned[p.Symbol] = struct{}{}
					add(point, p.Symbol)
				}
			}
			switch inst.Op {
			case wir.OpStaticMemberWrite:
				if global, ok := wirGlobalTableFieldRootSymbol(bindings, body, inst.Dst); ok {
					assigned[global] = struct{}{}
					add(point, global)
				}
			case wir.OpIterate:
				for _, result := range body.Operands(inst.Results) {
					p := wirOperandPath(body, result)
					if p.Symbol != 0 && len(p.Segments) == 0 {
						assigned[p.Symbol] = struct{}{}
						add(point, p.Symbol)
					}
				}
			case wir.OpSelect:
				for _, target := range body.CallResultTargets(point) {
					if target.Path.Symbol != 0 {
						add(point, target.Path.Symbol)
					}
				}
				for _, op := range body.Operands(inst.List) {
					p := wirOperandPath(body, op)
					if p.Symbol != 0 {
						add(point, p.Symbol)
					}
				}
			}
		}
	}

	needed := pathSymbolsFromWIR(graph, body)
	for sym := range needed {
		if _, ok := assigned[sym]; !ok || shouldSeedAtEntry(bindings, sym) {
			add(graph.Entry(), sym)
		}
	}
	return defs
}

type definitionKey struct {
	point cfg.Point
	sym   symbol.ID
}

func shouldSeedAtEntry(bindings *bind.Result, sym symbol.ID) bool {
	kind, ok := bindings.Kind(sym)
	if !ok {
		return true
	}
	return kind == symbol.Param || kind == symbol.Global || kind == symbol.Upvalue
}

func pathSymbolsFromWIR(graph cfg.Graph, body *wir.Body) map[symbol.ID]struct{} {
	needed := make(map[symbol.ID]struct{})
	addProofPath := func(p pathdom.Path) {
		if p.Symbol != 0 {
			needed[p.Symbol] = struct{}{}
		}
	}
	addPath := func(p pathdom.Path) {
		if p.Symbol != 0 && len(p.Segments) != 0 {
			needed[p.Symbol] = struct{}{}
		}
	}
	addOperandPath := func(op wir.Operand) {
		addPath(wirOperandPath(body, op))
	}
	addProofOperand := func(op wir.Operand) {
		addProofPath(wirOperandPath(body, op))
	}
	addCheck := func(check wir.Check) {
		addProofPath(check.Path)
		addProofPath(check.OtherPath)
	}

	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			addOperandPath(inst.Dst)
			addOperandPath(inst.A)
			addOperandPath(inst.B)
			addOperandPath(inst.Call.Callee)
			addProofOperand(inst.Call.Receiver)
			for _, target := range body.CallResultTargets(point) {
				addPath(target.Path)
			}
			for _, op := range body.Operands(inst.Results) {
				addOperandPath(op)
			}
			for _, op := range body.Operands(inst.List) {
				if inst.Op == wir.OpCall || inst.Op == wir.OpSelect {
					addProofOperand(op)
				} else {
					addOperandPath(op)
				}
			}
			if inst.Op == wir.OpDynamicIndexRead {
				addProofOperand(inst.A)
				addProofOperand(inst.B)
			}
			if inst.Op == wir.OpUnOp && inst.Operator == wir.UnLen {
				addProofOperand(inst.A)
			}
			if inst.Check != 0 {
				addCheck(body.Check(inst.Check))
			}
			for _, implied := range body.ImpliedChecks(inst.ImpliedChecks) {
				addCheck(implied.Check)
			}
			for _, sufficient := range body.SufficientChecks(inst.SufficientChecks) {
				addCheck(sufficient.Check)
			}
		}
	}
	return needed
}

func wirOperandPath(body *wir.Body, op wir.Operand) pathdom.Path {
	if body == nil || op.Kind != wir.OperandPath {
		return pathdom.Path{}
	}
	p := body.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return pathdom.Path{}
	}
	return p
}

func wirGlobalTableFieldRootSymbol(bindings *bind.Result, body *wir.Body, op wir.Operand) (symbol.ID, bool) {
	if bindings == nil {
		return 0, false
	}
	p := wirOperandPath(body, op)
	if p.Symbol == 0 || bindings.Name(p.Symbol) != "_G" {
		return 0, false
	}
	kind, ok := bindings.Kind(p.Symbol)
	if !ok || kind != symbol.Global {
		return 0, false
	}
	name, ok := p.DirectFieldName()
	if !ok {
		return 0, false
	}
	global, ok := bindings.GlobalSymbol(name)
	return global, ok && global != 0
}
