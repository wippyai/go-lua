package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ConditionTypeDomain handles type narrowing based on type predicates and boolean constraints.
//
// ConditionTypeDomain is one of the three subdomains in ConditionProofDomain. It tracks narrowings
// derived from type-related constraints:
//
//   - HasType: Narrows a union to include only the specified type.
//     Example: HasType(x, "string") on x:string|number yields x:string
//
//   - NotHasType: Removes a type from a union.
//     Example: NotHasType(x, "nil") on x:string|nil yields x:string
//
//   - Truthy: Narrows to values that are truthy (not nil or false).
//     Example: Truthy(x) on x:string|nil yields x:string
//
//   - Falsy: Narrows to values that are falsy (nil or false).
//     Example: Falsy(x) on x:string|nil yields x:nil
//
//   - Eq/Ne with nil: Narrows based on nil equality.
//     Example: x ~= nil on x:string|nil yields x:string
//
// The domain maintains a map of narrowed types keyed by canonical path keys.
// When no narrowing exists, the domain falls back to env.LookupPathType for base types.
//
// ConditionTypeDomain supports boolean discriminant narrowing: when a field like .ok is
// tested for truthiness, and the parent type uses .ok as a discriminant, the
// parent type is also narrowed. This enables result type patterns:
//
//	---@type {ok: true, value: T} | {ok: false, error: string}
//	local result = ...
//	if result.ok then
//	    -- result is narrowed to {ok: true, value: T}
//	end
type ConditionTypeDomain struct {
	Narrowed map[constraint.PathKey]typ.Type
	Env      constraint.Env
	Unsat    bool
}

// NewConditionTypeDomain creates a new ConditionTypeDomain with the given environment.
//
// The environment provides type resolution functions:
//   - env.LookupPathType: Returns base type for a path key before narrowing
//   - env.ResolveType: Resolves type keys to actual types (for HasType atoms)
//   - env.Resolver: Field/index access for boolean discriminant narrowing
func NewConditionTypeDomain(env constraint.Env) *ConditionTypeDomain {
	return &ConditionTypeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type),
		Env:      env,
	}
}

// TypeAt returns the type for a PathKey, preferring narrowed types over base types.
//
// Lookup order:
//  1. d.Narrowed[key] - explicitly narrowed type from constraint application
//  2. env.LookupPathType(key) - base type from declarations
//
// Returns nil if the key has no type in either source.
func (d *ConditionTypeDomain) TypeAt(key constraint.PathKey) typ.Type {
	if t, ok := d.Narrowed[key]; ok {
		return t
	}
	return d.Env.LookupPathType(key)
}

// NarrowedTypeAt returns only the explicitly narrowed type, not the base type.
//
// Unlike TypeAt, this returns nil if there is no narrowing, even if the key
// has a base type. Used to check whether narrowing has occurred for a path.
func (d *ConditionTypeDomain) NarrowedTypeAt(key constraint.PathKey) typ.Type {
	return d.Narrowed[key]
}

// ApplyAtom applies a type-related constraint atom to the domain.
//
// Supported atom kinds:
//   - AtomKindHasType: Narrow to include only the specified type
//   - AtomKindNotHasType: Exclude the specified type from a union
//   - AtomKindTruthy: Narrow to truthy values (excludes nil and false)
//   - AtomKindFalsy: Narrow to falsy values (nil or false only)
//   - AtomKindEq with nil RHS: Narrow to exactly nil
//   - AtomKindNe with nil RHS: Exclude nil from the type
//   - AtomKindEq with two vars: No-op (equality tracked in E-graph)
//   - AtomKindNe with two vars: Exclude singleton types if applicable
//
// Returns false and sets Unsat=true if the atom proves unsatisfiability
// (e.g., HasType("string") on a number-only type).
func (d *ConditionTypeDomain) ApplyAtom(atom constraint.Atom) bool {
	if d.Unsat {
		return false
	}

	switch atom.Kind {
	case constraint.AtomKindHasType:
		return d.applyHasType(atom)
	case constraint.AtomKindNotHasType:
		return d.applyNotHasType(atom)
	case constraint.AtomKindTruthy:
		return d.applyTruthy(atom)
	case constraint.AtomKindFalsy:
		return d.applyFalsy(atom)
	case constraint.AtomKindEq:
		if atom.Right.IsNil() {
			return d.applyIsNil(atom)
		}
		if atom.Left.IsVar() && atom.Right.IsVar() {
			return d.applyEqVars(atom)
		}
	case constraint.AtomKindNe:
		if atom.Right.IsNil() {
			return d.applyNotNil(atom)
		}
		if atom.Left.IsVar() && atom.Right.IsVar() {
			return d.applyNeVars(atom)
		}
	}
	return true
}

