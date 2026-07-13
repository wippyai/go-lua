package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ParamSlot describes one runtime parameter slot for a function.
type ParamSlot struct {
	Symbol       symbol.ID
	Name         string
	Position     ast.Position
	Type         ast.TypeExpr
	SourceIndex  int
	Vararg       bool
	ImplicitSelf bool
}

// Capture describes one declaration directly captured by a function body.
type Capture struct {
	Captured          symbol.ID
	CapturedName      string
	DeclaringFunction *ast.FunctionExpr
}

// FunctionOriginKind classifies the syntactic form that introduced a function.
type FunctionOriginKind uint8

const (
	FunctionOriginDeclaration FunctionOriginKind = iota + 1
	FunctionOriginLocalAssignment
	FunctionOriginLiteral
	FunctionOriginMethod
)

// FunctionOrigin records where a function expression was introduced.
type FunctionOrigin struct {
	Func   *ast.FunctionExpr
	Symbol symbol.ID
	Parent *ast.FunctionExpr
	Kind   FunctionOriginKind

	Stmt       ast.Stmt
	LocalIndex int
	Method     string

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
}

// LocalFunctionUseClosure is binder-owned evidence about every runtime use of
// one locally bound function value. DirectCalls are parser identities and are
// intentionally projected to operation sites by the solved-body exporter.
// The zero value is not a proof.
type LocalFunctionUseClosure struct {
	FunctionSymbol         symbol.ID
	TargetSymbol           symbol.ID
	DirectCalls            []*ast.FuncCallExpr
	RuntimeUseScanComplete bool
	BindingStable          bool
	ValueDoesNotEscape     bool
	CallSetComplete        bool
}

// LocalFunctionUseClosures reports conservative, whole-unit use evidence in
// lexical function order. A read counts as closed only when the binder saw it
// as the callee identifier of a direct call in the same lexical body. Captures,
// aliases, returns, stores, arguments, and all unclassified reads fail closed.
func (r *Result) LocalFunctionUseClosures() []LocalFunctionUseClosure {
	if r == nil {
		return nil
	}
	out := make([]LocalFunctionUseClosure, 0, len(r.functions))
	targetCounts := make(map[symbol.ID]int, len(r.functions))
	seenTargets := make(map[symbol.ID]struct{}, len(targetCounts))
	for _, fn := range r.functions {
		origin, ok := r.functionOrigins[fn]
		if ok && origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			targetCounts[origin.TargetSymbol]++
		}
	}
	for _, fn := range r.functions {
		origin, ok := r.functionOrigins[fn]
		if !ok || origin.Symbol == 0 || !origin.HasTargetSymbol || origin.TargetSymbol == 0 {
			continue
		}
		if _, seen := seenTargets[origin.TargetSymbol]; seen {
			continue
		}
		seenTargets[origin.TargetSymbol] = struct{}{}
		calls := append([]*ast.FuncCallExpr(nil), r.directCalls[origin.TargetSymbol]...)
		directReads := make(map[*ast.IdentExpr]struct{}, len(calls))
		for _, call := range calls {
			if call == nil {
				continue
			}
			if ident, ok := call.Func.(*ast.IdentExpr); ok && ident != nil {
				directReads[ident] = struct{}{}
			}
		}
		allReadsDirect := len(directReads) == len(r.readIdents[origin.TargetSymbol])
		if allReadsDirect {
			for _, read := range r.readIdents[origin.TargetSymbol] {
				if _, direct := directReads[read]; !direct {
					allReadsDirect = false
					break
				}
			}
		}
		stable := r.runtimeUseScanComplete && targetCounts[origin.TargetSymbol] == 1 && len(r.writeIdents[origin.TargetSymbol]) == 0 && allReadsDirect
		closed := r.runtimeUseScanComplete && allReadsDirect
		if closed {
			for _, candidate := range r.functions {
				captured := false
				for _, capture := range r.directCaptures[candidate] {
					if capture.Captured == origin.TargetSymbol {
						captured = true
						break
					}
				}
				if captured {
					closed = false
					break
				}
			}
		}
		stable = stable && closed
		out = append(out, LocalFunctionUseClosure{
			FunctionSymbol: origin.Symbol, TargetSymbol: origin.TargetSymbol,
			DirectCalls: calls, RuntimeUseScanComplete: r.runtimeUseScanComplete, BindingStable: stable,
			ValueDoesNotEscape: closed, CallSetComplete: stable && closed,
		})
	}
	return out
}

