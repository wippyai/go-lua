package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// topologyCatalog is cold compilation scratch. Its maps never enter Graph and
// therefore cannot become a second runtime authority: Graph freezes the same
// rows as sorted, flat CSR catalogs below.
type topologyCatalog struct {
	summaries []summaryMapping
	weak      []weakTargetMapping
	summaryAt map[Surface]int
	weakAt    map[Surface]int
}

type summaryMapping struct {
	surface        Surface
	keys           []uint64
	representative Surface
}

type weakTargetMapping struct {
	surface    Surface
	candidates []Surface
}

func buildTopologyCatalog(topology TopologySpec) (topologyCatalog, bool) {
	catalog := topologyCatalog{
		summaries: make([]summaryMapping, len(topology.Summaries)),
		weak:      make([]weakTargetMapping, len(topology.WeakTargets)),
		summaryAt: make(map[Surface]int, len(topology.Summaries)),
		weakAt:    make(map[Surface]int, len(topology.WeakTargets)),
	}
	for index, row := range topology.Summaries {
		if !validSummaryMapping(row) {
			return topologyCatalog{}, false
		}
		if _, duplicate := catalog.summaryAt[row.Surface]; duplicate {
			return topologyCatalog{}, false
		}
		catalog.summaryAt[row.Surface] = index
		catalog.summaries[index] = summaryMapping{surface: row.Surface, keys: append([]uint64(nil), row.Keys...)}
	}
	if !canonicalizeSummaryRepresentatives(catalog.summaries) {
		return topologyCatalog{}, false
	}
	sort.Slice(catalog.summaries, func(left, right int) bool {
		return lessSurface(catalog.summaries[left].surface, catalog.summaries[right].surface)
	})
	catalog.summaryAt = make(map[Surface]int, len(catalog.summaries))
	for index, row := range catalog.summaries {
		if _, duplicate := catalog.summaryAt[row.surface]; duplicate {
			return topologyCatalog{}, false
		}
		catalog.summaryAt[row.surface] = index
	}
	for index, row := range topology.WeakTargets {
		if !validWeakTargetMapping(row) {
			return topologyCatalog{}, false
		}
		if _, duplicate := catalog.weakAt[row.Surface]; duplicate {
			return topologyCatalog{}, false
		}
		resolved, ok := resolveWeakCoverage(row.Candidates, catalog)
		if !ok {
			return topologyCatalog{}, false
		}
		catalog.weakAt[row.Surface] = index
		catalog.weak[index] = weakTargetMapping{surface: row.Surface, candidates: resolved}
	}
	sort.Slice(catalog.weak, func(left, right int) bool {
		return lessSurface(catalog.weak[left].surface, catalog.weak[right].surface)
	})
	catalog.weakAt = make(map[Surface]int, len(catalog.weak))
	for index, row := range catalog.weak {
		if _, duplicate := catalog.weakAt[row.surface]; duplicate {
			return topologyCatalog{}, false
		}
		catalog.weakAt[row.surface] = index
	}
	return catalog, true
}

func validSummaryMapping(row SummaryMapping) bool {
	return row.Surface.Available() && row.Surface.Form == SurfaceReadSummary && row.Surface.Mode == TargetModeNone &&
		row.Surface.Semantic.Available() && row.Surface.Normalizer.Available() && row.Surface.Semantic == row.Surface.Normalizer &&
		len(row.Keys) != 0 && validRawKeySet(row.Keys)
}

func validWeakTargetMapping(row WeakTargetMapping) bool {
	if !row.Surface.Available() || row.Surface.Form != SurfaceWriteExact || row.Surface.Mode != TargetModeWeak ||
		row.Surface.Semantic.Available() || row.Surface.Normalizer.Available() || len(row.Candidates) == 0 {
		return false
	}
	for index, candidate := range row.Candidates {
		if !validWeakCoverageSurface(candidate, row.Surface.Factor) || index > 0 && !lessSurface(row.Candidates[index-1], candidate) {
			return false
		}
	}
	return true
}

func validWeakCoverageSurface(surface Surface, factor composition.Key) bool {
	if !surface.Available() || surface.Factor != factor {
		return false
	}
	switch surface.Form {
	case SurfaceReadExact:
		return !surface.Semantic.Available() && !surface.Normalizer.Available()
	case SurfaceReadSummary:
		return surface.Semantic.Available() && surface.Normalizer.Available() && surface.Semantic == surface.Normalizer
	default:
		return false
	}
}

func validRawKeySet(keys []uint64) bool {
	for index := range keys {
		if index > 0 && keys[index-1] >= keys[index] {
			return false
		}
	}
	return true
}