func (d *ConditionTypeDomain) applyHasType(atom constraint.Atom) bool {
	key := atom.Left.Path
	if parent, field, ok := splitConditionPathKey(key); ok {
		if parentType := d.TypeAt(parent); parentType != nil {
			parentNarrowed := narrow.ByFieldTypeKey(parentType, field, atom.TypeKey, &d.Env, d.Env.ResolveType)
			if parentNarrowed == nil || parentNarrowed.Kind().IsNever() {
				d.Unsat = true
				return false
			}
			if !typ.TypeEquals(parentNarrowed, parentType) {
				d.Narrowed[parent] = parentNarrowed
			}
		}
	}

	base := d.TypeAt(key)
	if base == nil {
		return true
	}
	narrowed := narrow.ByTypeKey(base, atom.TypeKey, d.Env.ResolveType)
	if narrowed == nil || narrowed.Kind().IsNever() {
		d.Unsat = true
		return false
	}
	d.Narrowed[key] = narrowed
	return true
}

func (d *ConditionTypeDomain) applyNotHasType(atom constraint.Atom) bool {
	key := atom.Left.Path
	base := d.TypeAt(key)
	if base == nil {
		return true
	}
	narrowed := narrow.ExcludeByTypeKey(base, atom.TypeKey, d.Env.ResolveType)
	if narrowed == nil || narrowed.Kind().IsNever() {
		d.Unsat = true
		return false
	}
	d.Narrowed[key] = narrowed
	return true
}

func (d *ConditionTypeDomain) applyTruthy(atom constraint.Atom) bool {
	key := atom.Left.Path
	base := d.TypeAt(key)
	if base != nil {
		narrowed := narrow.ToTruthy(base)
		if narrowed == nil || narrowed.Kind().IsNever() {
			d.Unsat = true
			return false
		}
		d.Narrowed[key] = narrowed
	}

	// Parent narrowing: if this is a field path (r.ok), narrow the parent (r)
	// by the boolean discriminant if applicable.
	if parent, field, ok := splitConditionPathKey(key); ok {
		if parentType := d.TypeAt(parent); parentType != nil {
			if d.Env.HasResolver() && constraint.IsBooleanDiscriminantField(parentType, field, &d.Env) {
				parentNarrowed := narrow.ByFieldLiteral(parentType, field, typ.True, &d.Env)
				if parentNarrowed != nil && !parentNarrowed.Kind().IsNever() {
					d.Narrowed[parent] = parentNarrowed
				}
			} else if d.Env.HasResolver() {
				if parentNarrowed := narrow.ByFieldTruthy(parentType, field, &d.Env); parentNarrowed != nil && !parentNarrowed.Kind().IsNever() {
					if !typ.TypeEquals(parentNarrowed, parentType) {
						d.Narrowed[parent] = parentNarrowed
					}
				}
			}
		}
	}
	return true
}

func (d *ConditionTypeDomain) applyFalsy(atom constraint.Atom) bool {
	key := atom.Left.Path
	base := d.TypeAt(key)
	if base != nil {
		narrowed := narrow.ToFalsy(base)
		if narrowed == nil || narrowed.Kind().IsNever() {
			d.Unsat = true
			return false
		}
		d.Narrowed[key] = narrowed
	}

	// Parent narrowing: if this is a field path (r.ok), narrow the parent (r)
	// by the boolean discriminant if applicable.
	if parent, field, ok := splitConditionPathKey(key); ok {
		if parentType := d.TypeAt(parent); parentType != nil {
			if d.Env.HasResolver() && constraint.IsBooleanDiscriminantField(parentType, field, &d.Env) {
				parentNarrowed := narrow.ByFieldLiteral(parentType, field, typ.False, &d.Env)
				if parentNarrowed != nil && !parentNarrowed.Kind().IsNever() {
					d.Narrowed[parent] = parentNarrowed
				}
			} else if d.Env.HasResolver() {
				if parentNarrowed := narrow.ByFieldFalsy(parentType, field, &d.Env); parentNarrowed != nil && !parentNarrowed.Kind().IsNever() {
					if !typ.TypeEquals(parentNarrowed, parentType) {
						d.Narrowed[parent] = parentNarrowed
					}
				}
			}
		}
	}
	return true
}