// FunctionSymbol returns the function identity symbol for fn.
func (r *Result) FunctionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if r == nil || fn == nil {
		return 0, false
	}
	id, ok := r.functionSymbols[fn]
	return id, ok && id != 0
}

// FunctionBySymbol returns the function expression identified by sym.
func (r *Result) FunctionBySymbol(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if r == nil || sym == 0 {
		return nil, false
	}
	fn, ok := r.functionsBySymbol[sym]
	return fn, ok
}

// Functions returns all bound functions in parent-before-child order.
func (r *Result) Functions() []*ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return cloneFunctions(r.functions)
}

// NestedFunctions returns the direct nested functions declared under parent.
func (r *Result) NestedFunctions(parent *ast.FunctionExpr) []*ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return cloneFunctions(r.nestedFunctions[parent])
}

// FunctionOrigins returns all bound function origins in parent-before-child order.
func (r *Result) FunctionOrigins() []FunctionOrigin {
	if r == nil {
		return nil
	}
	if len(r.functions) == 0 {
		return nil
	}
	origins := make([]FunctionOrigin, 0, len(r.functions))
	for _, fn := range r.functions {
		origin, ok := r.functionOrigins[fn]
		if !ok {
			continue
		}
		origins = append(origins, origin)
	}
	return origins
}

// ForEachFunctionOrigin visits bound function origins in parent-before-child
// order without allocating a caller-owned slice.
func (r *Result) ForEachFunctionOrigin(visit func(FunctionOrigin) bool) {
	if r == nil || visit == nil {
		return
	}
	for _, fn := range r.functions {
		origin, ok := r.functionOrigins[fn]
		if !ok {
			continue
		}
		if !visit(origin) {
			return
		}
	}
}

// FunctionOrigin returns the origin metadata for fn.
func (r *Result) FunctionOrigin(fn *ast.FunctionExpr) (FunctionOrigin, bool) {
	if r == nil || fn == nil {
		return FunctionOrigin{}, false
	}
	origin, ok := r.functionOrigins[fn]
	return origin, ok && origin.Func != nil
}

// MethodOriginReceiverSymbol returns the root value symbol that owns a colon
// method definition, such as methods in `function methods:f(...)`.
func (r *Result) MethodOriginReceiverSymbol(origin FunctionOrigin) (symbol.ID, bool) {
	if r == nil || origin.Kind != FunctionOriginMethod {
		return 0, false
	}
	stmt, ok := origin.Stmt.(*ast.FuncDefStmt)
	if !ok || stmt == nil || stmt.Name == nil || stmt.Name.Method == "" {
		return 0, false
	}
	return r.receiverRootSymbol(stmt.Name.Receiver)
}

func (r *Result) receiverRootSymbol(expr ast.Expr) (symbol.ID, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		id, ok := r.SymbolOf(e)
		return id, ok && id != 0
	case *ast.AttrGetExpr:
		return r.receiverRootSymbol(e.Object)
	default:
		return 0, false
	}
}

// ParentFunction returns the direct lexical parent of fn, if fn is known.
func (r *Result) ParentFunction(fn *ast.FunctionExpr) (*ast.FunctionExpr, bool) {
	origin, ok := r.FunctionOrigin(fn)
	if !ok {
		return nil, false
	}
	return origin.Parent, true
}

// FunctionDescendsFrom reports whether fn is a strict lexical descendant of
// ancestor. Binding records functions in lexical preorder and seals subtree
// intervals once, making this query constant-time regardless of nesting depth.
// A nil ancestor denotes the containing chunk and includes every known function.
func (r *Result) FunctionDescendsFrom(fn, ancestor *ast.FunctionExpr) bool {
	if r == nil || fn == nil || fn == ancestor {
		return false
	}
	index, known := r.functionIndex[fn]
	if !known {
		return false
	}
	if ancestor == nil {
		return true
	}
	start, known := r.functionIndex[ancestor]
	if !known {
		return false
	}
	return index > start && index < r.functionSubtreeEnd[ancestor]
}

