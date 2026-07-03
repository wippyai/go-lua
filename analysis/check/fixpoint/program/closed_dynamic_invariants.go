package program

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/compiler/ast"
)

type closedDynamicAllValueInvariant = factapply.ClosedDynamicAllValueInvariant

type closedDynamicAllValueAccumulator struct {
	tables      map[pathdom.PathKey]pathdom.Path
	initialized bool
	unsafe      bool
}

func applyClosedDynamicAllValueEntryStates(
	keys *programKeys,
	prepared preparedBodies,
	reg *axis.Registry,
	root *body.Result,
	results map[*ast.FunctionExpr]*body.Result,
) {
	if keys == nil || reg == nil || len(results) == 0 {
		return
	}
	candidates := inferClosedDynamicAllValueInvariants(reg, results)
	if len(candidates) == 0 {
		return
	}
	keys.closedDynamicAllValues = append([]factapply.ClosedDynamicAllValueInvariant(nil), candidates...)
	invariants := filterClosedDynamicAllValueEntryInvariants(reg, root, results, candidates)
	if len(invariants) == 0 {
		return
	}
	for i := range keys.functions {
		static := prepared.function(keys.functions[i].funcExpr)
		keys.functions[i] = addClosedDynamicAllValueEntryState(keys.functions[i], static, invariants)
	}
	keys.contexts.TransformEntries(func(context keyedFunction) keyedFunction {
		static := prepared.function(context.funcExpr)
		return addClosedDynamicAllValueEntryState(context, static, invariants)
	})
}

func addClosedDynamicAllValueEntryState(fn keyedFunction, static *body.Static, invariants []closedDynamicAllValueInvariant) keyedFunction {
	entryKeys := fn.entryKeys
	if entryKeys == nil && static != nil {
		entryKeys = static.KeySpace()
	}
	if entryKeys == nil {
		return fn
	}
	entry := fn.entryState
	changed := false
	for _, invariant := range invariants {
		var ok bool
		entry, ok = addClosedDynamicAllValueInvariant(entryKeys, entry, invariant)
		changed = changed || ok
	}
	if !changed {
		return fn
	}
	fn.entryState = entry
	fn.entryKeys = entryKeys
	fn.hasEntryState = true
	return fn
}

func addClosedDynamicAllValueInvariant(ks *keyspace.KeySpace, entry state.State, invariant closedDynamicAllValueInvariant) (state.State, bool) {
	containerKey, ok := symbolRootKey(ks, invariant.Container)
	if !ok {
		return entry, false
	}
	tableKey, ok := symbolRootStateKey(ks, invariant.Table)
	if !ok {
		return entry, false
	}
	before := entry.DynamicIndexAllValuesKeyMembershipTables(containerKey)
	out := entry.AddDynamicIndexAllValuesKeyMembership(containerKey, tableKey)
	after := out.DynamicIndexAllValuesKeyMembershipTables(containerKey)
	return out, len(after) != len(before)
}

func inferClosedDynamicAllValueInvariants(reg *axis.Registry, results map[*ast.FunctionExpr]*body.Result) []closedDynamicAllValueInvariant {
	acc := make(map[pathdom.PathKey]*closedDynamicAllValueAccumulator)
	for _, result := range results {
		collectClosedDynamicAllValueInvariantsFromResult(reg, result, acc)
	}
	out := make([]closedDynamicAllValueInvariant, 0, len(acc))
	for _, containerKey := range sortedPathKeys(acc) {
		candidate := acc[containerKey]
		if candidate == nil || candidate.unsafe || !candidate.initialized || len(candidate.tables) == 0 {
			continue
		}
		container := candidateContainerPath(containerKey, results)
		if container.IsEmpty() {
			continue
		}
		for _, tableKey := range sortedTableKeys(candidate.tables) {
			out = append(out, closedDynamicAllValueInvariant{
				Container: container,
				Table:     candidate.tables[tableKey],
			})
		}
	}
	return out
}

