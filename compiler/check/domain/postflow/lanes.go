// Package postflow owns carrier shapes for the noncanonical postflow projection
// lanes. These are compatibility/export projection facts, not canonical Summary
// facts.
package postflow

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// FieldKey identifies a statically-known field/index slot in postflow
// projection state.
type FieldKey = fieldkey.Key

// FieldValues maps a typed field/path segment to its product-domain value.
type FieldValues = map[FieldKey]product.AbstractValue

// CapturedTypes maps captured symbols to their flow-derived product values for
// a graph.
type CapturedTypes map[cfg.SymbolID]product.AbstractValue

// CapturedFieldAssigns maps nested function symbols to assignments they make
// to captured variables from parent scopes.
type CapturedFieldAssigns map[cfg.SymbolID]map[cfg.SymbolID]FieldValues

// ConstructorFields maps class symbols to field assignments captured in
// constructors.
type ConstructorFields map[cfg.SymbolID]FieldValues