// ForEachDescendantFunctionOrigin visits a lexical subtree in parent-before-
// child order. Work is proportional only to the returned subtree.
func (r *Result) ForEachDescendantFunctionOrigin(ancestor *ast.FunctionExpr, visit func(FunctionOrigin) bool) {
	if r == nil || visit == nil {
		return
	}
	start, end := 0, len(r.functions)
	if ancestor != nil {
		index, ok := r.functionIndex[ancestor]
		if !ok {
			return
		}
		start, end = index+1, r.functionSubtreeEnd[ancestor]
	}
	for _, fn := range r.functions[start:end] {
		if origin, ok := r.functionOrigins[fn]; ok && !visit(origin) {
			return
		}
	}
}

// DeclaringFunction returns the function that owns a declaration symbol.
func (r *Result) DeclaringFunction(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if r == nil || sym == 0 {
		return nil, false
	}
	fn, ok := r.declaringFunctions[sym]
	return fn, ok
}

// DirectCaptures returns declarations directly captured by fn in first-use order.
func (r *Result) DirectCaptures(fn *ast.FunctionExpr) []Capture {
	if r == nil || fn == nil {
		return nil
	}
	return cloneCaptures(r.directCaptures[fn])
}

// EntryCaptures returns the order-preserving closure inputs required by fn and
// its descendants, excluding declarations owned by fn itself. Lexical subtree
// intervals localize enumeration; the transitive slice is deliberately
// ephemeral so deeply nested programs do not retain quadratic capture lists.
func (r *Result) EntryCaptures(fn *ast.FunctionExpr) []Capture {
	if r == nil || fn == nil {
		return nil
	}
	start, known := r.functionIndex[fn]
	if !known || !r.hasEntryCaptures[fn] {
		return nil
	}
	out := cloneCaptures(r.directCaptures[fn])
	seen := make(map[symbol.ID]struct{}, len(out))
	for _, capture := range out {
		if capture.Captured != 0 {
			seen[capture.Captured] = struct{}{}
		}
	}
	for _, descendant := range r.functions[start+1 : r.functionSubtreeEnd[fn]] {
		for _, capture := range r.directCaptures[descendant] {
			if capture.Captured == 0 || capture.DeclaringFunction == fn {
				continue
			}
			if _, exists := seen[capture.Captured]; exists {
				continue
			}
			seen[capture.Captured] = struct{}{}
			out = append(out, capture)
		}
	}
	return out
}

// HasEntryCaptures reports in constant time whether EntryCaptures is non-empty.
// This is the solve-loop query; it stores one bit per function rather than all
// transitive capture projections.
func (r *Result) HasEntryCaptures(fn *ast.FunctionExpr) bool {
	if r == nil || fn == nil {
		return false
	}
	return r.hasEntryCaptures[fn]
}

// DirectGlobalReads returns global symbols directly read by fn in first-use
// order. Globals are not closure captures, but interprocedural analysis needs
// them as entry-state dependencies because Lua globals are mutable values.
func (r *Result) DirectGlobalReads(fn *ast.FunctionExpr) []symbol.ID {
	if r == nil || fn == nil {
		return nil
	}
	return cloneSymbols(r.directGlobalReads[fn])
}

// ChunkGlobalReads returns global symbols directly read by the lexical chunk
// (outside nested functions) in first-use order.
func (r *Result) ChunkGlobalReads() []symbol.ID {
	if r == nil {
		return nil
	}
	return cloneSymbols(r.chunkGlobalReads)
}

// ParamSymbols returns ordered parameter symbols for fn.
func (r *Result) ParamSymbols(fn *ast.FunctionExpr) []symbol.ID {
	if r == nil || fn == nil {
		return nil
	}
	return cloneSymbols(r.paramSymbols[fn])
}

// VarargSymbol returns the vararg parameter identity for fn, when present.
func (r *Result) VarargSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if r == nil || fn == nil {
		return 0, false
	}
	id, ok := r.varargSymbols[fn]
	return id, ok && id != 0
}

// ParamSlots returns the bind-owned runtime parameter layout for fn.
func (r *Result) ParamSlots(fn *ast.FunctionExpr) []ParamSlot {
	if r == nil || fn == nil {
		return nil
	}
	return cloneParamSlots(r.paramSlots[fn])
}

