package runtimekind

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// StructureSpecs is this domain's declaration of the Lua runtime family
// vocabulary: one row per family type() distinguishes, in the domain's own
// order.
//
// The ordinals are not invented here. A family's ordinal is its Kind constant,
// which is also its bit position in Set, so the declaration adopts the
// vocabulary the domain already carries rather than restating it: a family
// added, removed, or reordered in Kind and not here is a rejected build.
//
// A row's key is the string type(v) yields for the family. That is the
// family's one spelling in the analyzer, so a consumer that needs the name
// reads it from the sealed table instead of keeping a second list of the eight
// names beside it.
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
