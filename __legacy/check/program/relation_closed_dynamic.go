package program

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// inferRelationClosedDynamicInvariants proves the closed reverse-map theorem
// directly from the immutable lexical forest. It replaces the historical
// solve-result census: no body execution, abstract state, or second fixpoint
// participates. A candidate survives iff every non-delete writer stores a
// value whose path was first used as a key of the same primary table set, and
// both roots have certified fresh-empty initializers in the forest.
type relationClosedDynamicInvariants map[lexicalidentity.StableLexicalBodyID][]factapply.ClosedDynamicAllValueInvariant

func inferRelationClosedDynamicInvariants(reg *axis.Registry, prepared preparedBodies) relationClosedDynamicInvariants {
	if reg == nil {
		return nil
	}
	statics := relationProgramStatics(prepared)
	fresh := make(map[pathdom.PathKey]struct{})
	freshByBody := make(map[lexicalidentity.StableLexicalBodyID]map[pathdom.PathKey]struct{}, len(statics))
	for _, static := range statics {
		if static == nil || static.Graph() == nil || static.OperationPlan() == nil {
			continue
		}
		facts, graph := static.OperationPlan().Facts(), static.Graph()
		for raw := 0; raw < graph.Size(); raw++ {
			if assignment, ok := facts.RootAssignment(cfg.Point(raw)); ok && relationFreshEmptyRoot(facts, assignment) {
				key := pathdom.Path{Symbol: assignment.TargetSymbol()}.Key()
				fresh[key] = struct{}{}
				owned := freshByBody[static.StableLexicalBodyID()]
				if owned == nil {
					owned = make(map[pathdom.PathKey]struct{})
					freshByBody[static.StableLexicalBodyID()] = owned
				}
				owned[key] = struct{}{}
			}
		}
	}
	acc := make(map[pathdom.PathKey]*relationClosedDynamicAccumulator)
	for _, static := range statics {
		inferRelationClosedDynamicBodyWriters(reg, static, acc)
	}
	invariants := finalizeRelationClosedDynamicInvariants(acc, fresh)
	out := make(relationClosedDynamicInvariants)
	for _, static := range statics {
		if static == nil {
			continue
		}
		owned := freshByBody[static.StableLexicalBodyID()]
		for _, invariant := range invariants {
			if _, container := owned[invariant.Container.Key()]; !container {
				continue
			}
			if _, table := owned[invariant.Table.Key()]; !table {
				continue
			}
			out[static.StableLexicalBodyID()] = append(out[static.StableLexicalBodyID()], invariant)
		}
	}
	return out
}

type relationClosedDynamicAccumulator struct {
	container   pathdom.Path
	tables      map[pathdom.PathKey]pathdom.Path
	sites       map[dynamicindex.Site]struct{}
	initialized bool
	unsafe      bool
}

func inferRelationClosedDynamicBodyWriters(reg *axis.Registry, static *body.Static, acc map[pathdom.PathKey]*relationClosedDynamicAccumulator) {
	if reg == nil || static == nil || static.Graph() == nil || static.OperationPlan() == nil {
		return
	}
	plan, graph := static.OperationPlan(), static.Graph()
	facts := plan.Facts()
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	observe := func(point cfg.Point, container, valuePath pathdom.Path, site dynamicindex.Site) {
		candidate := acc[container.Key()]
		if candidate == nil {
			candidate = &relationClosedDynamicAccumulator{container: container, sites: make(map[dynamicindex.Site]struct{})}
			acc[container.Key()] = candidate
		}
		if site != "" {
			candidate.sites[site] = struct{}{}
		}
		tables := relationDominatingPrimaryTables(facts, plan, graph, dom, point, valuePath)
		if len(tables) == 0 {
			candidate.unsafe = true
			return
		}
		if !candidate.initialized {
			candidate.tables, candidate.initialized = tables, true
			return
		}
		for key := range candidate.tables {
			if _, present := tables[key]; !present {
				delete(candidate.tables, key)
			}
		}
		candidate.unsafe = len(candidate.tables) == 0
	}
	for _, point := range graph.RPO() {
		if write, ok := facts.DynamicIndexWrite(point); ok {
			container, hasContainer := relationSymbolRootPath(write.TablePath())
			if hasContainer && !relationDynamicWriteDeletes(reg, facts, write) {
				valuePath, hasValue := relationDynamicWriteValuePath(facts, write)
				if !hasValue {
					candidate := acc[container.Key()]
					if candidate == nil {
						candidate = &relationClosedDynamicAccumulator{container: container, sites: make(map[dynamicindex.Site]struct{})}
						acc[container.Key()] = candidate
					}
					candidate.unsafe = true
				} else {
					observe(point, container, valuePath, dynamicindex.SiteForPoint(int(point)))
				}
			}
		}
		for _, write := range relationSignatureTableMutatorWrites(plan, facts, point) {
			if !write.valueOK {
				candidate := acc[write.container.Key()]
				if candidate == nil {
					candidate = &relationClosedDynamicAccumulator{container: write.container, sites: make(map[dynamicindex.Site]struct{})}
					acc[write.container.Key()] = candidate
				}
				candidate.unsafe = true
				continue
			}
			observe(point, write.container, write.value, write.site)
		}
	}
	return
}

