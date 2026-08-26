// runtime_factor_bind.go runs the per-Factor binding pass: shape matchers, surface ordering and weak targets.

package engine

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
)

// summaryUnitKey is the carrier Unit identity of one declared summary read.
// Two surfaces share a Unit only when they name the same canonical key vector
// under the same declared fold: a coordinate-wise reader and a correlated
// reader of the same keys observe different partitions, so they are distinct
// Units even though their key vectors agree.
type summaryUnitKey struct {
	representative equation.Surface
	distributive   bool
}

type summaryBindingRow[K ~uint32 | ~uint64] struct {
	unit summaryUnitKey
	keys []K
}

// surfaceVectorRow keeps selector vectors positional. Weak rows later sort
// their resolved Units because weak coverage is a set; selector rows never do.
type surfaceVectorRow struct {
	surface    equation.Surface
	candidates []equation.Surface
}

type factorGraphCatalog[K ~uint32 | ~uint64] struct {
	exactReads     []equation.Surface
	summaries      []summaryBindingRow[K]
	summaryAliases map[equation.Surface]summaryUnitKey
	strongWrites   []equation.Surface
	weakWrites     []surfaceVectorRow
	dynamicRead    bool
	routeWrite     bool
	carryTargets   []carryTargetRow
}

type carryTargetRow struct {
	member  composition.Key
	targets []equation.Surface
	route   bool
}

