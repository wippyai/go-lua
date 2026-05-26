// Package escape is the Escape/Allocation axis of the reduced-product abstract
// value.
//
// # Axis law
//
// The Escape/Allocation axis abstracts whether a value escapes its allocating
// frame and therefore where it must be allocated. Its lattice orders escape
// states from NoEscape (stack-allocatable) to Escapes (heap-required); Top is
// "escapes" (conservative) and Bottom is unreachable. Join is the least upper
// bound: if a value escapes on any incoming path it escapes at the join.
//
// The axis is independently sound: Join never reports fresh unless it holds on all
// paths, so stack allocation derived from this axis is always safe.
//
// # Lattice
//
// The carrier is the three-element chain bottom < fresh < escaped. Fresh is
// confined to its allocating frame; escaped has been published. Top is escaped
// (the conservative assumption) and Bottom is unreachable. Join takes the higher
// point, so a value that escapes on any incoming path escapes at the join.
//
// The product populates the axis with the safe default (Top) at admission, and its
// cross-axis reducer is the identity: this axis carries the lattice but does not
// yet drive the transfer engine's fresh-vs-escaped demotion, which stays on the
// engine's fresh-table and escape-evidence events.
package escape
