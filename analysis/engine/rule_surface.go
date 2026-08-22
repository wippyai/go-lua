package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Solver-side binding-surface value constructors. Per-issuance declarations
// return immutable row bundles of these values; construction folds those rows
// into the sealed equation topology.

func validateSummarySurface(mapping *ruleSummaryMapping, state *schemaBindingState, authority *schemaBindingAuthority) bool {
	if mapping == nil || mapping.state == nil || mapping.authority == nil || mapping.state != state || mapping.authority != authority {
		return false
	}
	factor, normalizer := mapping.factor, mapping.normalizer
	surface := mapping.surface
	return factor.Available() && normalizer.Available() && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceReadSummary && surface.Semantic == normalizer && surface.Normalizer == normalizer && surface.Mode == equation.TargetModeNone
}

// summaryKeyVector is the engine-owned adapter for an already-materialized
// query summary vector. Rule operands use the scalar source seam directly;
// this adapter is only for sealed query metadata that is already a vector.
type summaryKeyVector struct {
	keys  []uint64
	valid bool
}

func newSummaryKeyVector(keys []uint64) summaryKeyVector {
	copyOf := make([]uint64, len(keys))
	copy(copyOf, keys)
	return summaryKeyVector{keys: copyOf, valid: true}
}

func (vector summaryKeyVector) SummaryKeyCount() int {
	if !vector.valid {
		return -1
	}
	return len(vector.keys)
}

func (vector summaryKeyVector) SummaryKeyAt(index int) (uint64, bool) {
	if !vector.valid || index < 0 || index >= len(vector.keys) {
		return 0, false
	}
	return vector.keys[index], true
}

func summaryFactorKeyEnd(state *schemaBindingState, factor composition.Key) (uint64, bool) {
	if state == nil || state.schema == nil || !factor.Available() {
		return 0, false
	}
	ordinal, ok := state.schema.factorOrdinalOf(factor)
	if !ok || ordinal >= uint64(len(state.factors)) {
		return 0, false
	}
	row, ok := state.factors[ordinal].(schemaFactorBinding)
	if !ok || row == nil || row.schemaFactorAlgebra() == nil {
		return 0, false
	}
	return row.schemaFactorAlgebra().KeyEnd(), true
}

// summaryKeysAllowed admits an empty vector only for the one schema Factor
// whose sealed algebra has no coordinates. Non-empty vectors must be strict,
// in-range ordered coordinates owned by that Factor; malformed, unsorted, or
// out-of-range source rows refuse before their digest is issued.
func summaryKeysAllowed(state *schemaBindingState, factor composition.Key, keys summaryKeySource) bool {
	if keys == nil {
		return false
	}
	count := keys.SummaryKeyCount()
	if count < 0 {
		return false
	}
	keyEnd, keyEndOK := summaryFactorKeyEnd(state, factor)
	if !keyEndOK {
		return false
	}
	if count == 0 {
		return keyEnd == 0
	}
	var previous uint64
	for index := 0; index < count; index++ {
		key, present := keys.SummaryKeyAt(index)
		if !present || key >= keyEnd || index > 0 && previous >= key {
			return false
		}
		previous = key
	}
	return true
}

// RuleReadSurface and ruleWriteSurface are owner-issued sealed coordinate
// values. The Ref factory preserves the originating Binding authority and
// declaration folding rejects foreign/equal-but-distinct refs.
type RuleReadSurface struct {
	value     equation.Surface
	authority *schemaBindingAuthority
	// anchored marks a surface whose coordinate is derived from the issuance
	// anchor rather than from an owner Ref. Two issuances that produce the
	// same anchored coordinate are a construction fault, so the declaration
	// pass claims each one exactly once.
	anchored bool
	summary  *ruleSummaryMapping
}

type ruleSummaryMapping struct {
	state      *schemaBindingState
	authority  *schemaBindingAuthority
	factor     composition.Key
	normalizer composition.Key
	surface    equation.Surface
	keys       summaryKeySource
}

type ruleWriteSurface struct {
	value     equation.Surface
	authority *schemaBindingAuthority
	anchored  bool
}