type functionOriginDetails struct {
	kind            FunctionOriginKind
	stmt            ast.Stmt
	localIndex      int
	method          string
	targetSymbol    symbol.ID
	hasTargetSymbol bool

	receiverType    TypeDecl
	hasReceiverType bool
}

func (r *Result) registerFunction(fn, parent *ast.FunctionExpr, details functionOriginDetails) symbol.ID {
	if fn == nil {
		return 0
	}
	if id, ok := r.functionSymbols[fn]; ok {
		return id
	}
	id := r.newSymbol("", symbol.Function)
	r.functionSymbols[fn] = id
	r.functionsBySymbol[id] = fn
	r.functions = append(r.functions, fn)
	r.nestedFunctions[parent] = append(r.nestedFunctions[parent], fn)
	r.declaringFunctions[id] = fn
	r.functionOrigins[fn] = FunctionOrigin{
		Func:            fn,
		Symbol:          id,
		Parent:          parent,
		Kind:            details.kind,
		Stmt:            details.stmt,
		LocalIndex:      details.localIndex,
		Method:          details.method,
		TargetSymbol:    details.targetSymbol,
		HasTargetSymbol: details.hasTargetSymbol,
	}
	return id
}

// finalizeFunctionIndexes seals lexical indexes after the binder has finished.
// It also records only whether an effective entry capture exists. Full
// transitive lists are enumerated ephemerally from the sealed subtree interval.
func (r *Result) finalizeFunctionIndexes() {
	if r == nil || len(r.functions) == 0 {
		return
	}
	for index, fn := range r.functions {
		r.functionIndex[fn] = index
		r.functionSubtreeEnd[fn] = index + 1
		r.hasEntryCaptures[fn] = len(r.directCaptures[fn]) != 0
	}
	for index := len(r.functions) - 1; index >= 0; index-- {
		fn := r.functions[index]
		origin := r.functionOrigins[fn]
		if origin.Parent != nil && r.functionSubtreeEnd[origin.Parent] < r.functionSubtreeEnd[fn] {
			r.functionSubtreeEnd[origin.Parent] = r.functionSubtreeEnd[fn]
		}
	}
	for _, descendant := range r.functions {
		for _, capture := range r.directCaptures[descendant] {
			for ancestor := r.functionOrigins[descendant].Parent; ancestor != nil; ancestor = r.functionOrigins[ancestor].Parent {
				if capture.Captured == 0 || capture.DeclaringFunction == ancestor {
					continue
				}
				r.hasEntryCaptures[ancestor] = true
			}
		}
	}
}

func (b *binder) currentFunction() *ast.FunctionExpr {
	if len(b.functionStack) == 0 {
		return nil
	}
	return b.functionStack[len(b.functionStack)-1]
}