func bindFactorFromGraph[K ~uint32 | ~uint64, V any](implementation *FactorImplementation[K, V], runtime *runtimeBinding) (*boundFactor[K, V], bool) {
	if implementation == nil || !factorRowAvailable(implementation.row) || implementation.algebra == nil || runtime == nil || !runtime.valid() {
		return nil, false
	}
	row := implementation.row
	state := row.schemaFactorBindingState()
	schema := row.schemaFactorSchema()
	semantic := row.schemaFactorSemanticKey()
	if state == nil || schema == nil || state != runtime.state || schema != runtime.schema || row.schemaFactorOrdinal() != implementation.ordinal || row.schemaFactorAlgebra() != implementation.algebra || !semantic.Available() {
		return nil, false
	}
	if runtime.graph.CompositionID() != schema.coldID() {
		return nil, false
	}
	index, indexed := schema.cold.FactorIndex(semantic)
	if !indexed || index != implementation.ordinal {
		return nil, false
	}
	uses, taken := runtime.takeFactorUses(semantic)
	if !taken {
		return nil, false
	}
	catalog, catalogOK := collectFactorGraphCatalog[K, V](implementation, runtime.graph, uses)
	if !catalogOK {
		return nil, false
	}
	bound := &boundFactor[K, V]{
		implementation:  implementation,
		reads:           make(map[equation.Surface]boundUnit, len(catalog.exactReads)+len(catalog.summaryAliases)),
		writes:          make(map[equation.Surface]boundTarget, len(catalog.strongWrites)+len(catalog.weakWrites)),
		carryTargets:    make(map[composition.Key][]carrier.Target, len(catalog.carryTargets)),
		carryRouteScope: make(map[composition.Key]bool, len(catalog.carryTargets)),
	}
	// Source columns are copied exactly once from the cold relation owner into
	// this Program-bound factor.  The solve runtime keeps only immutable values
	// and cannot retain, reopen, or invoke the owner capability.
	if owner := row.schemaFactorRelationOwner(); owner != nil {
		if columns, columnsOK := owner.(memberrelation.SourceColumns[V]); columnsOK {
			count := columns.RelationCount()
			if count < 0 {
				return nil, false
			}
			bound.sourceColumns = make([]memberrelation.SourceColumn[V], count)
			bound.sourcePresent = make([]bool, count)
			for index := 0; index < count; index++ {
				column, present := columns.SourceFactColumn(uint32(index))
				if !present {
					continue
				}
				if !column.Valid() {
					return nil, false
				}
				bound.sourceColumns[index] = column.Clone()
				bound.sourcePresent[index] = true
			}
		}
	}
	// The families rules installed for their own sealed ordinals are claimed
	// here, at seal, beside the materialized columns. A claim is resolved by
	// exactly the Factor it names, and it must be typed in that Factor's key
	// and fact types: a claim that is not is a rule installing a family for a
	// Factor it does not write to.
	//
	// The claim table this reads is the transient composition one: opaque
	// installers keyed by the ordinal they were claimed against. It is resolved
	// exactly here, once, into a table typed in this Factor's own key and fact,
	// and never consulted again - so it is never a second untyped authority
	// over which family a rule has. The typed table is sized from the sealed
	// rule table, because a claim is a position in that table.
	for ruleOrdinal, claim := range runtime.state.ruleFamilies {
		if claim.factor != implementation.ordinal {
			continue
		}
		if ruleOrdinal > uint64(^uint32(0)) {
			return nil, false
		}
		installer, typed := claim.installer.(execution.RuleFamilyInstaller[K, V])
		if !typed {
			return nil, false
		}
		if bound.families == nil {
			families, opened := execution.NewRuleFamilies[K, V](int(schema.ruleCount()))
			if !opened {
				return nil, false
			}
			bound.families = families
		}
		if !bound.families.Install(uint32(ruleOrdinal), installer) {
			return nil, false
		}
	}
	binding, ok := factbinding.Bind(implementation.algebra, runtime.guards, func(binding *factbinding.Binding[K, V]) bool {
		if catalog.dynamicRead {
			// A staged target Factor declares each exact Unit once, in owner key
			// order. This is O(R) per actually targeted Factor, rather than the
			// former candidate×root cold surface. Static exact reads still share
			// these same Units through bound.reads.
			if implementation.algebra.KeyEnd() > uint64(^uint(0)>>1) {
				return false
			}
			bound.dynamicUnits = make([]carrier.Unit, int(implementation.algebra.KeyEnd()))
			exactIndex := 0
			for raw := uint64(0); raw < implementation.algebra.KeyEnd(); raw++ {
				key := K(raw)
				if uint64(key) != raw {
					return false
				}
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				bound.dynamicUnits[int(raw)] = unit
				for exactIndex < len(catalog.exactReads) && catalog.exactReads[exactIndex].Local == raw+1 {
					surface := catalog.exactReads[exactIndex]
					bound.reads[surface] = boundUnit{unit: unit, kind: carrier.ExactUnit, local: surface.Local}
					exactIndex++
				}
			}
			if exactIndex != len(catalog.exactReads) {
				return false
			}
		} else {
			for _, surface := range catalog.exactReads {
				if surface.Local == 0 {
					return false
				}
				raw := surface.Local - 1
				key := K(raw)
				if uint64(key) != raw {
					return false
				}
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				bound.reads[surface] = boundUnit{unit: unit, kind: carrier.ExactUnit, local: surface.Local}
			}
		}
		summaryUnits := make(map[summaryUnitKey]boundUnit, len(catalog.summaries))
		for ordinal, summary := range catalog.summaries {
			var unit carrier.Unit
			var declared bool
			if summary.unit.distributive {
				unit, declared = binding.DeclareDistributiveSummary(summary.keys)
			} else {
				unit, declared = binding.DeclareSummary(summary.keys)
			}
			if !declared {
				return false
			}
			keys := make([]uint64, len(summary.keys))
			for index, key := range summary.keys {
				keys[index] = uint64(key)
			}
			summaryUnits[summary.unit] = boundUnit{unit: unit, kind: carrier.SummaryUnit, local: uint64(ordinal) + 1, summaryKeys: keys}
		}
		for alias, summary := range catalog.summaryAliases {
			unit, present := summaryUnits[summary]
			if !present {
				return false
			}
			bound.reads[alias] = unit
		}
		if catalog.routeWrite {
			if !catalog.dynamicRead || len(bound.dynamicUnits) != int(implementation.algebra.KeyEnd()) {
				return false
			}
			bound.routeTargets = make([]carrier.Target, len(bound.dynamicUnits))
			for index, unit := range bound.dynamicUnits {
				if unit == (carrier.Unit{}) {
					return false
				}
				target, declared := binding.DeclareStrong(unit)
				if !declared {
					return false
				}
				bound.routeTargets[index] = target
			}
		}
		for _, surface := range catalog.strongWrites {
			unit, present := bound.reads[equation.Surface{Factor: semantic, Form: equation.SurfaceReadExact, Local: surface.Local}]
			if !present {
				return false
			}
			var target carrier.Target
			if catalog.routeWrite {
				raw := surface.Local - 1
				if raw >= uint64(len(bound.routeTargets)) {
					return false
				}
				target = bound.routeTargets[int(raw)]
			} else {
				var declared bool
				target, declared = binding.DeclareStrong(unit.unit)
				if !declared {
					return false
				}
			}
			bound.writes[surface] = boundTarget{target: target, mode: carrier.StrongTarget, local: surface.Local}
		}
		return declareWeakTargets(binding, bound, catalog.weakWrites)
	})
	if !ok || binding == nil || len(bound.reads) != len(catalog.exactReads)+len(catalog.summaryAliases) || len(bound.writes) != len(catalog.strongWrites)+len(catalog.weakWrites) || catalog.dynamicRead && len(bound.dynamicUnits) != int(implementation.algebra.KeyEnd()) || catalog.routeWrite && len(bound.routeTargets) != int(implementation.algebra.KeyEnd()) {
		return nil, false
	}
	for _, row := range catalog.carryTargets {
		targets := make([]carrier.Target, len(row.targets))
		for index, surface := range row.targets {
			target, present := bound.writes[surface]
			if !present {
				return nil, false
			}
			targets[index] = target.target
		}
		bound.carryTargets[row.member] = targets
		bound.carryRouteScope[row.member] = row.route
	}
	bound.binding = binding
	return bound, true
}

