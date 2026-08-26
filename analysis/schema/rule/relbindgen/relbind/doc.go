// Package relbind is the generator for the two artifacts the semantic ABI
// admits as irreducible generated code.
//
// Relation-engine section 6.1 admits exactly two generated artifacts per
// family: a thin typed semantic-operation binding, and a thin typed owner-
// column publisher. This package emits those two and refuses anything else.
//
// The emission is driven by the sealed signature vocabulary itself. A slot's
// frame shape is signature.DeliveryKind, so a scalar slot decodes through
// ScalarAt and a span slot borrows through SpanAt; a family's row bound is
// model.CardinalityKind, so an exactly-one family addresses its destination by
// a declared scalar slot and a bounded-many family names each destination by
// the owner's own content identity. Those are the ABI's derived facts, and the
// generator computes them rather than reading a capability label.
//
// What the generator cannot state is owner mathematics. Each family supplies
// one hand-authored judgment whose Evaluate names only the decoded argument
// and the bounded emitter the generated artifact declares, so the mathematics
// stays where its owner wrote it and the generated half stays free of it.
//
//go:generate go run ./cmd/relbind -root ../../../../..
package relbind