func (d *ConditionTypeDomain) applyIsNil(atom constraint.Atom) bool {
	key := atom.Left.Path
	if parent, field, ok := splitConditionPathKey(key); ok {
		if parentType := d.TypeAt(parent); parentType != nil {
			parentNarrowed := narrow.ByFieldTypeKey(parentType, field, narrow.BuiltinTypeKey("nil"), &d.Env, d.Env.ResolveType)
			if parentNarrowed == nil || parentNarrowed.Kind().IsNever() {
				d.Unsat = true
				return false
			}
			if !typ.TypeEquals(parentNarrowed, parentType) {
				d.Narrowed[parent] = parentNarrowed
			}
		}
	}

	base := d.TypeAt(key)
	if base == nil {
		return true
	}
	narrowed := narrow.FilterByKind(base, kind.Nil)
	if narrowed == nil || narrowed.Kind().IsNever() {
		d.Unsat = true
		return false
	}
	d.Narrowed[key] = narrowed
	return true
}

func (d *ConditionTypeDomain) applyNotNil(atom constraint.Atom) bool {
	key := atom.Left.Path
	base := d.TypeAt(key)
	if base == nil {
		return true
	}
	narrowed := narrow.RemoveNil(base)
	if narrowed == nil || narrowed.Kind().IsNever() {
		d.Unsat = true
		return false
	}
	d.Narrowed[key] = narrowed
	return true
}

func (d *ConditionTypeDomain) applyEqVars(atom constraint.Atom) bool {
	// EqPath (x == y) does NOT narrow types.
	// Equivalence is tracked in the E-graph. Type narrowings are only from
	// HasType, Truthy, Falsy atoms, and propagate through equivalence classes.
	return true
}

func (d *ConditionTypeDomain) applyNeVars(atom constraint.Atom) bool {
	leftKey := atom.Left.Path
	rightKey := atom.Right.Path
	leftType := d.TypeAt(leftKey)
	rightType := d.TypeAt(rightKey)

	if leftType == nil || rightType == nil {
		return true
	}

	// For x ~= y, if y is a singleton type (literal or nil), exclude it from x
	// and vice versa
	changed := false

	if unwrap.IsSingleton(rightType) {
		narrowedLeft := narrow.ExcludeType(leftType, rightType)
		if narrowedLeft != nil && !narrowedLeft.Kind().IsNever() {
			d.Narrowed[leftKey] = narrowedLeft
			changed = true
		} else if narrowedLeft != nil && narrowedLeft.Kind().IsNever() {
			d.Unsat = true
			return false
		}
	}

	if unwrap.IsSingleton(leftType) {
		narrowedRight := narrow.ExcludeType(rightType, leftType)
		if narrowedRight != nil && !narrowedRight.Kind().IsNever() {
			d.Narrowed[rightKey] = narrowedRight
			changed = true
		} else if narrowedRight != nil && narrowedRight.Kind().IsNever() {
			d.Unsat = true
			return false
		}
	}

	_ = changed
	return true
}

// IsUnsat returns true if the domain has proven a contradiction.
func (d *ConditionTypeDomain) IsUnsat() bool { return d.Unsat }

// Clone creates a deep copy of the ConditionTypeDomain for speculative evaluation.
//
// The Narrowed map is deep copied; the Env is shared by reference.
func (d *ConditionTypeDomain) Clone() *ConditionTypeDomain {
	c := &ConditionTypeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type, len(d.Narrowed)),
		Env:      d.Env,
		Unsat:    d.Unsat,
	}
	for _, k := range constraint.SortedPathKeys(d.Narrowed) {
		c.Narrowed[k] = d.Narrowed[k]
	}
	return c
}

// Join computes the least upper bound of two ConditionTypeDomain states.
//
// Join semantics for type domains:
//   - If either domain is unsatisfiable, return a clone of the other
//   - For keys in both domains, compute type union (typejoin.Types)
//   - Keys only in one domain are dropped (no narrowing survives)
//
// The intuition: after joining branches, we only know facts that hold in BOTH.
// If one branch has x:string and the other doesn't narrow x at all, the joined
// result has no narrowing for x (could be anything).
func (d *ConditionTypeDomain) Join(other *ConditionTypeDomain) *ConditionTypeDomain {
	o := other
	if d.Unsat {
		return o.Clone()
	}
	if o.Unsat {
		return d.Clone()
	}

	result := &ConditionTypeDomain{
		Narrowed: make(map[constraint.PathKey]typ.Type),
		Env:      d.Env,
	}

	// Type union for keys in both (meet of facts)
	for _, key := range constraint.SortedPathKeys(d.Narrowed) {
		dt := d.Narrowed[key]
		if ot, ok := o.Narrowed[key]; ok {
			result.Narrowed[key] = typejoin.Types(dt, ot)
		}
		// Keys only in one side are dropped
	}
	return result
}
