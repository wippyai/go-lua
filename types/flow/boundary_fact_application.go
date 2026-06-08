package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// BoundaryLocalPath is the caller- or entry-local identity of a boundary path.
// Boundary application needs both the source path for path-shaped transfer
// transactions and the stable address for address-indexed proof lanes; callers
// normalize that pair once at the boundary.
type BoundaryLocalPath struct {
	path    constraint.Path
	address StableAddress
}

// BoundaryLocalPathOfPath normalizes a concrete local path into the address
// view used by point-state fact domains.
func BoundaryLocalPathOfPath(path constraint.Path) (BoundaryLocalPath, bool) {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return BoundaryLocalPath{}, false
	}
	return BoundaryLocalPath{
		path:    cloneAddressPath(path),
		address: addr,
	}, true
}

// BoundaryPathRebaser maps a boundary-relative fact path to the normalized
// caller or entry point identity where the fact is materialized.
type BoundaryPathRebaser func(BoundaryPath) (BoundaryLocalPath, bool)

// BoundaryAppendKeyPlan captures append-key replay decisions that must be made
// from the pre-mutation point state and applied after other call side effects.
type BoundaryAppendKeyPlan struct {
	array               constraint.Path
	key                 constraint.Path
	table               constraint.Path
	hasTable            bool
	writtenTables       []constraint.Path
	freshEmpty          bool
	preserveHistoryBase bool
}

// BoundaryFactApplication reports non-flow consequences produced while applying
// boundary facts. Flow owns the fact transactions; transfer owns materializing
// these results into its refinement effect vocabulary.
type BoundaryFactApplication struct {
	KeyProvenance []KeyProvenanceResult
}

// BoundaryAppendKeyPlans selects append-key replay plans from the current
// caller state before side effects can invalidate the evidence used to preserve
// key-array provenance.
func BoundaryAppendKeyPlans(state PointState, facts BoundaryFacts, rebase BoundaryPathRebaser) []BoundaryAppendKeyPlan {
	if !facts.HasProof() || rebase == nil {
		return nil
	}
	appendKeys := facts.AppendKeys()
	if len(appendKeys) == 0 {
		return nil
	}
	var plans []BoundaryAppendKeyPlan
	for _, fact := range appendKeys {
		array, ok := rebase(fact.Array)
		if !ok || array.path.IsEmpty() {
			continue
		}
		key, ok := rebase(fact.Key)
		if !ok || key.path.IsEmpty() {
			continue
		}
		factsView := PointFactsOf(state)
		plan := BoundaryAppendKeyPlan{
			array:               array.path,
			key:                 key.path,
			freshEmpty:          AppendFreshEmptySeedPath(state, array.path),
			preserveHistoryBase: factsView.HasAppendHistoryBase(array.path),
		}
		if fact.HasTable {
			table, ok := rebase(fact.Table)
			if !ok || table.path.IsEmpty() {
				continue
			}
			plan.table = table.path
			plan.hasTable = true
		}
		plan.writtenTables = boundaryIndexWriteTablesForAppendedKey(facts.IndexWrites(), rebase, key.address, plan.table, plan.hasTable)
		plans = append(plans, plan)
	}
	return plans
}

func boundaryIndexWriteTablesForAppendedKey(
	indexWrites []BoundaryIndexWriteFact,
	rebase BoundaryPathRebaser,
	key StableAddress,
	explicitTable constraint.Path,
	hasExplicitTable bool,
) []constraint.Path {
	if rebase == nil || key.Key() == "" || len(indexWrites) == 0 {
		return nil
	}
	var tables []constraint.Path
	for _, fact := range indexWrites {
		writeKey, ok := rebase(fact.Key)
		if !ok || !writeKey.address.Equal(key) {
			continue
		}
		table, ok := rebase(fact.Table)
		if !ok || table.path.IsEmpty() {
			continue
		}
		if hasExplicitTable && !table.path.Equal(explicitTable) {
			continue
		}
		tables = append(tables, table.path)
	}
	return tables
}