func canonicalizeSummaryRepresentatives(rows []summaryMapping) bool {
	order := make([]int, len(rows))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(left, right int) bool {
		leftRow, rightRow := rows[order[left]], rows[order[right]]
		if comparison := compareKey(leftRow.surface.Factor, rightRow.surface.Factor); comparison != 0 {
			return comparison < 0
		}
		if comparison := compareRawKeySets(leftRow.keys, rightRow.keys); comparison != 0 {
			return comparison < 0
		}
		return lessSurface(leftRow.surface, rightRow.surface)
	})
	for begin := 0; begin < len(order); {
		end := begin + 1
		for end < len(order) && sameSummaryUnit(rows[order[begin]], rows[order[end]]) {
			end++
		}
		representative := rows[order[begin]].surface
		for index := begin; index < end; index++ {
			rows[order[index]].representative = representative
		}
		begin = end
	}
	return true
}

// A carrier unit belongs to one Factor. Equal raw key vectors across Factors
// are therefore not aliases: the unit equivalence is exactly the pair
// (Factor, raw-key set), never the key set alone.
func sameSummaryUnit(left, right summaryMapping) bool {
	return left.surface.Factor == right.surface.Factor && compareRawKeySets(left.keys, right.keys) == 0
}

func compareRawKeySets(left, right []uint64) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func resolveWeakCoverage(candidates []Surface, catalog topologyCatalog) ([]Surface, bool) {
	resolved := make([]Surface, len(candidates))
	for index, candidate := range candidates {
		switch candidate.Form {
		case SurfaceReadExact:
			resolved[index] = candidate
		case SurfaceReadSummary:
			mapping, ok := catalog.summary(candidate)
			if !ok {
				return nil, false
			}
			resolved[index] = mapping.representative
		default:
			return nil, false
		}
	}
	sort.Slice(resolved, func(left, right int) bool { return lessSurface(resolved[left], resolved[right]) })
	unique := resolved[:0]
	for _, candidate := range resolved {
		if len(unique) == 0 || unique[len(unique)-1] != candidate {
			unique = append(unique, candidate)
		}
	}
	return append([]Surface(nil), unique...), len(unique) != 0
}

func (catalog topologyCatalog) summary(surface Surface) (summaryMapping, bool) {
	index, ok := catalog.summaryAt[surface]
	if !ok || index < 0 || index >= len(catalog.summaries) {
		return summaryMapping{}, false
	}
	return catalog.summaries[index], true
}

func (catalog topologyCatalog) weakTarget(surface Surface) (weakTargetMapping, bool) {
	index, ok := catalog.weakAt[surface]
	if !ok || index < 0 || index >= len(catalog.weak) {
		return weakTargetMapping{}, false
	}
	return catalog.weak[index], true
}

// validateTopologyCatalogUsage rejects a second, latent capability catalog.
// A mapping must be demanded by one Rule/Query surface (or, for a summary,
// by the coverage of such a weak target); otherwise it is foreign topology
// state and cannot survive compilation.
func validateTopologyCatalogUsage(topology TopologySpec, catalog topologyCatalog) bool {
	usedSummary := make(map[Surface]bool, len(catalog.summaries))
	usedWeak := make(map[Surface]bool, len(catalog.weak))
	authoredWeakCoverage := make(map[Surface][]Surface, len(topology.WeakTargets))
	for _, mapping := range topology.WeakTargets {
		authoredWeakCoverage[mapping.Surface] = mapping.Candidates
	}
	markSurface := func(surface Surface) bool {
		switch {
		case surface.Form == SurfaceReadSummary:
			if _, ok := catalog.summary(surface); !ok {
				return false
			}
			usedSummary[surface] = true
		case surface.Form == SurfaceWriteExact && surface.Mode == TargetModeWeak:
			if _, ok := catalog.weakTarget(surface); !ok {
				return false
			}
			usedWeak[surface] = true
		}
		return true
	}
	for _, rule := range topology.Rules {
		for _, read := range rule.Reads {
			if !markSurface(read.Surface) {
				return false
			}
		}
		for _, write := range rule.Writes {
			if !markSurface(write.Surface) {
				return false
			}
			for _, target := range write.TargetCandidates {
				if !markSurface(target) {
					return false
				}
			}
		}
	}
	for _, query := range topology.Queries {
		for _, surface := range query.Surfaces {
			if !markSurface(surface) {
				return false
			}
		}
	}
	for surface := range usedWeak {
		coverage, ok := authoredWeakCoverage[surface]
		if !ok {
			return false
		}
		// Validate reachability against authored coverage before aliases are
		// normalized. Otherwise an alias that correctly contributes to a weak
		// target would be misclassified as unused merely because it shares its
		// canonical unit with another summary surface.
		for _, candidate := range coverage {
			if candidate.Form == SurfaceReadSummary {
				usedSummary[candidate] = true
			}
		}
	}
	return len(usedSummary) == len(catalog.summaries) && len(usedWeak) == len(catalog.weak)
}

