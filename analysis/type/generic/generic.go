// Package generic provides generic type projections.
package generic

import (
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Arg returns the type argument at index from an instantiated generic.
func Arg(t typ.Type, index int) (typ.Type, bool) {
	if index < 0 {
		return nil, false
	}
	inst, ok := unwrap.Alias(t).(*typ.Instantiated)
	if !ok || inst == nil || index >= len(inst.TypeArgs) || inst.TypeArgs[index] == nil {
		return nil, false
	}
	return inst.TypeArgs[index], true
}

// InstantiateOne creates a one-argument instantiation of generic using arg as
// the payload. Meta<T> arguments project to T before instantiation.
func InstantiateOne(generic typ.Type, arg typ.Type) (typ.Type, bool) {
	g, ok := unwrap.Alias(generic).(*typ.Generic)
	if !ok || g == nil || len(g.TypeParams) != 1 || arg == nil {
		return nil, false
	}
	payload := payloadType(arg)
	if payload == nil {
		return nil, false
	}
	if constraint := g.TypeParams[0].Constraint; constraint != nil && !subtype.IsSubtype(payload, constraint) {
		return nil, false
	}
	return typ.Instantiate(g, payload), true
}

func payloadType(t typ.Type) typ.Type {
	if meta, ok := unwrap.Alias(t).(*typ.Meta); ok && meta != nil && meta.Of != nil {
		return meta.Of
	}
	return t
}
