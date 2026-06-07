package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// BoundaryPathRebaser maps a boundary-relative fact path to the caller or entry
// point path where the fact is materialized.
type BoundaryPathRebaser func(BoundaryPath) (constraint.Path, bool)

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
		if !ok || array.IsEmpty() {
			continue
		}
		key, ok := rebase(fact.Key)
		if !ok || key.IsEmpty() {
			continue
		}
		factsView := PointFactsOf(state)
		plan := BoundaryAppendKeyPlan{
			array:               array,
			key:                 key,
			freshEmpty:          AppendFreshEmptySeedPath(state, array),
			preserveHistoryBase: factsView.HasAppendHistoryBase(array),
		}
		if fact.HasTable {
			table, ok := rebase(fact.Table)
			if !ok || table.IsEmpty() {
				continue
			}
			plan.table = table
			plan.hasTable = true
		}
		plan.writtenTables = boundaryIndexWriteTablesForAppendedKey(facts.IndexWrites(), rebase, key, plan.table, plan.hasTable)
		plans = append(plans, plan)
	}
	return plans
}

func boundaryIndexWriteTablesForAppendedKey(
	indexWrites []BoundaryIndexWriteFact,
	rebase BoundaryPathRebaser,
	key constraint.Path,
	explicitTable constraint.Path,
	hasExplicitTable bool,
) []constraint.Path {
	if rebase == nil || key.IsEmpty() || len(indexWrites) == 0 {
		return nil
	}
	var tables []constraint.Path
	for _, fact := range indexWrites {
		writeKey, ok := rebase(fact.Key)
		if !ok || !writeKey.Equal(key) {
			continue
		}
		table, ok := rebase(fact.Table)
		if !ok || table.IsEmpty() {
			continue
		}
		if hasExplicitTable && !table.Equal(explicitTable) {
			continue
		}
		tables = append(tables, table)
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
			TablePath:              table,
			KeyPath:                key,
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
		proofResult, proofChanged := ApplyKeyProvenancePathProof(out, KeyProvenancePathProof{
			Kind:      KeyProvenanceDynamicIndexWrite,
			TablePath: table,
			KeyPath:   key,
		})
		result = appendBoundaryKeyProvenanceResult(result, proofResult)
		changed = proofChanged || changed
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
		proofResult, proofChanged := ApplyKeyProvenancePathProof(out, KeyProvenancePathProof{
			Kind:      KeyProvenanceKeyArrayAssignment,
			ArrayPath: array,
			TablePath: table,
		})
		result = appendBoundaryKeyProvenanceResult(result, proofResult)
		changed = proofChanged || changed
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
		arrayAddr, arrayOK := StableAddressOfPath(array)
		tableAddr, tableOK := StableAddressOfPath(table)
		if !arrayOK || !tableOK {
			continue
		}
		changed = ApplyKeyArrayValueProof(out, KeyArrayValueProof{
			Array: arrayAddr,
			Table: tableAddr,
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
		arrayAddr, arrayOK := StableAddressOfPath(array)
		sourceAddr, sourceOK := StableAddressOfPath(source)
		if !arrayOK || !sourceOK {
			continue
		}
		changed = ApplyAppendElementFieldOriginProof(out, AppendElementFieldOriginProof{
			Array:       arrayAddr,
			Field:       fact.Field,
			Source:      sourceAddr,
			SourceField: fact.SourceField,
		}) || changed
	}
	var ops []NumericOp
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := rebase(fact.Target)
		if !ok {
			continue
		}
		if op, ok := NumericLenGeConstPathOp(target, fact.Lower); ok {
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
