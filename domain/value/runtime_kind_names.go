package value

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// sealRuntimeKindAtoms admits the result names from the one sealed structural
// vocabulary. The Value domain does not own a second list of type() strings:
// the table supplies both the runtime family identity and its spelling.
//
// These are computed literals rather than authored Link literals. They are
// exact Value alternatives for the type() result, but they deliberately have
// no ExactKey identity and therefore cannot become Heap keys by accident.
func (schema *valueBuilder) sealRuntimeKindAtoms() bool {
	if schema == nil || schema.structural.Count(structure.CategoryRuntimeKind) != int(runtimekind.Count)-1 {
		return false
	}
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, entryOK := schema.structural.At(structure.CategoryRuntimeKind, uint16(kind))
		declared, declaredOK := runtimekind.KindFor(entry)
		if !entryOK || !declaredOK || declared != kind || !entry.Accepted() || entry.Spelling() == "" {
			return false
		}
		atom := schema.addAtom(atomRow{
			kind:    atomComputedLiteral,
			runtime: runtimekind.String,
			key:     keyspace.LiteralValue{Kind: keyspace.LiteralString, String: entry.Spelling()},
		})
		if atom == 0 {
			return false
		}
		schema.runtimeKindNameAtoms[kind] = atom
	}
	return true
}
