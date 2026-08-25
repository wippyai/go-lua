// Package relationoracle is a deliberately small full-recomputation reference
// evaluator used only by tests of the physical relational engine.
//
// Relation and Row are immutable logical values. A row is keyed by a model.RowID
// and every cell is keyed by a model.ColumnID and model.TypeID; no mount
// address, ordinal, arrangement, or execution protocol appears here. Values
// are opaque ValueTokens, and TypeID-specific equality/Join behavior enters
// only through AlgebraRegistry. Scope carries only an opaque identity.ContentID.
// SelectByScope and Join consult a caller-supplied ScopeAlgebra, while Merge
// and Publish consult the TypeID-keyed AlgebraRegistry. Apply crosses its only
// semantic boundary through the tiny Judgment interface and retains
// outcome.Result separately from rows.
//
// The direct operator surface is Input/Scan, SelectByScope, Project, Join,
// Merge, GroupBy, Complete, Apply, and Publish. Every operator recomputes
// from immutable logical rows; the implementation intentionally has no
// physical index or incremental protocol.
package relationoracle
