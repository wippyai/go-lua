package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// ParamSlot describes one parameter slot for a function.
type ParamSlot struct {
	Symbol       Symbol
	Name         string
	Position     ast.Position
	Type         ast.TypeExpr
	SourceIndex  int
	Vararg       bool
	ImplicitSelf bool
}

// Capture describes one declaration directly captured by a function body.
type Capture struct {
	Captured          Symbol
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
	Parent *ast.FunctionExpr
	Kind   FunctionOriginKind
	// Static classifies a function whose ordinary body occurs exclusively
	// beneath typeof/annotation syntax. It shares the same binding and capture
	// relations but never contributes runtime-use evidence.
	Static bool

	Stmt       ast.Stmt
	LocalIndex int
	Method     string
}

// FunctionOrigin returns the origin metadata for fn.
func (r *Result) FunctionOrigin(fn *ast.FunctionExpr) (FunctionOrigin, bool) {
	if r == nil || fn == nil {
		return FunctionOrigin{}, false
	}
	origin, ok := r.functionOrigins[fn]
	return origin, ok && origin.Func != nil
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
	var seen map[*ast.FunctionExpr]map[Symbol]struct{}
	mark := func(fn *ast.FunctionExpr, capture Capture) bool {
		if fn == nil || capture.Captured == 0 {
			return false
		}
		if seen == nil {
			seen = make(map[*ast.FunctionExpr]map[Symbol]struct{})
		}
		symbols := seen[fn]
		if symbols == nil {
			symbols = make(map[Symbol]struct{})
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
				origin, known := r.functionOrigins[current]
				if !known {
					break
				}
				parent := origin.Parent
				parentOrigin, parentKnown := r.functionOrigins[parent]
				if !parentKnown || origin.Static != parentOrigin.Static {
					break
				}
				current = parent
			}
		}
	}
}

// VarargSymbol returns the vararg parameter identity for fn, when present.
func (r *Result) VarargSymbol(fn *ast.FunctionExpr) (Symbol, bool) {
	if r == nil || fn == nil {
		return 0, false
	}
	id, ok := r.varargSymbols[fn]
	return id, ok && id != 0
}

// ParamSlots returns the bind-owned parameter layout for fn.
func (r *Result) ParamSlots(fn *ast.FunctionExpr) []ParamSlot {
	if r == nil || fn == nil {
		return nil
	}
	return cloneParamSlots(r.paramSlots[fn])
}

func cloneParamSlots(slots []ParamSlot) []ParamSlot {
	if len(slots) == 0 {
		return nil
	}
	return append([]ParamSlot(nil), slots...)
}

type functionOriginDetails struct {
	kind       FunctionOriginKind
	static     bool
	stmt       ast.Stmt
	localIndex int
	method     string

	receiverType    TypeDecl
	hasReceiverType bool
}

func (r *Result) registerFunction(fn, parent *ast.FunctionExpr, details functionOriginDetails) {
	if fn == nil {
		return
	}
	if _, ok := r.functionOrigins[fn]; ok {
		return
	}
	r.functions = append(r.functions, fn)
	r.functionOrigins[fn] = FunctionOrigin{
		Func:       fn,
		Parent:     parent,
		Kind:       details.kind,
		Static:     details.static,
		Stmt:       details.stmt,
		LocalIndex: details.localIndex,
		Method:     details.method,
	}
}

func (b *binder) currentFunction() *ast.FunctionExpr {
	if len(b.functions) == 0 {
		return nil
	}
	return b.functions[len(b.functions)-1].fn
}

func (b *binder) currentFunctionStatic() bool {
	fn := b.currentFunction()
	if fn == nil {
		return false
	}
	origin, ok := b.result.functionOrigins[fn]
	return ok && origin.Static
}

func (b *binder) recordDirectCapture(id Symbol) {
	if id == 0 {
		return
	}
	current := b.currentFunction()
	if current == nil {
		return
	}
	kind, ok := b.result.kinds[id]
	if !ok || (kind != SymbolLocal && kind != SymbolParam) {
		return
	}
	declaringFn := b.declaringFunctions[id]
	if declaringFn == current {
		return
	}
	seen := b.directCaptureSeen[current]
	if seen == nil {
		seen = make(map[Symbol]struct{})
		if b.directCaptureSeen == nil {
			b.directCaptureSeen = make(map[*ast.FunctionExpr]map[Symbol]struct{})
		}
		b.directCaptureSeen[current] = seen
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

func (b *binder) bindVararg(expr *ast.Comma3Expr) {
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

func positionAt(parlist *ast.ParList, index int) ast.Position {
	if parlist == nil || index < 0 || index >= len(parlist.NamePositions) {
		return ast.Position{}
	}
	return parlist.NamePositions[index]
}

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
