// Package contribution owns the immutable producer-contribution plane.
//
// A contribution row is addressed by an already-issued Handle, output port,
// destination RowID, and the exact mounted CellToken that supplied that row.
// State never derives an invocation identity, owns no aggregate cell writer,
// and never removes a destination cell as a whole. Upsert replaces exactly one
// producer row; Remove deletes exactly one producer row. Consumers reduce
// exact output Targets through the deterministic callback surface exposed
// here.
package contribution
