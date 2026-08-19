// Package flowtest holds test-only fixture plumbing shared by the packages
// under analysis/program/flow/internal. It is an internal package: nothing
// outside analysis/program/flow may import it, and it exports no production
// behavior, only helpers that assemble or tear down pipeline fixtures the
// same way every calling package's tests already did by hand.
package flowtest

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// CloseFinalizers aborts a Source/Static/Authored/Imports finalizer set in
// reverse construction order. Each Abort is independent: it locks only its
// own owner's state and has no cross-owner precondition, so the order of
// these calls does not change which finalizers end up aborted.
func CloseFinalizers(sourceFinal source.Finalizer, staticFinal static.Finalizer, flowFinal authored.Finalizer, moduleFinal imports.Finalizer) {
	_ = moduleFinal.Abort()
	_ = flowFinal.Abort()
	_ = staticFinal.Abort()
	_ = sourceFinal.Abort()
}

// ContentIDAt returns an identity.ContentID with only its first byte set to
// value, the shape every package-local test ID helper built by hand.
func ContentIDAt(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

// Term returns the keyspace.Term for a (family, ordinal) pair.
func Term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

// FamilySpans builds one source.FamilySpans row per keyspace.Family, each
// carrying counts[family] synthetic single-line spans named after name.
func FamilySpans(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	var families []source.FamilySpans
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		families = append(families, source.FamilySpans{Family: family, Spans: spans})
	}
	return families
}

// LiteralRows builds count rows via build, one per ordinal from 1 to count.
// The owner passed to build is overrides[ordinal-1] when that override
// exists, otherwise defaultOwner - the owner-with-override pattern each
// package's literal-row builders (Nil/Bool/Integer/...) repeated by hand.
func LiteralRows[T any](count uint32, overrides []keyspace.Term, defaultOwner keyspace.Term, build func(owner keyspace.Term, ordinal uint32) T) []T {
	var rows []T
	for ordinal := uint32(1); ordinal <= count; ordinal++ {
		owner := defaultOwner
		if int(ordinal) <= len(overrides) {
			owner = overrides[ordinal-1]
		}
		rows = append(rows, build(owner, ordinal))
	}
	return rows
}
