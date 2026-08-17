// Package composite is the analyzer's composition layer, and it is that layer
// on two levels: this package is the composition root, and its subpackages are
// where cross-domain relations live.
//
// The package itself is the analyzer composition root: it composes the
// artifact columns with the domain registrations and seals the one
// process-global analyzer catalog. It is the single role permitted to know
// both worlds, and it sits above the schema surfaces and the domains rather
// than inside either. Its Compilation is the only authority accepted by the
// Program transformer; callers cannot manufacture an equivalent authority from
// a digest.
//
// Neither the axis nor the rule inventory is written here: they are the two
// surfaces of the analyzer declaration table, and this package composes that
// table's cold declaration pass.
//
// The subpackages host cross-domain relations that no single domain may own
// without minting a new inter-domain import edge.
//
// Placement law (kickside journal seq 2103):
//
//   - Logic reading a single domain lives in that domain's package under
//     analysis/domain/<x>, declared through the domain's registration.
//   - A cross-domain relation lives inside a member domain only when that
//     domain already imports every other member for its own semantics; the
//     relation is then that domain's semantics reaching through cones.
//   - Otherwise the relation lives here, in composite/<name>: a sibling
//     package above the peer members that imports them all and is the one
//     writer of the relation's result. Member domains learn nothing about
//     the relations they participate in.
//
// Invariant, checkable on the import graph: analysis/domain/<x> never
// imports analysis/domain/<y> unless the edge exists for <x>'s own
// semantics; composite/* packages are the only place a new multi-domain
// edge may appear. This package composes catalogs and holds no relation
// logic.
//
// Each subpackage holds the relation itself: the typed evidence it derives
// from its members, and the proof path that issues that evidence, stated in
// the members' own vocabulary.
//
// No subpackage carries a composite.Spec row on the schema composite surface
// (analysis/schema/composite), and the analyzer's composite inventory here is
// correspondingly empty. The row - its roles, the cones they read their
// members under, and its output axis - lands with the store cut, because the
// half a composite is declared against, the typed Frame and its admitted
// write, lands with the store cut. The deferral is documented at
// analysis/domain/composite/composite_table.go, which registers the surface
// with no rows so the omission stays visible.
package composite
