// Package store owns one immutable solve-local relation state version.
//
// Store is the certified column catalog and Version is the aggregate staging
// state.  Database owns committed W2 roots. A Version retains exactly one column.Version for every certified
// logical column.  Column storage, semantic values, and column deltas remain
// owned by state/internal/column; this package only authenticates aggregate membership
// and stages immutable aggregate roots for the database publication owner.
package store