func (b *binder) recordDirectCapture(id symbol.ID) {
	if id == 0 {
		return
	}
	current := b.currentFunction()
	if current == nil {
		return
	}
	kind, ok := b.result.kinds[id]
	if !ok || (kind != symbol.Local && kind != symbol.Param) {
		return
	}
	declaringFn := b.result.declaringFunctions[id]
	if declaringFn == current {
		return
	}
	seen := b.result.directCaptureSeen[current]
	if seen == nil {
		seen = make(map[symbol.ID]struct{})
		b.result.directCaptureSeen[current] = seen
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	b.result.directCaptures[current] = append(b.result.directCaptures[current], Capture{
		Captured:          id,
		CapturedName:      b.result.names[id],
		DeclaringFunction: declaringFn,
	})
}

func (b *binder) recordDirectGlobalRead(id symbol.ID) {
	if id == 0 {
		return
	}
	current := b.currentFunction()
	if current == nil {
		if _, ok := b.result.chunkGlobalSeen[id]; ok {
			return
		}
		kind, ok := b.result.kinds[id]
		if !ok || kind != symbol.Global {
			return
		}
		b.result.chunkGlobalSeen[id] = struct{}{}
		b.result.chunkGlobalReads = append(b.result.chunkGlobalReads, id)
		return
	}
	kind, ok := b.result.kinds[id]
	if !ok || kind != symbol.Global {
		return
	}
	seen := b.result.directGlobalSeen[current]
	if seen == nil {
		seen = make(map[symbol.ID]struct{})
		b.result.directGlobalSeen[current] = seen
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	b.result.directGlobalReads[current] = append(b.result.directGlobalReads[current], id)
}

func (b *binder) bindVararg(expr *ast.Comma3Expr) {
	current := b.currentFunction()
	if current == nil {
		return
	}
	id, ok := b.result.varargSymbols[current]
	if !ok || id == 0 {
		return
	}
	b.result.addOccurrence(id, Occurrence{Role: b.occurrenceRole(id, OccurrenceRead), Span: ast.SpanOf(expr)})
	b.recordDirectCapture(id)
}

func (b *binder) bindFunction(fn *ast.FunctionExpr, method bool, origin functionOriginDetails) {
	if fn == nil {
		return
	}

	parent := b.currentFunction()
	b.result.registerFunction(fn, parent, origin)
	b.functionStack = append(b.functionStack, fn)

	oldVisibleDeferred := b.visibleDeferred
	b.visibleDeferred = len(b.deferred)

	b.pushScope()
	b.bindTypeParamConstraints(fn.TypeParams)
	fnTypeParams := b.defineTypeParams(fn.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[fn] = fnTypeParams
	}

	params := make([]symbol.ID, 0)
	slots := make([]ParamSlot, 0)
	names := []string(nil)
	types := []ast.TypeExpr(nil)
	hasVargs := false
	varargType := ast.TypeExpr(nil)
	if fn.ParList != nil {
		names = fn.ParList.Names
		types = fn.ParList.Types
		hasVargs = fn.ParList.HasVargs
		varargType = fn.ParList.VarargType
	}
	if method && (len(names) == 0 || names[0] != "self") {
		id := b.newSymbol("self", symbol.Param)
		b.result.setDeclaration(id, Declaration{Synthetic: true})
		params = append(params, id)
		b.define("self", id)
		slots = append(slots, ParamSlot{
			Symbol:       id,
			Name:         "self",
			SourceIndex:  -1,
			ImplicitSelf: true,
		})
	}
	for i, name := range names {
		id := b.newSymbol(name, symbol.Param)
		position := positionAt(fn.ParList, i)
		b.result.setDeclaration(id, declarationForPosition(position, name, false))
		params = append(params, id)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        name,
			Position:    position,
			Type:        typeAt(types, i),
			SourceIndex: i,
		})
	}
	if origin.hasReceiverType {
		b.result.methodReceiverTypes[fn] = origin.receiverType
	}
	b.result.paramSymbols[fn] = params
	if hasVargs {
		id := b.newSymbol("...", symbol.Param)
		varargPosition := ast.Position{}
		if fn.ParList != nil {
			varargPosition = fn.ParList.VarargPosition
		}
		b.result.setDeclaration(id, declarationForPosition(varargPosition, "...", true))
		b.result.varargSymbols[fn] = id
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        "...",
			Position:    varargPosition,
			Type:        varargType,
			SourceIndex: len(names),
			Vararg:      true,
		})
	}
	b.result.paramSlots[fn] = slots
	b.bindTypeExprs(types)
	b.bindTypeExpr(varargType)
	b.bindTypeExprs(fn.ReturnTypes)
	b.bindStmts(fn.Stmts)
	b.popScope()

	b.visibleDeferred = oldVisibleDeferred
	b.functionStack = b.functionStack[:len(b.functionStack)-1]
}

func positionAt(parlist *ast.ParList, index int) ast.Position {
	if parlist == nil || index < 0 || index >= len(parlist.NamePositions) {
		return ast.Position{}
	}
	return parlist.NamePositions[index]
}

func namePosition(positions []ast.Position, index int) ast.Position {
	if index < 0 || index >= len(positions) {
		return ast.Position{}
	}
	return positions[index]
}

func (b *binder) bindFunctionTypeSignature(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	b.bindTypeParamConstraints(fn.TypeParams)
	b.pushTypeScope()
	b.defineTypeParams(fn.TypeParams)
	if fn.ParList != nil {
		b.bindTypeExprs(fn.ParList.Types)
		b.bindTypeExpr(fn.ParList.VarargType)
	}
	b.bindTypeExprs(fn.ReturnTypes)
	b.popTypeScope()
}

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