func compareSurface(left, right Surface) int {
	if left.Form < right.Form {
		return -1
	}
	if left.Form > right.Form {
		return 1
	}
	if comparison := compareKey(left.Factor, right.Factor); comparison != 0 {
		return comparison
	}
	if left.Local < right.Local {
		return -1
	}
	if left.Local > right.Local {
		return 1
	}
	if comparison := compareKey(left.Semantic, right.Semantic); comparison != 0 {
		return comparison
	}
	if comparison := compareKey(left.Normalizer, right.Normalizer); comparison != 0 {
		return comparison
	}
	if left.Mode < right.Mode {
		return -1
	}
	if left.Mode > right.Mode {
		return 1
	}
	return 0
}

func lessSurface(left, right Surface) bool { return compareSurface(left, right) < 0 }

func writeRuleCatalog(writer *canonical.Writer, row RuleInstance, catalog topologyCatalog) bool {
	if writer.Count(uint64(len(row.Reads))) != nil {
		return false
	}
	for _, read := range row.Reads {
		if !writeSurfaceCatalog(writer, read.Surface, catalog) {
			return false
		}
	}
	if writer.Count(uint64(len(row.Writes))) != nil {
		return false
	}
	for _, write := range row.Writes {
		if !writeSurfaceCatalog(writer, write.Surface, catalog) || writer.Count(uint64(len(write.TargetCandidates))) != nil {
			return false
		}
		for _, target := range write.TargetCandidates {
			if !writeSurfaceCatalog(writer, target, catalog) {
				return false
			}
		}
	}
	return true
}

// writeSurfaceCatalog emits only the mapping content; the caller has already
// emitted the Surface itself in the enclosing Rule/Query identity.
func writeSurfaceCatalog(writer *canonical.Writer, surface Surface, catalog topologyCatalog) bool {
	switch {
	case surface.Form == SurfaceReadSummary:
		mapping, ok := catalog.summary(surface)
		return ok && writer.Uint(1) == nil && writeRawKeySet(writer, mapping.keys)
	case surface.Form == SurfaceWriteExact && surface.Mode == TargetModeWeak:
		mapping, ok := catalog.weakTarget(surface)
		if !ok || writer.Uint(2) != nil || writer.Count(uint64(len(mapping.candidates))) != nil {
			return false
		}
		for _, candidate := range mapping.candidates {
			if !writeSurface(writer, candidate) || !writeSurfaceCatalog(writer, candidate, catalog) {
				return false
			}
		}
		return true
	default:
		return writer.Uint(0) == nil
	}
}

func writeRawKeySet(writer *canonical.Writer, keys []uint64) bool {
	if writer.Count(uint64(len(keys))) != nil {
		return false
	}
	for _, key := range keys {
		if writer.Uint(key) != nil {
			return false
		}
	}
	return true
}

// installCatalog copies the compiler scratch into immutable, sorted CSR rows.
// The Graph deliberately retains no scratch lookup map and no caller slice.
func (graph *Graph) installCatalog(catalog topologyCatalog) bool {
	if graph == nil || len(catalog.summaryAt) != len(catalog.summaries) || len(catalog.weakAt) != len(catalog.weak) {
		return false
	}
	graph.summarySurfaces = make([]Surface, len(catalog.summaries))
	graph.summaryRepresentatives = make([]Surface, len(catalog.summaries))
	graph.summaryOffsets = make([]int, len(catalog.summaries)+1)
	for index, row := range catalog.summaries {
		if index > 0 && !lessSurface(graph.summarySurfaces[index-1], row.surface) {
			return false
		}
		graph.summarySurfaces[index] = row.surface
		graph.summaryRepresentatives[index] = row.representative
		graph.summaryOffsets[index] = len(graph.summaryKeys)
		graph.summaryKeys = append(graph.summaryKeys, row.keys...)
	}
	graph.summaryOffsets[len(catalog.summaries)] = len(graph.summaryKeys)
	graph.weakTargetSurfaces = make([]Surface, len(catalog.weak))
	graph.weakTargetOffsets = make([]int, len(catalog.weak)+1)
	for index, row := range catalog.weak {
		if index > 0 && !lessSurface(graph.weakTargetSurfaces[index-1], row.surface) {
			return false
		}
		graph.weakTargetSurfaces[index] = row.surface
		graph.weakTargetOffsets[index] = len(graph.weakTargetCandidates)
		graph.weakTargetCandidates = append(graph.weakTargetCandidates, row.candidates...)
	}
	graph.weakTargetOffsets[len(catalog.weak)] = len(graph.weakTargetCandidates)
	return graph.validCatalog()
}