// ApplyBoundaryFacts applies a finite boundary postcondition stream to a point
// state in the same reduced-product order as local fact publication.
func ApplyBoundaryFacts(
	out *PointState,
	facts BoundaryFacts,
	rebase BoundaryPathRebaser,
	appendPlans []BoundaryAppendKeyPlan,
) (BoundaryFactApplication, bool) {
	if out == nil || rebase == nil || !facts.HasProof() {
		return BoundaryFactApplication{}, false
	}
	var result BoundaryFactApplication
	changed := false
	for _, fact := range facts.IndexWrites() {
		table, ok := rebase(fact.Table)
		if !ok {
			continue
		}
		key, ok := rebase(fact.Key)
		if !ok {
			continue
		}
		changed = ApplyMapWritePathTransaction(out, MapWritePathTransaction{
			TablePath:              table.path,
			KeyPath:                key.path,
			KeyValue:               product.FromType(typ.Unknown),
			Value:                  fact.Value,
			AllowOpaqueKeyReadback: true,
		}) || changed
	}
	for _, fact := range facts.KeyPresence() {
		table, ok := rebase(fact.Table)
		if !ok {
			continue
		}
		key, ok := rebase(fact.Key)
		if !ok {
			continue
		}
		txResult, txChanged := ApplyKeyProvenancePathTransaction(out, KeyProvenancePathTransaction{
			Kind:      KeyProvenanceDynamicIndexWrite,
			TablePath: table.path,
			KeyPath:   key.path,
		})
		result = appendBoundaryKeyProvenanceResult(result, txResult)
		changed = txChanged || changed
	}
	for _, fact := range facts.KeyArrays() {
		array, ok := rebase(fact.Array)
		if !ok {
			continue
		}
		table, ok := rebase(fact.Table)
		if !ok {
			continue
		}
		txResult, txChanged := ApplyKeyProvenancePathTransaction(out, KeyProvenancePathTransaction{
			Kind:      KeyProvenanceKeyArrayAssignment,
			ArrayPath: array.path,
			TablePath: table.path,
		})
		result = appendBoundaryKeyProvenanceResult(result, txResult)
		changed = txChanged || changed
	}
	for _, fact := range facts.KeyArrayValues() {
		array, ok := rebase(fact.Array)
		if !ok {
			continue
		}
		table, ok := rebase(fact.Table)
		if !ok {
			continue
		}
		changed = ApplyKeyArrayValueProof(out, KeyArrayValueProof{
			Array: array.address,
			Table: table.address,
			Value: fact.Value,
		}) || changed
	}
	changed = applyBoundaryAppendKeyPlans(out, appendPlans) || changed
	for _, fact := range facts.AppendElementFieldOrigins() {
		array, ok := rebase(fact.Array)
		if !ok {
			continue
		}
		source, ok := rebase(fact.Source)
		if !ok {
			continue
		}
		changed = ApplyAppendElementFieldOriginProof(out, AppendElementFieldOriginProof{
			Array:       array.address,
			Field:       fact.Field,
			Source:      source.address,
			SourceField: fact.SourceField,
		}) || changed
	}
	var ops []NumericOp
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := rebase(fact.Target)
		if !ok {
			continue
		}
		if op, ok := NumericLenGeConstPathOp(target.path, fact.Lower); ok {
			ops = append(ops, op)
		}
	}
	if len(ops) > 0 {
		changed = ApplyNumericEffect(out, NumericEffect{Ops: ops}) || changed
	}
	return result, changed
}

func appendBoundaryKeyProvenanceResult(app BoundaryFactApplication, result KeyProvenanceResult) BoundaryFactApplication {
	if result.KeyRefinementValue.IsZero() || result.KeyRefinementPath.Symbol == 0 {
		return app
	}
	app.KeyProvenance = append(app.KeyProvenance, result)
	return app
}

func applyBoundaryAppendKeyPlans(out *PointState, plans []BoundaryAppendKeyPlan) bool {
	if out == nil || len(plans) == 0 {
		return false
	}
	changed := false
	for _, plan := range plans {
		changed = ApplyAppendKeyReplayPathTransaction(out, AppendKeyReplayPathTransaction{
			ArrayPath:           plan.array,
			KeyPath:             plan.key,
			ExplicitTablePath:   plan.table,
			HasExplicitTable:    plan.hasTable,
			WrittenTablePaths:   plan.writtenTables,
			FreshEmpty:          plan.freshEmpty,
			PreserveHistoryBase: plan.preserveHistoryBase,
		}) || changed
	}
	return changed
}