func exactReadSurface(row schemaFactorBinding, local uint64) equation.Surface {
	return equation.Surface{Factor: row.schemaFactorSemanticKey(), Form: equation.SurfaceReadExact, Local: local}
}

func exactWriteSurface(row schemaFactorBinding, local uint64) equation.Surface {
	return equation.Surface{Factor: row.schemaFactorSemanticKey(), Form: equation.SurfaceWriteExact, Local: local, Mode: equation.TargetModeStrong}
}

func matchesFactorReadShape(schema *Schema, ordinal uint64, surface equation.Surface, kind readFormKind) bool {
	if schema == nil || !schema.Available() || ordinal >= schema.factorCount() || !surface.Available() || surface.Factor != schema.factorSemanticAt(ordinal) {
		return false
	}
	if kind == exactReadForm {
		return surface.Form == equation.SurfaceReadExact && surface.Mode == equation.TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	}
	if kind != summaryReadForm || surface.Form != equation.SurfaceReadSummary || surface.Mode != equation.TargetModeNone || !surface.Semantic.Available() || surface.Normalizer != surface.Semantic {
		return false
	}
	count, ok := schema.factorFormCount(ordinal)
	if !ok {
		return false
	}
	for index := 0; index < count; index++ {
		form, formOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if formOK && summaryReadRowKind(form.Kind) && form.Semantic == surface.Semantic {
			return true
		}
	}
	return false
}

