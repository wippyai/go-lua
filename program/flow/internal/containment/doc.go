// Package containment proves Flow's one canonical child-to-parent relation.
//
// The pass is deliberately a derived proof only. It retains dense parents,
// interval coordinates, and the exact static-expression bitset; source,
// owner views, spans, and all construction scratch disappear at return.
package containment
