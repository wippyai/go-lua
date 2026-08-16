package typ

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Param represents a function parameter with name, type, and optionality.
type Param struct {
	Name     string
	Type     Type
	Optional bool // True if parameter has a default value
	// Receiver records the calling-convention meaning historically inferred
	// from Name == "self". Name remains unchanged for presentation/encoding.
	Receiver bool
}

// Function represents a function type with parameters and returns.
//
// Functions support generics via TypeParams, variadic arguments via Variadic,
// and multiple return values via Returns.
type Function struct {
	TypeParams        []*TypeParam // Generic type parameters (empty for non-generic)
	Params            []Param      // Positional parameters
	Variadic          Type         // Variadic element type (nil if not variadic)
	Returns           []Type       // Return types (empty for void functions)
	hash              uint64
	equalityHashCache *equalityHashCache
	typeProperties
	strCache stringCache
	semantic atomic.Pointer[Function]
}

// FunctionPresentation is immutable source-facing metadata for a function.
// It deliberately has no setters; callers may read labels without consulting
// semantic parameter identity.
type FunctionPresentation struct {
	owner *Function
}

// ParamName returns the source label for one positional parameter.
func (p FunctionPresentation) ParamName(index int) (string, bool) {
	if p.owner == nil || index < 0 || index >= len(p.owner.Params) {
		return "", false
	}
	return p.owner.Params[index].Name, true
}

// ParamNames returns an ownership-isolated label list.
func (p FunctionPresentation) ParamNames() []string {
	if p.owner == nil || len(p.owner.Params) == 0 {
		return nil
	}
	out := make([]string, len(p.owner.Params))
	for i := range p.owner.Params {
		out[i] = p.owner.Params[i].Name
	}
	return out
}

func (f *Function) Presentation() FunctionPresentation {
	return FunctionPresentation{owner: f}
}

// SemanticType returns the concrete, label-free function node constructed with
// f. It is suitable for semantic interning: ordinary parameter names are empty,
// explicit receiver positions retain the canonical "self" marker, and all
// parameter/result/type-parameter child nodes are shared with f.
//
// This PR2a view canonicalizes this function boundary only. Composite child
// types can still contain presentation-bearing function nodes; a future paired
// semantic graph at every immutable composite constructor is required before
// arbitrary types can enter typewitness without a graph walk.
func (f *Function) SemanticType() *Function {
	if f == nil {
		return nil
	}
	if semantic := f.semantic.Load(); semantic != nil {
		return semantic
	}
	candidate := newSemanticFunction(f)
	if f.semantic.CompareAndSwap(nil, candidate) {
		return candidate
	}
	return f.semantic.Load()
}

// FunctionBuilder provides a fluent API for constructing function types.
//
// Example:
//
//	fn := typ.Func().
//	    Param("x", typ.Number).
//	    Param("y", typ.Number).
//	    Returns(typ.Number).
//	    Build()
type FunctionBuilder struct {
	typeParams []*TypeParam
	params     []Param
	variadic   Type
	returns    []Type
}

// Func starts building a function type.
func Func() *FunctionBuilder {
	return &FunctionBuilder{}
}

// ReserveParams avoids reallocating while appending known effective parameters.
func (b *FunctionBuilder) ReserveParams(n int) *FunctionBuilder {
	if b == nil || n <= 1 || cap(b.params) >= n {
		return b
	}
	params := make([]Param, len(b.params), n)
	copy(params, b.params)
	b.params = params
	return b
}

// TypeParam adds a type parameter for generic functions.
func (b *FunctionBuilder) TypeParam(name string, constraint Type) *FunctionBuilder {
	b.typeParams = append(b.typeParams, NewTypeParam(name, constraint))
	return b
}

// TypeParamRef adds an already-created type parameter binder. Use this when the
// function's binder and the scope entries used to resolve its annotations must
// be the same node.
func (b *FunctionBuilder) TypeParamRef(param *TypeParam) *FunctionBuilder {
	if param == nil {
		return b
	}
	b.typeParams = append(b.typeParams, param)
	return b
}

// Param adds a required parameter.
func (b *FunctionBuilder) Param(name string, t Type) *FunctionBuilder {
	b.params = append(b.params, Param{Name: name, Type: t})
	return b
}

// OptParam adds an optional parameter.
func (b *FunctionBuilder) OptParam(name string, t Type) *FunctionBuilder {
	b.params = append(b.params, Param{Name: name, Type: t, Optional: true})
	return b
}

// Variadic sets variadic parameter type.
func (b *FunctionBuilder) Variadic(t Type) *FunctionBuilder {
	b.variadic = t
	return b
}

// Returns sets return types.
func (b *FunctionBuilder) Returns(types ...Type) *FunctionBuilder {
	b.returns = types
	return b
}

// Build creates the function type.
func (b *FunctionBuilder) Build() *Function {
	return newCanonicalFunction(
		b.typeParams,
		b.params,
		b.variadic,
		b.returns,
	)
}

// CloneFunction returns an ownership-isolated copy of an already-canonical
// function type. It preserves the immutable hash/flag metadata and copies only
// the exported slice fields, avoiding a semantic rebuild. The private string
// cache is intentionally reset because sync.Once must not be copied after use.
func CloneFunction(fn *Function) *Function {
	if fn == nil {
		return nil
	}
	clone := &Function{
		TypeParams:        append([]*TypeParam(nil), fn.TypeParams...),
		Params:            append([]Param(nil), fn.Params...),
		Variadic:          fn.Variadic,
		Returns:           append([]Type(nil), fn.Returns...),
		hash:              fn.hash,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    fn.typeProperties.copyStatic(),
	}
	if functionSemanticNamesCanonical(clone.Params) {
		clone.semantic.Store(clone)
	}
	return clone
}

func (f *Function) Kind() kind.Kind { return kind.Function }

func (f *Function) String() string {
	return f.strCache.get(func() string { return renderTypeString(f) })
}

func (f *Function) Hash() uint64 { return f.hash }

func (f *Function) Equals(other Type) bool {
	return typeEquals(f, other)
}
