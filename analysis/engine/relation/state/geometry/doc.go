// Package geometry is the engine-local, read-only bridge from an authenticated
// mounted semantic address to the two pieces of input needed by state:
// a dense scalar row key and an exact support region.
//
// Geometry does not own a physical address book, a guard formula, or a
// terminal value.  The mounted witness owns row membership and scope
// authentication.  The cofiber authority performs the one conversion the
// mount layer deliberately cannot perform: schema/region.Region is the
// checked, concrete declaration while support.Mask belongs to the engine's
// guard manager.  Geometry receives only the sealed authority and never a
// conversion callback.
//
// In particular, this package does not hash logical identities, qualify a row
// key with a scope, or choose a default region.  A missing or foreign proof is
// a refusal.  A scope mask may describe many terminal cells; callers must use
// it as a partition input rather than assuming that a scope resolves to one
// cell.
package geometry
