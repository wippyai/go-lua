package runtimekind

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// RuntimeKindResultRole is the one semantic relation the runtime-kind
// provider exposes to operation declarations. It identifies the fact that a
// result value classifies an existing input value through the runtime-kind
// vocabulary. Individual family names remain the rows of CategoryRuntimeKind;
// they are not repeated in the operation or provider wire model.
const RuntimeKindResultRole = "runtime-kind/result"

// RuntimeKindPredicateRole is the neutral operation-predicate relation used
// when a runtime-kind result is compared against a declared family name. The
// branch polarity and route remain Program/Value-owned geometry.
const RuntimeKindPredicateRole = "runtime-kind/predicate"

// RuntimeKindResultRelationKey is the structural key of RuntimeKindResultRole.
// A portable provider carries this key as a string; manifesttarget derives the
// opaque schema.EntryID from it after the manifest crosses into the Lua domain.
const RuntimeKindResultRelationKey schema.Key = "semantic/" + RuntimeKindResultRole

// RuntimeKindPredicateRelationKey is the structural key of
// RuntimeKindPredicateRole.
const RuntimeKindPredicateRelationKey schema.Key = "semantic/" + RuntimeKindPredicateRole

// BehaviorStructureSpecs contributes the runtime-kind behavior relation to
// the sealed structural semantic-role vocabulary. It is separate from
// StructureSpecs so the latter remains exactly the closed family catalog that
// Kind and Set own.
func BehaviorStructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuntimeKindResultRole, RuntimeKindPredicateRole)
}

// StructureSpecs is this domain's declaration of the Lua runtime family
// vocabulary: one row per family type() distinguishes, in the domain's own
// order.
//
// The ordinals are not invented here. A family's ordinal is its Kind constant,
// which is also its bit position in Set, so the declaration adopts the
// vocabulary the domain already carries rather than restating it: a family
// added, removed, or reordered in Kind and not here is a rejected build.
//
// A row's key and its rendered spelling are both the string type(v) yields for
// the family. That is the family's one spelling in the analyzer, so a consumer
// that needs the name reads it from the sealed table instead of keeping a
// second list of the eight names beside it.
//
// Every family is projected. type() yields any of them, so no member is held
// back from the projection this vocabulary feeds.
func StructureSpecs() []structure.Spec {
	return []structure.Spec{
		member(Nil, "nil"),
		member(Boolean, "boolean"),
		member(Number, "number"),
		member(String, "string"),
		member(Table, "table"),
		member(Function, "function"),
		member(Thread, "thread"),
		member(Userdata, "userdata"),
	}
}

func member(kind Kind, name schema.Key) structure.Spec {
	return structure.Spec{
		Key:      name,
		Category: structure.CategoryRuntimeKind,
		Ordinal:  uint16(kind),
		Spelling: string(name),
		Accepted: true,
	}
}

// KindFor projects one declared row back to the family it declares. A consumer
// reading the sealed vocabulary recovers the family through this rather than
// converting an ordinal of its own.
func KindFor(entry *structure.Entry) (Kind, bool) {
	if entry == nil || entry.Category() != structure.CategoryRuntimeKind {
		return Invalid, false
	}
	kind := Kind(entry.Ordinal())
	return kind, kind.Valid()
}
