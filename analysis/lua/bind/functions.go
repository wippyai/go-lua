package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ParamSlot describes one parameter slot for a function. Source-only function
// literals reached through static type queries have slots but no runtime body.
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
	// DirectCallSetComplete permits a stable function binding to cross a
	// lexical closure boundary when every read remains the callee of a direct
	// call. Self-recursive capture remains excluded; recursive relation
	// ownership is established separately by the interprocedural solver.
	DirectCallSetComplete bool
	BindingStable         bool
	ValueDoesNotEscape    bool
	CallSetComplete       bool
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
		stableDirect := r.runtimeUseScanComplete && targetCounts[origin.TargetSymbol] == 1 && len(r.writeIdents[origin.TargetSymbol]) == 0 && allReadsDirect
		selfRecursive := false
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
					if candidate == fn {
						selfRecursive = true
					}
					closed = false
				}
			}
		}
		stable := stableDirect && closed
		out = append(out, LocalFunctionUseClosure{
			FunctionSymbol: origin.Symbol, TargetSymbol: origin.TargetSymbol,
			DirectCalls: calls, RuntimeUseScanComplete: r.runtimeUseScanComplete, BindingStable: stable,
			DirectCallSetComplete: stableDirect && !selfRecursive,
			ValueDoesNotEscape:    closed, CallSetComplete: stable && closed,
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

// StableLocalFunctionIdentity returns the binder function identity introduced
// for targetBinding when that non-global binding has exactly one function
// origin and has not subsequently been assigned. Ambiguous, mutable, method,
// global, and unknown targets fail closed.
//
// The lookup is constant-time. Consumers must not reconstruct this relation by
// scanning FunctionOrigins: the private index is sealed as functions register.
func (r *Result) StableLocalFunctionIdentity(targetBinding symbol.ID) (symbol.ID, bool) {
	if r == nil || targetBinding == 0 || len(r.writeIdents[targetBinding]) != 0 {
		return 0, false
	}
	kind, known := r.kinds[targetBinding]
	if !known || kind != symbol.Local {
		return 0, false
	}
	functionIdentity, indexed := r.functionTargetIndex[targetBinding]
	return functionIdentity, indexed && functionIdentity != 0
}

// StableDirectCallFunctionIdentity returns the stable lexical function bound
// to targetBinding only when every runtime read is the callee of a direct call
// and the function is not self-recursive through that binding. It is the
// constant-scope authority for consumers that may erase the function value
// itself while retaining its direct lexical call edge.
func (r *Result) StableDirectCallFunctionIdentity(targetBinding symbol.ID) (symbol.ID, bool) {
	identity, stable := r.StableLocalFunctionIdentity(targetBinding)
	if !stable || !r.runtimeUseScanComplete {
		return 0, false
	}
	calls := r.directCalls[targetBinding]
	directReads := make(map[*ast.IdentExpr]struct{}, len(calls))
	for _, call := range calls {
		if call == nil {
			continue
		}
		if ident, ok := call.Func.(*ast.IdentExpr); ok && ident != nil {
			directReads[ident] = struct{}{}
		}
	}
	reads := r.readIdents[targetBinding]
	if len(directReads) != len(reads) {
		return 0, false
	}
	for _, read := range reads {
		if _, direct := directReads[read]; !direct {
			return 0, false
		}
	}
	fn, found := r.functionsBySymbol[identity]
	if !found {
		return 0, false
	}
	for _, capture := range r.directCaptures[fn] {
		if capture.Captured == targetBinding {
			return 0, false
		}
	}
	return identity, true
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
	for {
		switch e := expr.(type) {
		case *ast.IdentExpr:
			id, ok := r.SymbolOf(e)
			return id, ok && id != 0
		case *ast.AttrGetExpr:
			expr = e.Object
		default:
			return 0, false
		}
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

// ForEachEntryCapture streams each function's effective closure-entry inputs
// in lexical first-use and boundary order. A capture is emitted once for every
// lexical function boundary it crosses, stopping at its declaring function.
// The temporary dedup state is bounded by emitted boundary edges; Result does
// not retain a transitive capture projection.
func (r *Result) ForEachEntryCapture(visit func(*ast.FunctionExpr, Capture) bool) {
	if r == nil || visit == nil {
		return
	}
	var seen map[*ast.FunctionExpr]map[symbol.ID]struct{}
	mark := func(fn *ast.FunctionExpr, capture Capture) bool {
		if fn == nil || capture.Captured == 0 {
			return false
		}
		if seen == nil {
			seen = make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
		}
		symbols := seen[fn]
		if symbols == nil {
			symbols = make(map[symbol.ID]struct{})
			seen[fn] = symbols
		}
		if _, exists := symbols[capture.Captured]; exists {
			return false
		}
		symbols[capture.Captured] = struct{}{}
		return true
	}
	for _, fn := range r.functions {
		for _, capture := range r.directCaptures[fn] {
			for current := fn; current != nil && current != capture.DeclaringFunction; {
				if !mark(current, capture) {
					break
				}
				if !visit(current, capture) {
					return
				}
				parent, known := r.ParentFunction(current)
				if !known {
					break
				}
				current = parent
			}
		}
	}
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

// ParamSlots returns the bind-owned parameter layout for fn. Source-only
// function literals reached through static type queries have slots but no
// FunctionOrigin or runtime body evidence.
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
	if details.kind != FunctionOriginMethod && details.hasTargetSymbol && details.targetSymbol != 0 {
		if _, indexed := r.functionTargetIndex[details.targetSymbol]; indexed {
			// Zero is a permanent ambiguity marker: later registrations cannot
			// make a multiply-origin binding unique again.
			r.functionTargetIndex[details.targetSymbol] = 0
		} else {
			r.functionTargetIndex[details.targetSymbol] = id
		}
	}
	return id
}

// finalizeFunctionIndexes seals lexical indexes after the binder has finished.
func (r *Result) finalizeFunctionIndexes() {
	if r == nil || len(r.functions) == 0 {
		return
	}
	for index, fn := range r.functions {
		r.functionIndex[fn] = index
		r.functionSubtreeEnd[fn] = index + 1
	}
	for index := len(r.functions) - 1; index >= 0; index-- {
		fn := r.functions[index]
		origin := r.functionOrigins[fn]
		if origin.Parent != nil && r.functionSubtreeEnd[origin.Parent] < r.functionSubtreeEnd[fn] {
			r.functionSubtreeEnd[origin.Parent] = r.functionSubtreeEnd[fn]
		}
	}
}

func (b *binder) currentFunction() *ast.FunctionExpr {
	if len(b.functions) == 0 {
		return nil
	}
	return b.functions[len(b.functions)-1].fn
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

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