// summaryReadFormFold resolves the declared fold of one summary read form
// from the sealed cold schema. The normalizer key names exactly one declared
// form, so the fold is recovered without any Rule, Query, or caller input.
func summaryReadFormFold(schema *Schema, ordinal uint64, semantic composition.Key) (bool, bool) {
	if schema == nil || !schema.Available() || ordinal >= schema.factorCount() || !semantic.Available() {
		return false, false
	}
	count, ok := schema.factorFormCount(ordinal)
	if !ok {
		return false, false
	}
	for index := 0; index < count; index++ {
		form, formOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if !formOK || form.Semantic != semantic || !summaryReadRowKind(form.Kind) {
			continue
		}
		return form.Kind == composition.FactorDistributiveSummaryRead, true
	}
	return false, false
}

// collectFactorGraphCatalog performs only cold work. Graph owns every
// occurrence surface, summary key row, weak cover, and selector target; the
// Factor owns only the typed conversion from raw key to K and the carrier
// declarations. There is no caller-supplied materialization language.
func collectFactorGraphCatalog[K ~uint32 | ~uint64, V any](implementation *FactorImplementation[K, V], graph *equation.Graph, uses graphFactorUses) (factorGraphCatalog[K], bool) {
	if implementation == nil || !factorRowAvailable(implementation.row) || implementation.algebra == nil || graph == nil {
		return factorGraphCatalog[K]{}, false
	}
	row := implementation.row
	schema := row.schemaFactorSchema()
	key := row.schemaFactorSemanticKey()
	keyEnd := implementation.algebra.KeyEnd()
	state := row.schemaFactorBindingState()
	exact := make(map[equation.Surface]struct{})
	summaries := make(map[summaryUnitKey][]K)
	aliases := make(map[equation.Surface]summaryUnitKey)
	strong := make(map[equation.Surface]struct{})
	weak := make(map[equation.Surface][]equation.Surface)
	dynamicRead := false
	routeWrite := false

	var collectRead func(equation.Surface) bool
	collectSummary := func(surface equation.Surface) bool {
		if !matchesFactorReadShape(schema, implementation.ordinal, surface, summaryReadForm) || surface.Mode != equation.TargetModeNone {
			return false
		}
		// The fold is read from the declared cold form the surface names, so a
		// summary can never acquire a fold from the Rule or Query that reads it.
		distributive, foldOK := summaryReadFormFold(schema, implementation.ordinal, surface.Semantic)
		if !foldOK {
			return false
		}
		representative, represented := graph.SummaryRepresentative(surface)
		if !represented || !matchesFactorReadShape(schema, implementation.ordinal, representative, summaryReadForm) {
			return false
		}
		// The declared key vector is the Factor's own projection, and a
		// Factor's coordinate universe is sealed. The empty vector is
		// therefore that Factor's sealed empty projection, admitted here on
		// the same terms as the proper subsets around it.
		keyRange, ranged := graph.SummaryKeyRange(representative)
		count := keyRange.Count()
		if !ranged || count < 0 {
			return false
		}
		keys := make([]K, count)
		for index := range keys {
			raw, present := keyRange.At(index)
			if !present || raw >= keyEnd {
				return false
			}
			keys[index] = K(raw)
			if uint64(keys[index]) != raw || index > 0 && keys[index-1] >= keys[index] {
				return false
			}
		}
		unit := summaryUnitKey{representative: representative, distributive: distributive}
		if prior, exists := summaries[unit]; exists && !sameSummaryKeys(prior, keys) {
			return false
		}
		summaries[unit] = keys
		aliases[surface] = unit
		if _, present := aliases[representative]; !present {
			representativeDistributive, representativeFoldOK := summaryReadFormFold(schema, implementation.ordinal, representative.Semantic)
			if !representativeFoldOK {
				return false
			}
			representativeUnit := summaryUnitKey{representative: representative, distributive: representativeDistributive}
			if prior, exists := summaries[representativeUnit]; exists && !sameSummaryKeys(prior, keys) {
				return false
			}
			summaries[representativeUnit] = keys
			aliases[representative] = representativeUnit
		}
		return true
	}
	collectRead = func(surface equation.Surface) bool {
		if !surface.Available() || surface.Factor != key || surface.Mode != equation.TargetModeNone {
			return false
		}
		switch surface.Form {
		case equation.SurfaceReadExact:
			if surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > keyEnd {
				return false
			}
			exact[surface] = struct{}{}
			return true
		case equation.SurfaceReadSummary:
			return collectSummary(surface)
		default:
			return false
		}
	}
	collectWeak := func(surface equation.Surface) bool {
		if surface.Factor != key || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeWeak || surface.Semantic.Available() || surface.Normalizer.Available() {
			return false
		}
		count, counted := graph.WeakTargetCandidateCount(surface)
		if !counted || count == 0 {
			return false
		}
		candidates := make([]equation.Surface, count)
		for index := range candidates {
			candidate, present := graph.WeakTargetCandidateAt(surface, index)
			if !present || !collectRead(candidate) {
				return false
			}
			candidates[index] = candidate
		}
		if prior, exists := weak[surface]; exists && !sameSurfaceVector(prior, candidates) {
			return false
		}
		weak[surface] = candidates
		return true
	}
	collectTarget := func(surface equation.Surface) bool {
		if !surface.Available() || surface.Factor != key || surface.Form != equation.SurfaceWriteExact || surface.Semantic.Available() || surface.Normalizer.Available() {
			return false
		}
		switch surface.Mode {
		case equation.TargetModeStrong:
			if surface.Local == 0 || surface.Local > keyEnd {
				return false
			}
			strong[surface] = struct{}{}
			exact[exactReadSurface(row, surface.Local)] = struct{}{}
			return true
		case equation.TargetModeWeak:
			return collectWeak(surface)
		default:
			return false
		}
	}
	for _, use := range uses.reads {
		row := use.row
		if use.index < 0 || state == nil {
			return factorGraphCatalog[K]{}, false
		}
		if use.sealedExact {
			// Exact surface geometry was sealed against this graph before the
			// catalog walk; no legacy per-read cell may be present or synthesized.
			if row != nil || use.surface.Form != equation.SurfaceReadExact || !collectRead(use.surface) {
				return factorGraphCatalog[K]{}, false
			}
			continue
		}
		if row == nil || !row.sealed() || row.ownerState() != state || row.ownerOrdinal >= uint64(len(state.rules)) || row.readOrdinal != uint64(use.index) || row.factor != key {
			return factorGraphCatalog[K]{}, false
		}
		if use.surface.Form == equation.SurfaceReadSelect {
			// A ReadSelect names its target Factor only. Exact Ref routes are
			// chosen row-locally by the staged locator; no target Unit or
			// candidate vector is present in the graph catalog. A selection
			// over the candidate alone declares no dependency: dependencies
			// order a selection against the row's own earlier joins, and a
			// select with no such dependency is well-formed rather than
			// unreachable, matching the declared cold row.
			if row.kind != composition.ReadSelect ||
				use.surface.Mode != equation.TargetModeNone || use.surface.Semantic != key || use.surface.Semantic != row.semantic || use.surface.Normalizer.Available() || !use.surface.LocalAvailable() {
				return factorGraphCatalog[K]{}, false
			}
			dynamicRead = true
			continue
		}
		if len(row.dependencies) != 0 || use.surface.Form == equation.SurfaceReadExact && row.kind != composition.ReadExact || use.surface.Form == equation.SurfaceReadSummary && row.kind != composition.ReadSummary || !collectRead(use.surface) {
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, use := range uses.writes {
		if use.index < 0 || !use.surface.Available() || use.surface.Factor != key {
			return factorGraphCatalog[K]{}, false
		}
		switch {
		case use.routeRead == 0 && use.surface.Form == equation.SurfaceWriteExact:
			if !collectTarget(use.surface) {
				return factorGraphCatalog[K]{}, false
			}
		case use.routeRead != 0 && use.surface.Form == equation.SurfaceWriteRoute:
			if use.surface.Mode != equation.TargetModeNone || use.surface.Semantic.Available() || use.surface.Normalizer.Available() {
				return factorGraphCatalog[K]{}, false
			}
			routeWrite = true
		default:
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, target := range uses.targets {
		if !collectTarget(target) {
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, closure := range uses.carryTargets {
		for _, target := range closure.targets {
			if !collectTarget(target) {
				return factorGraphCatalog[K]{}, false
			}
		}
	}
	for _, surface := range uses.queries {
		if surface.Form == equation.SurfaceReadSelect || !collectRead(surface) {
			return factorGraphCatalog[K]{}, false
		}
	}
	if routeWrite && !dynamicRead {
		return factorGraphCatalog[K]{}, false
	}
	// The exact read catalog is the closed set of coordinates this Factor's
	// owner can issue a read of: every cell a strong unrouted write names, and
	// - where a member publishes through a route - every cell of the sealed
	// coordinate universe that route resolves into. Collecting only the
	// coordinates some declaration happened to name left the set incidental:
	// an owner-issued Ref bound when a query had spelled its coordinate and
	// refused when none had, which is not a property of the Factor.
	if routeWrite {
		for local := uint64(1); local <= keyEnd; local++ {
			exact[exactReadSurface(row, local)] = struct{}{}
		}
	}
	result := factorGraphCatalog[K]{summaryAliases: aliases, dynamicRead: dynamicRead, routeWrite: routeWrite}
	for surface := range exact {
		result.exactReads = append(result.exactReads, surface)
	}
	sort.Slice(result.exactReads, func(left, right int) bool {
		return result.exactReads[left].Local < result.exactReads[right].Local
	})
	for unit, keys := range summaries {
		result.summaries = append(result.summaries, summaryBindingRow[K]{unit: unit, keys: keys})
	}
	sort.Slice(result.summaries, func(left, right int) bool {
		if comparison := equation.CompareKeyVectors(result.summaries[left].keys, result.summaries[right].keys); comparison != 0 {
			return comparison < 0
		}
		if result.summaries[left].unit.distributive != result.summaries[right].unit.distributive {
			return !result.summaries[left].unit.distributive
		}
		return lessRuntimeSurface(result.summaries[left].unit.representative, result.summaries[right].unit.representative)
	})
	for surface := range strong {
		result.strongWrites = append(result.strongWrites, surface)
	}
	sort.Slice(result.strongWrites, func(left, right int) bool {
		return lessRuntimeSurface(result.strongWrites[left], result.strongWrites[right])
	})
	result.weakWrites = sortedSurfaceVectors(weak)
	for member, closure := range uses.carryTargets {
		result.carryTargets = append(result.carryTargets, carryTargetRow{member: member, targets: append([]equation.Surface(nil), closure.targets...), route: closure.route})
	}
	sort.Slice(result.carryTargets, func(left, right int) bool {
		return lessRuntimeKey(result.carryTargets[left].member, result.carryTargets[right].member)
	})
	return result, true
}

func sameSurfaceVector(left, right []equation.Surface) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortedSurfaceVectors(rows map[equation.Surface][]equation.Surface) []surfaceVectorRow {
	result := make([]surfaceVectorRow, 0, len(rows))
	for surface, candidates := range rows {
		result = append(result, surfaceVectorRow{surface: surface, candidates: candidates})
	}
	sort.Slice(result, func(left, right int) bool { return lessRuntimeSurface(result[left].surface, result[right].surface) })
	return result
}

func compactCarrySurfaces(surfaces []equation.Surface) []equation.Surface {
	if len(surfaces) < 2 {
		return surfaces
	}
	sort.Slice(surfaces, func(left, right int) bool { return lessRuntimeSurface(surfaces[left], surfaces[right]) })
	end := 1
	for _, surface := range surfaces[1:] {
		if surface != surfaces[end-1] {
			surfaces[end] = surface
			end++
		}
	}
	return surfaces[:end]
}

func lessRuntimeSurface(left, right equation.Surface) bool {
	if comparison := compareRuntimeKey(left.Factor, right.Factor); comparison != 0 {
		return comparison < 0
	}
	if left.Form != right.Form {
		return left.Form < right.Form
	}
	if left.Local != right.Local {
		return left.Local < right.Local
	}
	if comparison := bytes.Compare(left.Content[:], right.Content[:]); comparison != 0 {
		return comparison < 0
	}
	if left.Mode != right.Mode {
		return left.Mode < right.Mode
	}
	if comparison := compareRuntimeKey(left.Semantic, right.Semantic); comparison != 0 {
		return comparison < 0
	}
	return compareRuntimeKey(left.Normalizer, right.Normalizer) < 0
}

func compareRuntimeKey(left, right composition.Key) int {
	for index := range left.ID {
		if left.ID[index] < right.ID[index] {
			return -1
		}
		if left.ID[index] > right.ID[index] {
			return 1
		}
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func sameSummaryKeys[K ~uint32 | ~uint64](left, right []K) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func unitLess(left, right boundUnit) bool {
	return left.kind < right.kind || left.kind == right.kind && left.local < right.local
}

type resolvedWeakTarget struct {
	surface    equation.Surface
	candidates []boundUnit
}

func declareWeakTargets[K ~uint32 | ~uint64, V any](binding *factbinding.Binding[K, V], bound *boundFactor[K, V], plans []surfaceVectorRow) bool {
	resolved := make([]resolvedWeakTarget, len(plans))
	for index, plan := range plans {
		if plan.surface.Form != equation.SurfaceWriteExact || plan.surface.Mode != equation.TargetModeWeak || len(plan.candidates) == 0 {
			return false
		}
		candidates := make([]boundUnit, len(plan.candidates))
		for candidateIndex, surface := range plan.candidates {
			unit, ok := bound.reads[surface]
			if !ok {
				return false
			}
			candidates[candidateIndex] = unit
		}
		sort.Slice(candidates, func(left, right int) bool { return unitLess(candidates[left], candidates[right]) })
		for candidateIndex := range candidates {
			if candidateIndex > 0 && !unitLess(candidates[candidateIndex-1], candidates[candidateIndex]) {
				return false
			}
		}
		resolved[index] = resolvedWeakTarget{surface: plan.surface, candidates: candidates}
	}
	sort.Slice(resolved, func(left, right int) bool {
		if lessUnitVector(resolved[left].candidates, resolved[right].candidates) {
			return true
		}
		if lessUnitVector(resolved[right].candidates, resolved[left].candidates) {
			return false
		}
		return lessRuntimeSurface(resolved[left].surface, resolved[right].surface)
	})
	for index, weak := range resolved {
		if index > 0 && sameUnitVector(resolved[index-1].candidates, weak.candidates) {
			bound.writes[weak.surface] = bound.writes[resolved[index-1].surface]
			continue
		}
		if index > 0 && !lessUnitVector(resolved[index-1].candidates, weak.candidates) {
			return false
		}
		units := make([]carrier.Unit, len(weak.candidates))
		for candidateIndex, candidate := range weak.candidates {
			units[candidateIndex] = candidate.unit
		}
		target, ok := binding.DeclareWeak(units)
		if !ok {
			return false
		}
		bound.writes[weak.surface] = boundTarget{target: target, mode: carrier.WeakTarget, local: uint64(index) + 1}
	}
	return true
}

func lessUnitVector(left, right []boundUnit) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if unitLess(left[index], right[index]) {
			return true
		}
		if unitLess(right[index], left[index]) {
			return false
		}
	}
	return len(left) < len(right)
}

func sameUnitVector(left, right []boundUnit) bool {
	return !lessUnitVector(left, right) && !lessUnitVector(right, left)
}
