// Package reduction owns the derived destination-cell fold for contribution
// rows.
//
// Contribution.State is the authority for producer rows.  This package is a
// pure projection: it never inserts, removes, or mutates a row and it never
// treats the visible aggregate as an authority.  A caller supplies one exact
// schema target, its canonical producer rows, the mounted value/lineage
// authorities, and the sealed output declaration.  The reducer validates the
// complete boundary before producing either a reduced cell or an explicit
// sparse removal.
package reduction
