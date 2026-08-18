package value

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// TargetInitialID is the sealed projection for one Host boot-root identity
// and Target initial value. The host alias resolution was completed during
// sealing; no hot caller can reopen Host or Boundary to recover it.
func (schema *Schema) TargetInitialID(root identity.ContentID, initial vocabulary.InitialValue) (Value, bool) {
	if schema == nil || !root.Available() || initial == 0 {
		return Value{}, false
	}
	value, ok := schema.targetInitials[targetInitialKey{root: root, initial: initial}]
	return value, ok && value.valid() && value.schema == schema
}

func (schema *valueBuilder) sealTargetInitialResults() bool {
	if schema == nil || schema.sealHost() == nil || schema.targetInitials == nil || len(schema.targetInitials) != 0 {
		return false
	}
	for rootIndex := 0; rootIndex < schema.sealHost().BootRoots().Count(); rootIndex++ {
		root, rootOK := schema.sealHost().BootRoots().At(rootIndex)
		rootID, rootIDOK := schema.sealHost().BootRoots().ID(root)
		if !rootOK || !rootIDOK || !rootID.Available() {
			return false
		}
		if !schema.visitTargetInitialValues(func(initial vocabulary.InitialValue) bool {
			fact, ok := schema.targetInitialCold(root, initial)
			if !ok {
				// InitialValueAbsent has no Value fact and remains a precise
				// no-candidate case rather than a fabricated bottom image.
				kind, kindOK := schema.sealBoundary().Target()
				if !kindOK || kind == nil {
					return false
				}
				initialKind, initialKindOK := kind.InitialValueKind(initial)
				return initialKindOK && initialKind == vocabulary.InitialValueAbsent
			}
			key := targetInitialKey{root: rootID, initial: initial}
			if prior, duplicate := schema.targetInitials[key]; duplicate {
				return schema.Equal(prior, fact)
			}
			schema.targetInitials[key] = fact
			return true
		}) {
			return false
		}
	}
	return true
}

// targetInitialCold projects one exact Target initial value in the actor-local
// context of boot. Heap remains responsible for classifying Nil and Absent as
// RawAbsent; this projection deliberately exposes nil only for consumers that
// require a runtime Value and rejects Absent because it has none.
//
// Primitive target payload precision remains Target/Numeric-owned. Value
// returns its compact runtime family atom, while roots and callables retain
// their exact Link identities. A denied callable is therefore not widened to
// opaque Function: its later typed Throw remains Call-owned and observable.
func (schema *valueBuilder) targetInitialCold(root linkhost.BootRoot, initial vocabulary.InitialValue) (Value, bool) {
	if schema == nil || schema.sealProject() == nil || initial == 0 {
		return Value{}, false
	}
	contract, ok := schema.sealBoundary().Target()
	if !ok || contract == nil {
		return Value{}, false
	}
	_, mappedRoot, rootOK := schema.sealHost().BootRoots().Mapping(root)
	kind, kindOK := contract.InitialValueKind(initial)
	if !rootOK || !kindOK {
		return Value{}, false
	}
	atom := Atom{}
	switch kind {
	case vocabulary.InitialValueAbsent:
		return Value{}, false
	case vocabulary.InitialValueNil:
		atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: atomNil}]}
	case vocabulary.InitialValueBoolean:
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
			atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: kind}]}
		}
	case vocabulary.InitialValueInteger, vocabulary.InitialValueFloat:
		if keyed, exact := schema.targetInitialLiteralAtom(initial, runtimekind.Number, false); exact {
			atom = keyed
		} else if kind == vocabulary.InitialValueFloat {
			bits, bitsOK := contract.InitialValueFloatBits(initial)
			if !bitsOK {
				return Value{}, false
			}
			if math.IsNaN(math.Float64frombits(bits)) {
				atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: atomNaN, runtime: runtimekind.Number}]}
			} else {
				atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.Number}]}
			}
		} else {
			atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.Number}]}
		}
	case vocabulary.InitialValueString:
		if keyed, exact := schema.targetInitialLiteralAtom(initial, runtimekind.String, false); exact {
			atom = keyed
		} else {
			atom = Atom{schema: schema.Schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.String}]}
		}
	case vocabulary.InitialValueRoot:
		targetRoot, ok := contract.InitialValueRoot(initial)
		if !ok {
			return Value{}, false
		}
		// Target entries may expose a different boot aggregate (for example,
		// the global `table` entry points at TableRoot).  Rebind that target
		// root through the same actor-local Host fence before projecting it;
		// comparing Target root ordinals would reject lawful aliases and using
		// a root from another Host/actor would cross the Value identity fence.
		projectedRoot := root
		if targetRoot != mappedRoot {
			actor, _, actorOK := schema.sealHost().BootRoots().Mapping(root)
			if !actorOK {
				return Value{}, false
			}
			projectedRoot, ok = schema.sealHost().BootRoots().For(actor, targetRoot)
			if !ok {
				return Value{}, false
			}
		}
		projectedID, projectedIDOK := schema.sealHost().BootRoots().ID(projectedRoot)
		if !projectedIDOK {
			return Value{}, false
		}
		atom, ok = schema.BootID(projectedID)
		if !ok {
			return Value{}, false
		}
	case vocabulary.InitialValueOperation, vocabulary.InitialValueDeniedOperation:
		seed, _, ok := schema.sealBoundary().Seeds().BootstrapCallable(initial)
		if !ok {
			require, hasRequire := schema.sealBoundary().RequireOperation()
			op, opOK := contract.InitialValueOperation(initial)
			if !hasRequire || !opOK || op != require {
				return Value{}, false
			}
			atom, ok = schema.ScopedLoader(op)
			if !ok {
				return Value{}, false
			}
			break
		}
		seedID, seedIDOK := schema.sealBoundary().Seeds().ID(seed)
		if !seedIDOK {
			return Value{}, false
		}
		atom, ok = schema.CallableID(seedID)
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
func (schema *valueBuilder) targetInitialLiteralAtom(initial vocabulary.InitialValue, runtime runtimekind.Kind, falsy bool) (Atom, bool) {
	if schema == nil || schema.sealProject() == nil {
		return Atom{}, false
	}
	contract, contractOK := schema.sealBoundary().Target()
	if !contractOK || contract == nil {
		return Atom{}, false
	}
	key, ok := schema.sealProject().Keys().ForInitial(contract, initial)
	if !ok {
		return Atom{}, false
	}
	literal, literalOK := schema.sealProject().Keys().Exact(key)
	if !literalOK {
		return Atom{}, false
	}
	id := schema.atomByRow[atomRow{
		kind:         atomLiteral,
		runtime:      runtime,
		key:          literal,
		hasKey:       true,
		literalFalsy: falsy,
	}]
	if id == 0 {
		return Atom{}, false
	}
	return Atom{schema: schema.Schema, id: id}, true
}
