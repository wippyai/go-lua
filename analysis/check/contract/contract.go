// Package contract exposes canonical callable contracts to post-solve
// obligation checkers. It is a read-only view over resolved module signatures
// and function types; it does not lower AST annotations.
package contract

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Contract is the canonical callable contract consumed by judgment producers.
// It owns type/effect facts only; names, spans, messages, and severities belong
// to lowering metadata and renderers.
type Contract struct {
	source    signature.Function
	params    []Param
	results   []Result
	variadic  Param
	hasVararg bool
}

// Param is one effective callable parameter.
type Param struct {
	Name         string
	Type         typ.Type
	Optional     bool
	Explicit     bool
	ImplicitSelf bool
}

// AcceptedType returns the type a caller may pass for this parameter. Optional
// parameters accept nil even when Type stores the non-nil payload type.
func (p Param) AcceptedType() typ.Type {
	if p.Optional {
		return typ.MaterializeOptional(p.Type)
	}
	return p.Type
}

// Result is one callable return slot.
type Result struct {
	Type     typ.Type
	Explicit bool
}

// FromSignature builds a canonical contract from an already-resolved signature.
func FromSignature(sig signature.Function) (Contract, bool) {
	if sig.Type == nil {
		return Contract{}, false
	}
	return FromFunctionType(sig.Type).withSignature(sig), true
}

// FromFunctionType builds a canonical contract from an already-resolved pure
// function type.
func FromFunctionType(fn *typ.Function) Contract {
	if fn == nil {
		return Contract{}
	}
	out := Contract{
		source:  signature.Function{Type: fn},
		params:  make([]Param, 0, len(fn.Params)),
		results: make([]Result, 0, len(fn.Returns)),
	}
	for _, param := range fn.Params {
		out.params = append(out.params, Param{
			Name:         param.Name,
			Type:         param.Type,
			Optional:     param.Optional || typevalue.TypeIncludesNil(param.Type) || !typeExplicit(param.Type),
			Explicit:     typeExplicit(param.Type),
			ImplicitSelf: param.Name == "self",
		})
	}
	for _, ret := range fn.Returns {
		out.results = append(out.results, Result{
			Type:     ret,
			Explicit: typeExplicit(ret),
		})
	}
	if fn.Variadic != nil {
		out.hasVararg = true
		out.variadic = Param{
			Type:     fn.Variadic,
			Optional: typevalue.TypeIncludesNil(fn.Variadic) || !typeExplicit(fn.Variadic),
			Explicit: typeExplicit(fn.Variadic),
		}
	}
	return out
}

func (c Contract) withSignature(sig signature.Function) Contract {
	c.source = sig.Clone()
	return c
}

// Signature returns an ownership-isolated copy of the source signature.
func (c Contract) Signature() signature.Function {
	return c.source.Clone()
}

// Params returns an ownership-isolated parameter list.
func (c Contract) Params() []Param {
	return append([]Param(nil), c.params...)
}

// ParamCount returns the number of fixed parameters before any variadic tail.
func (c Contract) ParamCount() int {
	return len(c.params)
}

// Results returns an ownership-isolated return-slot list.
func (c Contract) Results() []Result {
	return append([]Result(nil), c.results...)
}

// ParamAt returns the parameter for index, falling back to variadic when the
// contract has one.
func (c Contract) ParamAt(index int) (Param, bool) {
	if index < 0 {
		return Param{}, false
	}
	if index < len(c.params) {
		return c.params[index], true
	}
	if c.hasVararg {
		return c.variadic, true
	}
	return Param{}, false
}

// ResultAt returns the declared return slot at index.
func (c Contract) ResultAt(index int) (Result, bool) {
	if index < 0 || index >= len(c.results) {
		return Result{}, false
	}
	return c.results[index], true
}

// HasVararg reports whether this callable accepts variadic arguments.
func (c Contract) HasVararg() bool {
	return c.hasVararg
}

// Vararg returns the variadic parameter contract.
func (c Contract) Vararg() (Param, bool) {
	return c.variadic, c.hasVararg
}

// BindFirstParameter returns c with the first parameter consumed by a call-site
// owner that has already proven the argument is supplied implicitly.
func (c Contract) BindFirstParameter() (Contract, bool) {
	if len(c.params) == 0 {
		return c, false
	}
	c.params = append([]Param(nil), c.params[1:]...)
	return c, true
}

// RequiredArity returns the last required parameter position.
func (c Contract) RequiredArity() int {
	required := 0
	for i, param := range c.params {
		if param.Explicit && !param.Optional {
			required = i + 1
		}
	}
	return required
}

func typeExplicit(t typ.Type) bool {
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}
