// Package mounted owns the populations a Link seals when it places compiled
// Programs at mounts: which semantic execution points exist, which of them are
// independent execution roots, and where results are observed.
//
// A mount is a Link fact and a point is an artifact fact, so the population
// that relates the two belongs to neither alone. It is derived here, from the
// immutable compiled artifact plus the Link-local module identity that places
// it, and from nothing else. No query family, diagnostic flag, or solver state
// is consulted: what exists to be executed is a property of the sealed program
// at its mount, not of who intends to read it.
//
// Each population is a plain sealed value: an ordered row set with an exact
// count and an addressable row. Order is canonical -- rows are sorted by the
// bytes of their key -- so a census is a function of the sealed content alone
// and never of map iteration, registration order, mount order, or the order a
// consumer published in.
package mounted