func finalizeRelationClosedDynamicInvariants(acc map[pathdom.PathKey]*relationClosedDynamicAccumulator, fresh map[pathdom.PathKey]struct{}) []factapply.ClosedDynamicAllValueInvariant {
	containerKeys := make([]pathdom.PathKey, 0, len(acc))
	for key := range acc {
		containerKeys = append(containerKeys, key)
	}
	sort.Slice(containerKeys, func(i, j int) bool { return containerKeys[i] < containerKeys[j] })
	var out []factapply.ClosedDynamicAllValueInvariant
	for _, containerKey := range containerKeys {
		candidate := acc[containerKey]
		if candidate == nil || candidate.unsafe || !candidate.initialized {
			continue
		}
		if _, ok := fresh[candidate.container.Key()]; !ok {
			continue
		}
		tableKeys := make([]pathdom.PathKey, 0, len(candidate.tables))
		for key := range candidate.tables {
			if _, ok := fresh[key]; ok {
				tableKeys = append(tableKeys, key)
			}
		}
		sort.Slice(tableKeys, func(i, j int) bool { return tableKeys[i] < tableKeys[j] })
		for _, tableKey := range tableKeys {
			sites := make([]dynamicindex.Site, 0, len(candidate.sites))
			for site := range candidate.sites {
				sites = append(sites, site)
			}
			sort.Slice(sites, func(i, j int) bool { return sites[i] < sites[j] })
			if len(sites) == 0 {
				out = append(out, factapply.ClosedDynamicAllValueInvariant{Container: candidate.container, Table: candidate.tables[tableKey]})
			}
			for _, site := range sites {
				out = append(out, factapply.ClosedDynamicAllValueInvariant{Container: candidate.container, Table: candidate.tables[tableKey], Site: site})
			}
		}
	}
	return out
}

func relationProgramStatics(prepared preparedBodies) []*body.Static {
	out := make([]*body.Static, 0, 1+len(prepared.functions))
	if prepared.root != nil {
		out = append(out, prepared.root)
	}
	for _, static := range prepared.functions {
		out = append(out, static)
	}
	return out
}

func relationFreshEmptyRoot(facts factflow.Facts, assignment factflow.RootAssignment) bool {
	if assignment.TargetSymbol() == 0 || assignment.Kind() != factflow.RootAssignmentLocalDeclaration {
		return false
	}
	source := assignment.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	literal, ok := facts.ObjectLiteralView(source.ExprRef)
	return ok && literal.EntryCount() == 0
}

func relationSymbolRootPath(path pathdom.Path) (pathdom.Path, bool) {
	return pathdom.Path{Symbol: path.Symbol}, path.Symbol != 0 && path.Version == 0 && len(path.Segments) == 0
}

func relationDynamicWriteDeletes(reg *axis.Registry, facts factflow.Facts, write factflow.DynamicIndexWrite) bool {
	source := write.Source()
	if source.Kind == factflow.ValueSourceNil {
		return true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	return ok && presence.Equal(product.PresenceOf(value), presence.Absent())
}

func relationDynamicWriteValuePath(facts factflow.Facts, write factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if value, ok := write.ValuePath(); ok && value.Symbol != 0 {
		return value, true
	}
	return relationExpressionSourcePath(facts, write.Source())
}

func relationDynamicWriteKeyPath(facts factflow.Facts, write factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if key, ok := write.KeyPath(); ok && key.Symbol != 0 {
		return key, true
	}
	return relationExpressionSourcePath(facts, write.KeySource())
}

func relationExpressionSourcePath(facts factflow.Facts, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return pathdom.Path{}, false
	}
	path, ok := facts.ExpressionPathRef(source.ExprRef)
	return path, ok && path.Symbol != 0
}

