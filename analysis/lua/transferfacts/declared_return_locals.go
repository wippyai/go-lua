package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerDeclaredReturnLocalTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypes(bindings, graph, result, resolver, plainReturnIdentSymbol)
}

func lowerReturnLocalObjectLiteralTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypes(bindings, graph, result, resolver, returnObjectLiteralLocalSymbol)
}

func lowerDeclaredReturnLocalTypesFromWIR(bindings *bind.Result, graph cfg.Graph, body *wir.Body) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypesFromWIR(bindings, graph, body)
}

func lowerReturnLocalObjectLiteralTypesFromWIR(bindings *bind.Result, graph cfg.Graph, body *wir.Body) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypesFromWIR(bindings, graph, body)
}

type returnLocalSymbolResolver func(sourceprovenance.ASTSource, *bind.Result) (symbol.ID, bool)

func lowerReturnLocalTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver, resolveSymbol returnLocalSymbolResolver) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil || result == nil {
		return nil
	}
	if resolveSymbol == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	declared := resolvedDeclaredReturnContracts(result, resolver)
	if len(declared) == 0 {
		return nil
	}
	candidates := returnedTableLocalCandidates(graph, result)
	if len(candidates) == 0 {
		return nil
	}

	states := make([]returnSlotLocalState, len(declared))
	for i := range declared {
		if declared[i] == nil {
			states[i].invalid = true
		}
	}
	for _, point := range graph.RPO() {
		view, ok := result.ReturnView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok {
			continue
		}
		for slot := range states {
			if states[slot].invalid {
				continue
			}
			source, ok := returnSourceForSlot(fact, slot)
			if !ok {
				states[slot].invalid = true
				continue
			}
			id, ok := resolveSymbol(source, bindings)
			if !ok {
				if returnSourceMayOmitDeclaredLocal(source, declared[slot]) {
					continue
				}
				states[slot].invalid = true
				continue
			}
			if _, ok := candidates[id]; !ok {
				states[slot].invalid = true
				continue
			}
			if !states[slot].seen {
				states[slot].symbol = id
				states[slot].seen = true
				continue
			}
			if states[slot].symbol != id {
				states[slot].invalid = true
			}
		}
	}

	out := make(map[symbol.ID]typ.Type)
	for slot, state := range states {
		if state.invalid || !state.seen || state.symbol == 0 {
			continue
		}
		contract := returnedLocalDeclaredContract(declared[slot])
		if existing, ok := out[state.symbol]; ok && !typ.TypeEquals(existing, contract) {
			delete(out, state.symbol)
			continue
		}
		out[state.symbol] = contract
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lowerReturnLocalTypesFromWIR(bindings *bind.Result, graph cfg.Graph, body *wir.Body) map[symbol.ID]typ.Type {
	if graph == nil || body == nil {
		return nil
	}
	declared := body.DeclaredReturnTypes()
	if len(declared) == 0 {
		return nil
	}
	for i, t := range declared {
		if !declaredReturnLocalContractType(t) {
			declared[i] = nil
		}
	}
	tempDefs := wirSingleTempDefinitions(graph, body)
	candidates := returnedTableLocalCandidatesFromWIR(graph, body, tempDefs)
	if len(candidates) == 0 {
		return nil
	}
	states := make([]returnSlotLocalState, len(declared))
	for i := range declared {
		if declared[i] == nil {
			states[i].invalid = true
		}
	}
	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op != wir.OpReturn {
				continue
			}
			for slot := range states {
				if states[slot].invalid {
					continue
				}
				op, ok := wirReturnOperandForSlot(body, inst, slot)
				if !ok {
					states[slot].invalid = true
					continue
				}
				id, ok := wirReturnLocalSymbolFromOperand(bindings, body, tempDefs, op)
				if !ok {
					if wirReturnOperandMayOmitDeclaredLocal(body, op, declared[slot]) {
						continue
					}
					states[slot].invalid = true
					continue
				}
				if _, ok := candidates[id]; !ok {
					states[slot].invalid = true
					continue
				}
				if !states[slot].seen {
					states[slot].symbol = id
					states[slot].seen = true
					continue
				}
				if states[slot].symbol != id {
					states[slot].invalid = true
				}
			}
		}
	}
	out := make(map[symbol.ID]typ.Type)
	for slot, state := range states {
		if state.invalid || !state.seen || state.symbol == 0 {
			continue
		}
		contract := returnedLocalDeclaredContract(declared[slot])
		if existing, ok := out[state.symbol]; ok && !typ.TypeEquals(existing, contract) {
			delete(out, state.symbol)
			continue
		}
		out[state.symbol] = contract
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func returnedTableLocalCandidatesFromWIR(graph cfg.Graph, body *wir.Body, tempDefs map[uint32]wir.Instruction) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Assign != wir.AssignLocalDeclaration {
				continue
			}
			id, ok := wirRootPathSymbol(body, inst.Dst)
			if !ok {
				continue
			}
			if !wirAssignmentInitializesTable(body, tempDefs, inst) {
				continue
			}
			out[id] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func wirSingleTempDefinitions(graph cfg.Graph, body *wir.Body) map[uint32]wir.Instruction {
	out := make(map[uint32]wir.Instruction)
	ambiguous := make(map[uint32]struct{})
	if graph == nil || body == nil {
		return nil
	}
	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			recordWIRTempDefinition(out, ambiguous, inst, inst.Dst)
			for _, result := range body.Operands(inst.Results) {
				recordWIRTempDefinition(out, ambiguous, inst, result)
			}
		}
	}
	for temp := range ambiguous {
		delete(out, temp)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func recordWIRTempDefinition(out map[uint32]wir.Instruction, ambiguous map[uint32]struct{}, inst wir.Instruction, op wir.Operand) {
	if op.Kind != wir.OperandTemp {
		return
	}
	temp := op.Ref
	if _, seen := out[temp]; seen {
		ambiguous[temp] = struct{}{}
		return
	}
	out[temp] = inst
}

func wirAssignmentInitializesTable(body *wir.Body, tempDefs map[uint32]wir.Instruction, inst wir.Instruction) bool {
	if inst.Op == wir.OpMakeTable {
		return true
	}
	op, ok := inst.AssignmentSourceOperand()
	if !ok || op.Kind != wir.OperandTemp {
		return false
	}
	def, ok := tempDefs[op.Ref]
	return ok && def.Op == wir.OpMakeTable
}

func wirReturnOperandForSlot(body *wir.Body, inst wir.Instruction, slot int) (wir.Operand, bool) {
	if body == nil || slot < 0 {
		return wir.Operand{}, false
	}
	ops := body.Operands(inst.List)
	if slot >= len(ops) {
		return wir.Operand{}, false
	}
	if inst.ListSpread && slot == len(ops)-1 {
		return wir.Operand{}, false
	}
	return ops[slot], true
}

func wirRootPathSymbol(body *wir.Body, op wir.Operand) (symbol.ID, bool) {
	if body == nil || op.Kind != wir.OperandPath {
		return 0, false
	}
	p := body.Path(wir.PathRef(op.Ref))
	if p.Symbol == 0 || len(p.Segments) != 0 {
		return 0, false
	}
	return p.Symbol, true
}

func wirReturnLocalSymbolFromOperand(bindings *bind.Result, body *wir.Body, tempDefs map[uint32]wir.Instruction, op wir.Operand) (symbol.ID, bool) {
	if id, ok := wirRootPathSymbol(body, op); ok {
		return id, true
	}
	if op.Kind != wir.OperandTemp {
		return 0, false
	}
	def, ok := tempDefs[op.Ref]
	if !ok || def.Op != wir.OpCall || !wirCallCalleeIsGlobal(bindings, body, def, "setmetatable") {
		return 0, false
	}
	args := body.Operands(def.List)
	if len(args) == 0 {
		return 0, false
	}
	return wirRootPathSymbol(body, args[0])
}

func wirCallCalleeIsGlobal(bindings *bind.Result, body *wir.Body, inst wir.Instruction, name string) bool {
	if bindings == nil || body == nil || inst.Call.Method != 0 || inst.Call.Callee.Kind != wir.OperandPath || name == "" {
		return false
	}
	p := body.Path(wir.PathRef(inst.Call.Callee.Ref))
	if p.Symbol == 0 {
		return false
	}
	return bindings.SymbolResolvesToGlobal(p.Symbol, name)
}

func wirReturnOperandMayOmitDeclaredLocal(body *wir.Body, op wir.Operand, declared typ.Type) bool {
	if body == nil || declared == nil || !subtype.IsSubtype(typ.Nil, declared) {
		return false
	}
	if op.Kind != wir.OperandConst {
		return false
	}
	return body.Const(wir.ConstRef(op.Ref)).Kind == wir.ConstNil
}

func returnedLocalDeclaredContract(declared typ.Type) typ.Type {
	if inner := unwrap.Optional(declared); inner != nil {
		return inner
	}
	return declared
}

func returnSourceMayOmitDeclaredLocal(source sourceprovenance.ASTSource, declared typ.Type) bool {
	if declared == nil || !subtype.IsSubtype(typ.Nil, declared) {
		return false
	}
	if source.Kind == sourceprovenance.SourceNil {
		return true
	}
	if source.Kind != sourceprovenance.SourceExpression {
		return false
	}
	inner, ok := sourceprovenance.ProofInner(source.Expr)
	if !ok {
		return false
	}
	_, isNil := inner.(*ast.NilExpr)
	return isNil
}

type returnSlotLocalState struct {
	seen    bool
	invalid bool
	symbol  symbol.ID
}

func resolvedDeclaredReturnContracts(result *semantics.Result, resolver *typeresolve.Resolver) []typ.Type {
	decls := declaredReturnTypes(result)
	if len(decls) == 0 || resolver == nil {
		return nil
	}
	out := make([]typ.Type, len(decls))
	for i, decl := range decls {
		t, ok := resolver.Type(decl)
		if !ok || !declaredReturnLocalContractType(t) {
			continue
		}
		out[i] = t
	}
	return out
}

func declaredReturnLocalContractType(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return luatypeprojection.ReachesTableContract(t)
}

func returnedTableLocalCandidates(graph cfg.Graph, result *semantics.Result) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	for _, point := range graph.RPO() {
		view, ok := result.LocalAssignmentView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || !returnLocalInitializerCandidate(fact) {
			continue
		}
		out[fact.Symbol] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func returnLocalInitializerCandidate(fact semantics.LocalAssignmentFact) bool {
	return fact.HasSymbol &&
		fact.Symbol != 0 &&
		fact.Type == nil &&
		fact.Source.Kind == sourceprovenance.SourceExpression &&
		tableConstructorExpr(fact.Expr)
}

func returnSourceForSlot(fact semantics.ReturnFact, slot int) (sourceprovenance.ASTSource, bool) {
	for _, source := range fact.Sources {
		if source.TargetIndex == slot {
			return source, true
		}
	}
	return sourceprovenance.ASTSource{}, false
}

func plainReturnIdentSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if bindings == nil ||
		source.Kind != sourceprovenance.SourceExpression ||
		source.Expanded ||
		source.Adjusted ||
		source.OpenTail {
		return 0, false
	}
	inner, ok := sourceprovenance.ProofInner(source.Expr)
	if !ok {
		return 0, false
	}
	ident, ok := inner.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, false
	}
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

func returnObjectLiteralLocalSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if id, ok := plainReturnIdentSymbol(source, bindings); ok {
		return id, true
	}
	return setmetatableReturnLocalSymbol(source, bindings)
}

func setmetatableReturnLocalSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if bindings == nil ||
		source.Kind != sourceprovenance.SourceCall {
		return 0, false
	}
	inner, ok := sourceprovenance.ProofInner(source.Expr)
	if !ok {
		return 0, false
	}
	call, ok := inner.(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) == 0 || call.Func == nil {
		return 0, false
	}
	if !callCalleeIsGlobal(bindings, call.Func, "setmetatable") {
		return 0, false
	}
	arg, ok := sourceprovenance.ProofInner(call.Args[0])
	if !ok {
		return 0, false
	}
	ident, ok := arg.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, false
	}
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

func callCalleeIsGlobal(bindings *bind.Result, expr ast.Expr, name string) bool {
	if bindings == nil || expr == nil || name == "" {
		return false
	}
	ident, ok := sourceprovenance.ProofIdent(expr)
	if !ok || ident == nil {
		return false
	}
	sym, ok := bindings.SymbolOf(ident)
	return ok && bindings.SymbolResolvesToGlobal(sym, name)
}

func mergeSymbolTypes(base, extra map[symbol.ID]typ.Type) map[symbol.ID]typ.Type {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[symbol.ID]typ.Type, len(extra))
	}
	for id, t := range extra {
		if _, present := base[id]; present {
			continue
		}
		base[id] = t
	}
	return base
}