func ExactReadSurface[K ~uint32 | ~uint64](ref Ref[K]) (RuleReadSurface, bool) {
	row := ref.row
	if !factorRowAvailable(row) || uint64(ref.raw) >= row.schemaFactorAlgebra().KeyEnd() {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{value: equation.Surface{Factor: row.schemaFactorSemanticKey(), Form: equation.SurfaceReadExact, Local: uint64(ref.raw) + 1}, authority: row.schemaFactorBindingState().authority}, true
}

func exactRuleWriteSurface[K ~uint32 | ~uint64](ref Ref[K]) (ruleWriteSurface, bool) {
	row := ref.row
	if !factorRowAvailable(row) || uint64(ref.raw) >= row.schemaFactorAlgebra().KeyEnd() {
		return ruleWriteSurface{}, false
	}
	return ruleWriteSurface{value: equation.Surface{Factor: row.schemaFactorSemanticKey(), Form: equation.SurfaceWriteExact, Local: uint64(ref.raw) + 1, Mode: equation.TargetModeStrong}, authority: row.schemaFactorBindingState().authority}, true
}

// summaryReadSurface materializes a summary surface from the compiled read
// row and source-owned dense Value keys. The key-vector digest is the surface
// coordinate at full width, so two distinct vectors always name two distinct
// summary surfaces.
func summaryReadSurface(state *schemaBindingState, authority *schemaBindingAuthority, row *schemaRuleReadRow, keys summaryKeySource) (RuleReadSurface, bool) {
	if state == nil || state.schema == nil || authority == nil || state.authority != authority || state.phase != schemaBindingSealed || row == nil || !row.sealed() || !summaryKeysAllowed(state, row.factor, keys) {
		return RuleReadSurface{}, false
	}
	if row.ownerState() != state || row.kind != composition.ReadSummary || len(row.dependencies) != 0 || !row.factor.Available() || !row.semantic.Available() || row.semantic != row.normalizer {
		return RuleReadSurface{}, false
	}
	digest := summaryVectorDigestSource(keys)
	if digest == ([32]byte{}) {
		return RuleReadSurface{}, false
	}
	surface := equation.Surface{Factor: row.factor, Form: equation.SurfaceReadSummary, Content: digest, Semantic: row.semantic, Normalizer: row.normalizer}
	if !surface.Available() {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{
		value:     surface,
		authority: authority,
		summary: &ruleSummaryMapping{
			state: state, authority: authority,
			factor: row.factor, normalizer: row.semantic,
			surface: surface, keys: keys,
		},
	}, true
}

// summaryVectorDigestSource hashes an owner-issued scalar range without
// materializing a temporary slice. The framing is identical to the existing
// uint64 vector digest, so the sealed surface identity remains unchanged.
func summaryVectorDigestSource(keys summaryKeySource) [32]byte {
	if keys == nil || keys.SummaryKeyCount() < 0 {
		return [32]byte{}
	}
	digest, ok := framedDigest(summaryVectorDigestDomain, summaryVectorDigestVersion, func(writer *canonical.DigestWriter) bool {
		count := keys.SummaryKeyCount()
		if writer.Uint(summaryKeyWidth[uint64]()) != nil || writer.Count(uint64(count)) != nil {
			return false
		}
		for index := 0; index < count; index++ {
			key, present := keys.SummaryKeyAt(index)
			if !present || writer.Uint(key) != nil {
				return false
			}
		}
		return true
	})
	if !ok {
		return [32]byte{}
	}
	return digest
}

func sameSummaryKeySource(left []uint64, right summaryKeySource) bool {
	if right == nil || right.SummaryKeyCount() != len(left) {
		return false
	}
	for index, expected := range left {
		actual, present := right.SummaryKeyAt(index)
		if !present || actual != expected {
			return false
		}
	}
	return true
}

func materializeSummaryKeys(keys summaryKeySource) ([]uint64, bool) {
	if keys == nil || keys.SummaryKeyCount() < 0 {
		return nil, false
	}
	count := keys.SummaryKeyCount()
	result := make([]uint64, count)
	for index := range result {
		key, present := keys.SummaryKeyAt(index)
		if !present {
			return nil, false
		}
		result[index] = key
	}
	return result, true
}

func validSelectedDependencyRow(row *schemaRuleReadRow, surface equation.Surface) bool {
	if row == nil {
		return false
	}
	switch row.kind {
	case composition.ReadExact:
		return surface.Form == equation.SurfaceReadExact && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSelect:
		return surface.Form == equation.SurfaceReadSelect && surface.Semantic == surface.Factor && !surface.Normalizer.Available()
	case composition.ReadSummary:
		return surface.Form == equation.SurfaceReadSummary && surface.Semantic == row.semantic &&
			surface.Normalizer == row.normalizer && row.semantic.Available() && row.semantic == row.normalizer
	default:
		return false
	}
}

// anchoredSelectedContent mints the content coordinate of one mounted
// selected read. The preimage is length-framed under its own domain, so no
// pair of distinct anchors shares an encoding.
func anchoredSelectedContent(occurrence equation.Occurrence, operand equation.Operand, ordinal, read uint64) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredSelectedSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(ordinal) != nil || writer.Uint(read) != nil ||
		writer.Finish() != nil {
		return [32]byte{}, false
	}
	content := writer.Sum()
	return content, content != [32]byte{}
}

// anchoredRouteContent is the route-write sibling of anchoredSelectedContent.
func anchoredRouteContent(occurrence equation.Occurrence, operand equation.Operand, ordinal, write, read uint64) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredRouteSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(ordinal) != nil || writer.Uint(write) != nil || writer.Uint(read) != nil ||
		writer.Finish() != nil {
		return [32]byte{}, false
	}
	content := writer.Sum()
	return content, content != [32]byte{}
}

const (
	anchoredSelectedSurfaceDomain = "analysis/engine/selected-surface"
	anchoredRouteSurfaceDomain    = "analysis/engine/route-surface"
	anchoredSurfaceVersion        = 3
)

func writeAnchor(writer *canonical.DigestWriter, occurrence equation.Occurrence, operand equation.Operand) bool {
	occurrenceKey := occurrence.IdentityKey()
	operandKey := operand.IdentityKey()
	return writer.Bytes(occurrenceKey.ID[:]) == nil && writer.Uint(occurrenceKey.Version) == nil &&
		writer.Bytes(operandKey.ID[:]) == nil && writer.Uint(operandKey.Version) == nil
}