func (graph *Graph) validCatalog() bool {
	if graph == nil || len(graph.summarySurfaces) != len(graph.summaryRepresentatives) || len(graph.summaryOffsets) != len(graph.summarySurfaces)+1 ||
		len(graph.weakTargetOffsets) != len(graph.weakTargetSurfaces)+1 {
		return false
	}
	for index, surface := range graph.summarySurfaces {
		if !validSummaryMapping(SummaryMapping{Surface: surface, Keys: graph.summaryKeys[graph.summaryOffsets[index]:graph.summaryOffsets[index+1]]}) ||
			index > 0 && !lessSurface(graph.summarySurfaces[index-1], surface) {
			return false
		}
	}
	for index, surface := range graph.weakTargetSurfaces {
		candidates := graph.weakTargetCandidates[graph.weakTargetOffsets[index]:graph.weakTargetOffsets[index+1]]
		if !surface.Available() || surface.Form != SurfaceWriteExact || surface.Mode != TargetModeWeak || len(candidates) == 0 ||
			index > 0 && !lessSurface(graph.weakTargetSurfaces[index-1], surface) {
			return false
		}
		for candidateIndex, candidate := range candidates {
			if !validWeakCoverageSurface(candidate, surface.Factor) || candidateIndex > 0 && !lessSurface(candidates[candidateIndex-1], candidate) {
				return false
			}
		}
	}
	return true
}

func surfaceIndex(rows []Surface, surface Surface) (int, bool) {
	index := sort.Search(len(rows), func(index int) bool { return !lessSurface(rows[index], surface) })
	return index, index < len(rows) && rows[index] == surface
}

// SummaryKeyCount and SummaryKeyAt expose a compiled raw-key set without
// allocating a copy. Raw keys remain in [0, KeyEnd); only the typed Factor
// binder validates that range.
func (graph *Graph) SummaryKeyCount(surface Surface) (int, bool) {
	if !graph.valid() {
		return 0, false
	}
	index, ok := surfaceIndex(graph.summarySurfaces, surface)
	if !ok {
		return 0, false
	}
	return graph.summaryOffsets[index+1] - graph.summaryOffsets[index], true
}

func (graph *Graph) SummaryKeyAt(surface Surface, index int) (uint64, bool) {
	if !graph.valid() || index < 0 {
		return 0, false
	}
	row, ok := surfaceIndex(graph.summarySurfaces, surface)
	if !ok {
		return 0, false
	}
	begin, end := graph.summaryOffsets[row], graph.summaryOffsets[row+1]
	if index >= end-begin {
		return 0, false
	}
	return graph.summaryKeys[begin+index], true
}

// SummaryRepresentative returns the canonical read unit for an aliasing
// summary surface. It is a pure catalog lookup; it neither consults nor
// creates a carrier unit.
func (graph *Graph) SummaryRepresentative(surface Surface) (Surface, bool) {
	if !graph.valid() {
		return Surface{}, false
	}
	index, ok := surfaceIndex(graph.summarySurfaces, surface)
	if !ok {
		return Surface{}, false
	}
	return graph.summaryRepresentatives[index], true
}

func (graph *Graph) WeakTargetCandidateCount(surface Surface) (int, bool) {
	if !graph.valid() {
		return 0, false
	}
	index, ok := surfaceIndex(graph.weakTargetSurfaces, surface)
	if !ok {
		return 0, false
	}
	return graph.weakTargetOffsets[index+1] - graph.weakTargetOffsets[index], true
}

// WeakTargetCandidateAt returns the canonical exact/summary unit
// representative covered by surface. The catalog is a set, so rows are
// strictly ordered even when the authored coverage named summary aliases.
func (graph *Graph) WeakTargetCandidateAt(surface Surface, index int) (Surface, bool) {
	if !graph.valid() || index < 0 {
		return Surface{}, false
	}
	row, ok := surfaceIndex(graph.weakTargetSurfaces, surface)
	if !ok {
		return Surface{}, false
	}
	begin, end := graph.weakTargetOffsets[row], graph.weakTargetOffsets[row+1]
	if index >= end-begin {
		return Surface{}, false
	}
	return graph.weakTargetCandidates[begin+index], true
}
