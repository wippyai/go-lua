// Package index owns solve-local immutable physical arrangements over one
// mounted layout. Values remain owner-issued binding tokens; this package
// never decodes domain payloads or reconstructs logical row identities.
//
// New consumes only a concrete mounted witness and aggregate store version.
// It discovers postings through store.Version.Scan, joins ordered key-column
// partitions at identical geometry keys, and derives delivered postings from
// the layout's declared key/delivered columns. Each sealed root also owns an
// exact persistent RowID posting inverse; keyed readers redeem that inverse
// directly, while Next applies semantic store deltas by changed posting and
// shares immutable roots for lineage-only changes.
// Key edges are canonical representative groups ordered by opaque identity
// only for deterministic presentation. Because ValueEquality supplies no
// lawful order or hash, semantic insertion/lookup is linear in the groups at
// each depth; the warm lookup remains allocation-free.
package index
