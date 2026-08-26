package snapshot

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
)

// ExtendQuery adds one sealed query column to an already published hot
// snapshot without advancing its database generation.  This is a
// publication-only operation: it shares every existing column, denominator,
// and directory trie and appends only the new query column and its directory
// entries.  The database root that supplied the snapshot is not touched.
//
// The ordinary NewDelta path deliberately requires a generation advance for
// state changes.  A query projection is not a state change, so it uses this
// separate type-level entry point rather than lying about a successor root.
// It still uses DeclareQuery, PutColumn, the one canonical trie, and the same
// seal checks as every other query publication.
func ExtendQuery[K comparable, O any](base Snapshot, family identity.ContentID, content Content[K, O]) (Snapshot, QueryPlan[K, O], error) {
	if !base.Published() {
		return Snapshot{}, QueryPlan[K, O]{}, fmt.Errorf("%w: base", ErrUnavailableIdentity)
	}
	if !family.Available() {
		return Snapshot{}, QueryPlan[K, O]{}, fmt.Errorf("%w: query family", ErrUnavailableIdentity)
	}
	builder := Builder{
		builderCore: builderCore{
			schema:           base.schema,
			store:            base.store,
			generation:       base.generation,
			columns:          append([]any(nil), base.columns...),
			authored:         make([]bool, len(base.columns)),
			directory:        base.directory,
			denominators:     base.denominators.index,
			denominatorCount: base.denominators.count,
		},
		queries:    base.queries.plans,
		queryCount: base.queries.count,
	}
	if _, err := DeclareQuery(&builder, family, uint32(len(base.columns)), content); err != nil {
		return Snapshot{}, QueryPlan[K, O]{}, err
	}
	sealed, err := builder.Seal()
	if err != nil {
		return Snapshot{}, QueryPlan[K, O]{}, err
	}
	if !sealed.Published() || sealed.Schema() != base.Schema() || sealed.Store() != base.Store() || sealed.Generation() != base.Generation() || sealed.Columns() != base.Columns()+1 {
		return Snapshot{}, QueryPlan[K, O]{}, fmt.Errorf("snapshot: query extension changed publication anchor")
	}
	plan, opened := OpenQuery[K, O](&sealed, family)
	if !opened {
		return Snapshot{}, QueryPlan[K, O]{}, fmt.Errorf("snapshot: query extension was not addressable")
	}
	return sealed, plan, nil
}
