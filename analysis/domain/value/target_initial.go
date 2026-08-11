package value

import (
	"math"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	"github.com/wippyai/go-lua/program/target"
)

// TargetInitial projects one exact Target initial value in the actor-local
// context of boot. Heap remains responsible for classifying Nil and Absent as
// RawAbsent; this projection deliberately exposes nil only for consumers that
// require a runtime Value and rejects Absent because it has none.
//
// Primitive target payload precision remains Target/Numeric-owned. Value
// returns its compact runtime family atom, while roots and callables retain
// their exact Link identities. A denied callable is therefore not widened to
// opaque Function: its later typed Throw remains Call-owned and observable.
func (schema *Schema) TargetInitial(root linkhost.BootRoot, initial target.InitialValue) (Value, bool) {
	if schema == nil || schema.source == nil || initial == 0 {
		return Value{}, false
	}
	contract, ok := schema.source.Boundary().Target()
	if !ok || contract == nil {
		return Value{}, false
	}
	_, mappedRoot, rootOK := schema.source.Host().BootRoots().Mapping(root)
	kind, kindOK := contract.InitialValueKind(initial)
	if !rootOK || !kindOK {
		return Value{}, false
	}
	atom := Atom{}
	switch kind {
	case target.InitialValueAbsent:
		return Value{}, false
	case target.InitialValueNil:
		atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomNil}]}
	case target.InitialValueBoolean:
		value, ok := contract.InitialValueBoolean(initial)
		if !ok {
			return Value{}, false
		}
		if keyed, exact := schema.targetInitialLiteralAtom(initial, runtimekind.Boolean, !value); exact {
			atom = keyed
		} else {
			kind := atomFalse
			if value {
				kind = atomTrue
			}
			atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: kind}]}
		}
	case target.InitialValueInteger, target.InitialValueFloat:
		if keyed, exact := schema.targetInitialLiteralAtom(initial, runtimekind.Number, false); exact {
			atom = keyed
		} else if kind == target.InitialValueFloat {
			bits, bitsOK := contract.InitialValueFloatBits(initial)
			if !bitsOK {
				return Value{}, false
			}
			if math.IsNaN(math.Float64frombits(bits)) {
				atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomNaN, runtime: runtimekind.Number}]}
			} else {
				atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.Number}]}
			}
		} else {
			atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.Number}]}
		}
	case target.InitialValueString:
		if keyed, exact := schema.targetInitialLiteralAtom(initial, runtimekind.String, false); exact {
			atom = keyed
		} else {
			atom = Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.String}]}
		}
	case target.InitialValueRoot:
		targetRoot, ok := contract.InitialValueRoot(initial)
		if !ok || targetRoot != mappedRoot {
			return Value{}, false
		}
		atom, ok = schema.Boot(root)
		if !ok {
			return Value{}, false
		}
	case target.InitialValueOperation, target.InitialValueDeniedOperation:
		seed, _, ok := schema.source.Boundary().Seeds().BootstrapCallable(initial)
		if !ok {
			return Value{}, false
		}
		atom, ok = schema.Callable(seed)
		if !ok {
			return Value{}, false
		}
	default:
		return Value{}, false
	}
	return schema.Singleton(atom)
}

// targetInitialLiteralAtom uses only Link's sealed Target-initial projection.
// The bool-false bit is validated by the Target kind/value query that already
// selected this branch; no key payload decoding or lookup occurs at runtime.
func (schema *Schema) targetInitialLiteralAtom(initial target.InitialValue, runtime runtimekind.Kind, falsy bool) (Atom, bool) {
	if schema == nil || schema.source == nil {
		return Atom{}, false
	}
	contract, contractOK := schema.source.Boundary().Target()
	if !contractOK || contract == nil {
		return Atom{}, false
	}
	key, ok := schema.source.Project().Keys().ForInitial(contract, initial)
	if !ok {
		return Atom{}, false
	}
	id := schema.atomByRow[atomRow{
		kind:         atomLiteral,
		runtime:      runtime,
		key:          key,
		hasKey:       true,
		literalFalsy: falsy,
	}]
	if id == 0 {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}