func filterClosedDynamicAllValueEntryInvariants(
	reg *axis.Registry,
	root *body.Result,
	results map[*ast.FunctionExpr]*body.Result,
	in []closedDynamicAllValueInvariant,
) []closedDynamicAllValueInvariant {
	if len(in) == 0 {
		return nil
	}
	out := make([]closedDynamicAllValueInvariant, 0, len(in))
	for _, invariant := range in {
		if containerHasFreshEmptyTable(reg, root, results, invariant.Container) {
			out = append(out, invariant)
		}
	}
	return out
}

func collectClosedDynamicAllValueInvariantsFromResult(reg *axis.Registry, result *body.Result, acc map[pathdom.PathKey]*closedDynamicAllValueAccumulator) {
	if reg == nil || result == nil || result.Graph() == nil || result.KeySpace() == nil {
		return
	}
	dom := dominance.ComputeImmediateDominatorInfo(result.Graph())
	for _, point := range result.Graph().RPO() {
		write, ok := result.DynamicIndexWrite(point)
		if !ok {
			continue
		}
		containerPath, ok := symbolRootPath(write.TablePath())
		if !ok {
			continue
		}
		containerKey := containerPath.Key()
		candidate := acc[containerKey]
		if candidate == nil {
			candidate = &closedDynamicAllValueAccumulator{}
			acc[containerKey] = candidate
		}
		if dynamicWriteDeletesValue(reg, result, write) {
			continue
		}
		valuePath, ok := dynamicWriteValuePath(result, write)
		if !ok {
			candidate.unsafe = true
			continue
		}
		tables := dominatingPrimaryWriteTables(result, dom, point, valuePath)
		if len(tables) == 0 {
			candidate.unsafe = true
			continue
		}
		candidate.observe(tables)
	}
}

func (a *closedDynamicAllValueAccumulator) observe(tables map[pathdom.PathKey]pathdom.Path) {
	if !a.initialized {
		a.tables = clonePathMap(tables)
		a.initialized = true
		return
	}
	for key := range a.tables {
		if _, ok := tables[key]; !ok {
			delete(a.tables, key)
		}
	}
	if len(a.tables) == 0 {
		a.unsafe = true
	}
}

func dominatingPrimaryWriteTables(result *body.Result, dom *dominance.ImmediateDominators, point cfg.Point, valuePath pathdom.Path) map[pathdom.PathKey]pathdom.Path {
	if result == nil || result.Graph() == nil || dom == nil || valuePath.IsEmpty() {
		return nil
	}
	out := make(map[pathdom.PathKey]pathdom.Path)
	for _, candidate := range result.Graph().RPO() {
		if candidate == point {
			break
		}
		if !dom.StrictlyDominates(candidate, point) {
			continue
		}
		prior, ok := result.DynamicIndexWrite(candidate)
		if !ok {
			continue
		}
		keyPath, ok := dynamicWriteKeyPath(result, prior)
		if !ok || !keyPath.Equal(valuePath) {
			continue
		}
		table, ok := symbolRootPath(prior.TablePath())
		if !ok {
			continue
		}
		out[table.Key()] = table
	}
	return out
}

func dynamicWriteKeyPath(result *body.Result, write factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if keyPath, ok := write.KeyPath(); ok && keyPath.Symbol != 0 {
		return keyPath, true
	}
	return dynamicWriteSourceExpressionPath(result, write.KeySource())
}

func dynamicWriteValuePath(result *body.Result, write factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if valuePath, ok := write.ValuePath(); ok && valuePath.Symbol != 0 {
		return valuePath, true
	}
	return dynamicWriteSourceExpressionPath(result, write.Source())
}

func dynamicWriteSourceExpressionPath(result *body.Result, source factflow.ValueSource) (pathdom.Path, bool) {
	if result == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return pathdom.Path{}, false
	}
	sourcePath, ok := result.ExpressionPathRef(source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return sourcePath, true
}

