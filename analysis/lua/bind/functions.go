package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ParamSlot describes one runtime parameter slot for a function.
type ParamSlot struct {
	Symbol       symbol.ID
	Name         string
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
	FunctionOriginUnknown FunctionOriginKind = iota
	FunctionOriginDeclaration
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

// FunctionOrigin returns the origin metadata for fn.
func (r *Result) FunctionOrigin(fn *ast.FunctionExpr) (FunctionOrigin, bool) {
	if r == nil || fn == nil {
		return FunctionOrigin{}, false
	}
	origin, ok := r.functionOrigins[fn]
	return origin, ok && origin.Func != nil
}

// ParentFunction returns the direct lexical parent of fn, if fn is known.
func (r *Result) ParentFunction(fn *ast.FunctionExpr) (*ast.FunctionExpr, bool) {
	origin, ok := r.FunctionOrigin(fn)
	if !ok {
		return nil, false
	}
	return origin.Parent, true
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

func (b *binder) bindVararg() {
	current := b.currentFunction()
	if current == nil {
		return
	}
	id, ok := b.result.varargSymbols[current]
	if !ok || id == 0 {
		return
	}
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
		params = append(params, id)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        name,
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
		b.result.varargSymbols[fn] = id
		slots = append(slots, ParamSlot{
			Symbol:      id,
			Name:        "...",
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
