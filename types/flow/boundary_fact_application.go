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

// BoundaryLocalRoots names the concrete caller- or entry-local roots available
// for a boundary fact application. It keeps root selection separate from fact
// replay, while flow owns the boundary-path rebasing and normalization rules.
type BoundaryLocalRoots struct {
	params  map[int]constraint.Path
	returns map[int]constraint.Path
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

// NewBoundaryLocalRoots copies concrete local roots for parameter and return
// boundary paths. Invalid/empty roots are ignored.
func NewBoundaryLocalRoots(params, returns map[int]constraint.Path) BoundaryLocalRoots {
	return BoundaryLocalRoots{
		params:  cloneBoundaryLocalRootPaths(params),
		returns: cloneBoundaryLocalRootPaths(returns),
	}
}

// Rebase maps a boundary-relative path to its concrete local path and stable
// address. Callers supply only root identities; suffix composition is part of
// the boundary algebra.
func (r BoundaryLocalRoots) Rebase(path BoundaryPath) (BoundaryLocalPath, bool) {
	var root constraint.Path
	var ok bool
	switch path.Kind {
	case BoundaryPathParam:
		root, ok = r.params[path.Index]
	case BoundaryPathReturn:
		root, ok = r.returns[path.Index]
	default:
		return BoundaryLocalPath{}, false
	}
	if !ok || root.IsEmpty() {
		return BoundaryLocalPath{}, false
	}
	for _, seg := range path.Segments {
		root = root.Append(seg)
	}
	return BoundaryLocalPathOfPath(root)
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

// BoundaryFactApplication reports transfer-local consequences produced while
// applying boundary facts. Flow applies point-state fact lanes itself; only
// refinements outside the flow vocabulary are returned to transfer.
type BoundaryFactApplication struct {
	KeyProvenance   []KeyProvenanceResult
	LengthRelations []BoundaryLengthRelationApplication
}

// BoundaryFactReplayPlan records boundary replay decisions that must be made
// before a call mutates caller state. Flow owns these prestate facts because
// they are part of the boundary algebra, not transfer-site control flow.
type BoundaryFactReplayPlan struct {
	appendKeys     []BoundaryAppendKeyPlan
	prestateLowers []boundaryLengthLowerApplication
}

type boundaryLengthLowerApplication struct {
	Target constraint.Path
	Lower  int64
}

// BoundaryLengthRelationApplication is the caller-local form of a boundary
// length relation after boundary paths have been rebased.
type BoundaryLengthRelationApplication struct {
	Target constraint.Path
	Source constraint.Path
}

func boundaryFactPrestateLengthLowers(
	state PointState,
	facts BoundaryFacts,
	rebase BoundaryPathRebaser,
) []boundaryLengthLowerApplication {
	if rebase == nil || !facts.HasProof() {
		return nil
	}
	var lowers []boundaryLengthLowerApplication
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := rebase(fact.Target)
		if !ok || target.path.IsEmpty() || fact.Lower <= 0 {
			continue
		}
		lowers = append(lowers, boundaryLengthLowerApplication{
			Target: target.path,
			Lower:  fact.Lower,
		})
	}
	for _, fact := range facts.LengthRelations() {
		target, ok := rebase(fact.Target)
		if !ok || target.path.IsEmpty() {
			continue
		}
		source, ok := rebase(fact.Source)
		if !ok || source.path.IsEmpty() {
			continue
		}
		if lower := boundaryLengthRelationSourceLower(state, source.path); lower > 0 {
			lowers = append(lowers, boundaryLengthLowerApplication{
				Target: target.path,
				Lower:  lower,
			})
		}
	}
	return lowers
}

// PrepareBoundaryFactReplay captures all prestate-sensitive replay decisions for
// applying facts through roots.
func PrepareBoundaryFactReplay(state PointState, facts BoundaryFacts, roots BoundaryLocalRoots) BoundaryFactReplayPlan {
	if !facts.HasProof() {
		return BoundaryFactReplayPlan{}
	}
	return BoundaryFactReplayPlan{
		appendKeys:     BoundaryAppendKeyPlans(state, facts, roots.Rebase),
		prestateLowers: boundaryFactPrestateLengthLowers(state, facts, roots.Rebase),
	}
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
		changed = ApplyDynamicIndexWritePathTransaction(out, DynamicIndexWritePathTransaction{
			TablePath:     table.path,
			KeyPath:       key.path,
			KeyValue:      product.FromType(typ.Unknown),
			WrittenValue:  fact.Value,
			ReadbackValue: fact.Value,
		}) || changed
	}
	for _, fact := range facts.StaticMembers() {
		target, ok := rebase(fact.Target)
		if !ok || fact.Value.IsZero() {
			continue
		}
		changed = SetStaticMemberFact(out, target.address, fact.Value) || changed
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
	for _, fact := range facts.LengthRelations() {
		target, ok := rebase(fact.Target)
		if !ok || target.path.IsEmpty() {
			continue
		}
		source, ok := rebase(fact.Source)
		if !ok || source.path.IsEmpty() {
			continue
		}
		result.LengthRelations = append(result.LengthRelations, BoundaryLengthRelationApplication{
			Target: target.path,
			Source: source.path,
		})
		if lower := boundaryLengthRelationSourceLower(*out, source.path); lower > 0 {
			if op, ok := NumericLenGeConstPathOp(target.path, lower); ok {
				changed = ApplyNumericEffect(out, NumericEffect{Ops: []NumericOp{op}}) || changed
			}
		}
	}
	return result, changed
}

// ApplyBoundaryFactsWithReplay applies facts with replay decisions captured before
// caller-side mutation. The returned application contains only consequences that
// still require transfer-local materialization.
func ApplyBoundaryFactsWithReplay(
	out *PointState,
	facts BoundaryFacts,
	roots BoundaryLocalRoots,
	plan BoundaryFactReplayPlan,
) (BoundaryFactApplication, bool) {
	app, changed := ApplyBoundaryFacts(out, facts, roots.Rebase, plan.appendKeys)
	changed = applyBoundaryLengthLowerApplications(out, plan.prestateLowers) || changed
	return app, changed
}

func boundaryLengthRelationSourceLower(state PointState, source constraint.Path) int64 {
	ref, ok := ContainerRefOfPath(source)
	if !ok {
		return 0
	}
	lower := int64(0)
	if state.Num != nil {
		if numericLower, _, ok := NumericLenBoundsForContainer(state.Num, ref); ok && numericLower > lower {
			lower = numericLower
		}
	}
	if relationLower, ok := state.Rel.ContainerLowerBoundForRef(ref); ok && relationLower > lower {
		lower = relationLower
	}
	return lower
}

func appendBoundaryKeyProvenanceResult(app BoundaryFactApplication, result KeyProvenanceResult) BoundaryFactApplication {
	if result.KeyRefinementValue.IsZero() || result.KeyRefinementPath.Symbol == 0 {
		return app
	}
	app.KeyProvenance = append(app.KeyProvenance, result)
	return app
}

func applyBoundaryLengthLowerApplications(out *PointState, lowers []boundaryLengthLowerApplication) bool {
	if out == nil || len(lowers) == 0 {
		return false
	}
	ops := make([]NumericOp, 0, len(lowers))
	for _, lower := range lowers {
		if op, ok := NumericLenGeConstPathOp(lower.Target, lower.Lower); ok {
			ops = append(ops, op)
		}
	}
	if len(ops) == 0 {
		return false
	}
	return ApplyNumericEffect(out, NumericEffect{Ops: ops})
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

func cloneBoundaryLocalRootPaths(in map[int]constraint.Path) map[int]constraint.Path {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]constraint.Path, len(in))
	for idx, path := range in {
		if idx < 0 || path.IsEmpty() {
			continue
		}
		out[idx] = cloneAddressPath(path)
	}
	return out
}