func relationDominatingPrimaryTables(facts factflow.Facts, plan interface {
	GenericForOperation(cfg.Point) (factapply.GenericForOperation, bool)
}, graph cfg.Graph, dom *dominance.ImmediateDominators, point cfg.Point, valuePath pathdom.Path) map[pathdom.PathKey]pathdom.Path {
	if graph == nil || dom == nil || plan == nil || valuePath.Symbol == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]pathdom.Path)
	for _, candidate := range graph.RPO() {
		if !dom.StrictlyDominates(candidate, point) {
			continue
		}
		if write, ok := facts.DynamicIndexWrite(candidate); ok {
			keyPath, hasKey := relationDynamicWriteKeyPath(facts, write)
			if hasKey && keyPath.Equal(valuePath) {
				if table, hasTable := relationSymbolRootPath(write.TablePath()); hasTable {
					out[table.Key()] = table
				}
			}
		}
		generic, ok := plan.GenericForOperation(candidate)
		if !ok || generic.VariableIndex() != 0 || generic.Target() != valuePath.Symbol || len(valuePath.Segments) != 0 {
			continue
		}
		if table, ok := relationGenericForPrimaryTable(facts, generic); ok {
			out[table.Key()] = table
		}
	}
	return out
}

type relationTableMutatorWrite struct {
	container pathdom.Path
	value     pathdom.Path
	site      dynamicindex.Site
	valueOK   bool
}

func relationSignatureTableMutatorWrites(plan interface {
	SignatureCallOperation(cfg.Point) (operationplan.SignatureCallOperation, bool)
}, facts factflow.Facts, point cfg.Point) []relationTableMutatorWrite {
	op, ok := plan.SignatureCallOperation(point)
	if !ok {
		return nil
	}
	sig := op.Signature()
	if !sig.Effect.IsClosed() {
		return nil
	}
	site, ok := facts.CallSiteView(point)
	if !ok {
		return nil
	}
	var out []relationTableMutatorWrite
	for _, label := range sig.Effect.Labels {
		mutator, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		if !ok {
			continue
		}
		targetIndex, targetOK := effect.ResolveParamIndex(mutator.Target, site.ArgumentSourceCount())
		valueIndex, valueOK := effect.ResolveParamIndex(mutator.Value, site.ArgumentSourceCount())
		if !targetOK || !valueOK {
			continue
		}
		targetSource, targetOK := site.ArgumentSourceAt(targetIndex)
		valueSource, valueOK := site.ArgumentSourceAt(valueIndex)
		container, targetOK := relationValueSourcePath(facts, targetSource)
		container, rootOK := relationSymbolRootPath(container)
		if targetOK && rootOK {
			value, valueOK := relationValueSourcePath(facts, valueSource)
			out = append(out, relationTableMutatorWrite{container: container, value: value, site: effectlowering.TableMutatorDynamicIndexSite(mutator), valueOK: valueOK})
		}
	}
	return out
}

func relationGenericForPrimaryTable(facts factflow.Facts, generic factapply.GenericForOperation) (pathdom.Path, bool) {
	iterator, ok := generic.Iterator()
	if !ok || iterator.Kind != iteration.IterateKeyed {
		return pathdom.Path{}, false
	}
	source, _ := generic.ProtocolSource(0)
	if source.Kind != factapply.GenericForSourceCall || !source.HasCallPoint {
		return pathdom.Path{}, false
	}
	site, ok := facts.CallSiteView(source.CallPoint)
	if !ok {
		return pathdom.Path{}, false
	}
	sourceIndex, ok := effect.ResolveParamIndex(iterator.Source, site.ArgumentSourceCount())
	if !ok {
		return pathdom.Path{}, false
	}
	arg, ok := site.ArgumentSourceAt(sourceIndex)
	if !ok {
		return pathdom.Path{}, false
	}
	return relationValueSourcePath(facts, arg)
}

func relationValueSourcePath(facts factflow.Facts, source factflow.ValueSource) (pathdom.Path, bool) {
	key := relationProgramSourcePath(facts, source)
	if key == "" {
		return pathdom.Path{}, false
	}
	sym, segments, ok := pathaddr.ParseSymbolPathKey(key)
	if !ok || sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: sym, Segments: segments}, true
}

func mergeRelationClosedDynamicInvariants(base, inferred []factapply.ClosedDynamicAllValueInvariant) []factapply.ClosedDynamicAllValueInvariant {
	out := append([]factapply.ClosedDynamicAllValueInvariant(nil), base...)
	for _, candidate := range inferred {
		duplicate := false
		for _, prior := range out {
			if prior.Site == candidate.Site && prior.Container.Equal(candidate.Container) && prior.Table.Equal(candidate.Table) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, candidate)
		}
	}
	return out
}