func dynamicWriteDeletesValue(reg *axis.Registry, result *body.Result, write factflow.DynamicIndexWrite) bool {
	source := write.Source()
	if source.Kind == factflow.ValueSourceNil {
		return true
	}
	if reg == nil || result == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	value, ok := result.ExpressionValueRef(source.ExprRef)
	return ok && presence.Equal(product.PresenceOf(value), presence.Absent())
}

func containerHasFreshEmptyTable(reg *axis.Registry, root *body.Result, results map[*ast.FunctionExpr]*body.Result, p pathdom.Path) bool {
	if rootHasFreshEmptyTable(reg, root, p) {
		return true
	}
	for _, result := range results {
		if rootHasFreshEmptyTable(reg, result, p) {
			return true
		}
	}
	return false
}

func rootHasFreshEmptyTable(reg *axis.Registry, root *body.Result, p pathdom.Path) bool {
	if reg == nil || root == nil || p.Symbol == 0 || len(p.Segments) != 0 {
		return false
	}
	st, ok := root.ExitState()
	if !ok {
		return false
	}
	value := st.ReadSymbolValue(reg, p.Symbol)
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return false
	}
	object := st.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return false
	}
	return len(object.StaticMembers()) == 0 && len(object.DynamicIndexFacts()) == 0
}

func symbolRootPath(p pathdom.Path) (pathdom.Path, bool) {
	if p.Symbol == 0 || len(p.Segments) != 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: p.Symbol}, true
}

func symbolRootPathFromKey(ks *keyspace.KeySpace, key keyspace.Key) (pathdom.Path, bool) {
	if ks == nil || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	segments, ok := ks.SegmentsView(key)
	if !ok || len(segments) != 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: key.Sym}, true
}

func symbolRootPathFromStateKey(ks *keyspace.KeySpace, key pathaddr.StateKey) (pathdom.Path, bool) {
	if ks == nil || key == "" {
		return pathdom.Path{}, false
	}
	structural, ok := ks.FromStateKey(key.PathKey())
	if !ok {
		return pathdom.Path{}, false
	}
	return symbolRootPathFromKey(ks, structural)
}

func symbolRootKey(ks *keyspace.KeySpace, p pathdom.Path) (keyspace.Key, bool) {
	root, ok := symbolRootPath(p)
	if !ok || ks == nil {
		return keyspace.Key{}, false
	}
	key := ks.FromPath(root)
	return key, key.Kind != keyspace.KindInvalid
}

func symbolRootStateKey(ks *keyspace.KeySpace, p pathdom.Path) (pathaddr.StateKey, bool) {
	key, ok := symbolRootKey(ks, p)
	if !ok {
		return "", false
	}
	return pathaddr.StateKeyFromPathKey(ks.Format(key))
}

func clonePathMap(in map[pathdom.PathKey]pathdom.Path) map[pathdom.PathKey]pathdom.Path {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]pathdom.Path, len(in))
	for key, value := range in {
		out[key] = value.Clone()
	}
	return out
}

func sortedPathKeys(in map[pathdom.PathKey]*closedDynamicAllValueAccumulator) []pathdom.PathKey {
	out := make([]pathdom.PathKey, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedTableKeys(in map[pathdom.PathKey]pathdom.Path) []pathdom.PathKey {
	out := make([]pathdom.PathKey, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func candidateContainerPath(key pathdom.PathKey, results map[*ast.FunctionExpr]*body.Result) pathdom.Path {
	for _, result := range results {
		if result == nil || result.KeySpace() == nil {
			continue
		}
		stateKey, ok := pathaddr.StateKeyFromPathKey(key)
		if !ok {
			continue
		}
		path, ok := symbolRootPathFromStateKey(result.KeySpace(), stateKey)
		if ok {
			return path
		}
	}
	if local, ok := pathaddr.LocalPathFromKey(key); ok && local.Symbol != 0 && len(local.Segments) == 0 {
		return pathdom.Path{Symbol: local.Symbol}
	}
	return pathdom.Path{}
}
