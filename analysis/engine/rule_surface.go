package engine

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// Solver-side binding-surface value constructors; the source admission
// transaction that consumes these surfaces lives in receipt_rule_admission.go.

// RuleReadSurface and RuleWriteSurface are owner-issued exact coordinate
// receipts. The Ref factory preserves the originating Binding authority and
// the source transaction rejects foreign/equal-but-distinct refs.
type RuleReadSurface struct {
	value     equation.Surface
	authority *schemaBindingAuthority
	anchor    *mountedSelectedSurfaceAnchor
	summary   *ruleSummaryMapping
}

type ruleSummaryMapping struct {
	receipt bindingSummarySurfaceReceipt
	surface equation.Surface
	keys    []uint64
}

type RuleWriteSurface struct {
	value     equation.Surface
	targets   []equation.Surface
	relations []equation.CandidateRelation
	authority *schemaBindingAuthority
	route     *SchemaRouteWriteReceipt
	selector  *SchemaSelectWriteReceipt
	anchor    *mountedSelectedSurfaceAnchor
}

func ExactReadSurface[K ~uint32 | ~uint64](ref Ref[K]) (RuleReadSurface, bool) {
	if ref.bindingAuthority == nil || !ref.factorKey.Available() || uint64(ref.raw) == ^uint64(0) {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{value: equation.Surface{Factor: ref.factorKey, Form: equation.SurfaceReadExact, Local: uint64(ref.raw) + 1}, authority: ref.bindingAuthority}, true
}

func ExactWriteSurface[K ~uint32 | ~uint64](ref Ref[K]) (RuleWriteSurface, bool) {
	if ref.bindingAuthority == nil || !ref.factorKey.Available() || uint64(ref.raw) == ^uint64(0) {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: equation.Surface{Factor: ref.factorKey, Form: equation.SurfaceWriteExact, Local: uint64(ref.raw) + 1, Mode: equation.TargetModeStrong}, authority: ref.bindingAuthority}, true
}

func summaryLocal(digest [32]byte) uint64 {
	var value uint64
	for index := 0; index < 8; index++ {
		value |= uint64(digest[index]) << (8 * index)
	}
	if value == 0 {
		value = 1
	}
	return value
}

// SummaryReadSurface consumes a sealed ClosedRefs vector and the exact
// implementation-issued summary proof. Semantic/normalizer identity is read
// from Schema, never supplied by the caller.
func SummaryReadSurface[K ~uint32 | ~uint64](receipt SchemaSummaryReadReceipt, refs *ClosedRefs[K]) (RuleReadSurface, bool) {
	if !receipt.Valid() || refs == nil || !refs.closed || !refs.receipt.valid() || refs.receipt.authority != receipt.fence.authority {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{
		value:     equation.Surface{Factor: refs.receipt.semantic, Form: equation.SurfaceReadSummary, Local: summaryLocal(refs.digest), Semantic: receipt.semantic, Normalizer: receipt.semantic},
		authority: refs.receipt.authority,
		summary: &ruleSummaryMapping{
			receipt: receipt,
			surface: equation.Surface{Factor: refs.receipt.semantic, Form: equation.SurfaceReadSummary, Local: summaryLocal(refs.digest), Semantic: receipt.semantic, Normalizer: receipt.semantic},
			keys: func() []uint64 {
				keys := make([]uint64, len(refs.refs))
				for index, ref := range refs.refs {
					keys[index] = uint64(ref.raw)
				}
				return keys
			}(),
		},
	}, true
}

// SelectedReadSurface consumes the sealed selected-read schema proof and one
// exact output Ref. Dependencies are exact owner surfaces for the declared
// predecessor reads; their order is checked against the sealed Schema.
func SelectedReadSurface[K ~uint32 | ~uint64](receipt SchemaSelectedReadReceipt, ref Ref[K], dependencies []RuleReadSurface) (RuleReadSurface, bool) {
	if !receipt.Valid() || receipt.fence.authority == nil || ref.bindingAuthority != receipt.fence.authority || uint64(ref.raw) == ^uint64(0) || len(dependencies) != int(receipt.dependencyCount) {
		return RuleReadSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() || ref.factorKey != factor {
		return RuleReadSurface{}, false
	}
	for index, dependency := range dependencies {
		readIndex, ok := receipt.fence.schema.ruleReadDependencyAt(receipt.fence.rule, receipt.read, uint64(index))
		shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, readIndex)
		if !ok || !shapeOK || dependency.authority != receipt.fence.authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || dependency.value.Local == 0 || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, false
		}
	}
	return RuleReadSurface{value: equation.Surface{Factor: factor, Form: equation.SurfaceReadSelect, Local: uint64(ref.raw) + 1, Semantic: factor}, authority: ref.bindingAuthority}, true
}

func validSelectedDependencySurface(shape composition.RuleReadShape, surface equation.Surface) bool {
	switch shape.Kind {
	case composition.ReadExact:
		return surface.Form == equation.SurfaceReadExact && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSelect:
		return surface.Form == equation.SurfaceReadSelect && surface.Semantic == surface.Factor && !surface.Normalizer.Available()
	default:
		return false
	}
}

func anchoredSelectedLocal(occurrence equation.Occurrence, operand equation.Operand, receipt SchemaSelectedReadReceipt) uint64 {
	occurrenceKey := occurrence.IdentityKey()
	operandKey := operand.IdentityKey()
	encoded := []byte("analysis/engine/selected-surface/v2")
	encoded = append(encoded, occurrenceKey.ID[:]...)
	encoded = appendUint64(encoded, occurrenceKey.Version)
	encoded = append(encoded, operandKey.ID[:]...)
	encoded = appendUint64(encoded, operandKey.Version)
	encoded = appendUint64(encoded, receipt.fence.rule)
	encoded = appendUint64(encoded, receipt.read)
	digest := sha256.Sum256(encoded)
	return summaryLocal(digest)
}

func anchoredRouteLocal(occurrence equation.Occurrence, operand equation.Operand, receipt SchemaRouteWriteReceipt) uint64 {
	occurrenceKey := occurrence.IdentityKey()
	operandKey := operand.IdentityKey()
	encoded := []byte("analysis/engine/route-surface/v1")
	encoded = append(encoded, occurrenceKey.ID[:]...)
	encoded = appendUint64(encoded, occurrenceKey.Version)
	encoded = append(encoded, operandKey.ID[:]...)
	encoded = appendUint64(encoded, operandKey.Version)
	encoded = appendUint64(encoded, receipt.fence.rule)
	encoded = appendUint64(encoded, receipt.write)
	encoded = appendUint64(encoded, receipt.read)
	digest := sha256.Sum256(encoded)
	return summaryLocal(digest)
}

func appendUint64(encoded []byte, value uint64) []byte {
	for index := uint(0); index < 8; index++ {
		encoded = append(encoded, byte(value>>(index*8)))
	}
	return encoded
}

// SelectorRelation is an opaque proof of one prior target relation. The
// constructor checks every ordinal against the sealed selector shape before
// retaining the private equation relation.
type SelectorRelation struct {
	receipt SchemaSelectWriteReceipt
	value   equation.CandidateRelation
}

func NewSelectorRelation(receipt SchemaSelectWriteReceipt, prior uint64, matches [][]uint64) (SelectorRelation, bool) {
	if !receipt.Valid() || len(matches) != int(receipt.candidateCount) {
		return SelectorRelation{}, false
	}
	found := false
	for index := uint64(0); ; index++ {
		dependency, target, ok := receipt.fence.schema.ruleWriteDependencyAt(receipt.fence.rule, receipt.write, index)
		if !ok {
			break
		}
		if target {
			if found || dependency != prior {
				continue
			}
			found = true
		}
	}
	if !found {
		return SelectorRelation{}, false
	}
	for _, row := range matches {
		previous := uint64(0)
		for index, value := range row {
			if value >= receipt.candidateCount || index > 0 && value <= previous {
				return SelectorRelation{}, false
			}
			previous = value
		}
	}
	return SelectorRelation{receipt: receipt, value: equation.CandidateRelation{Prior: prior, Matches: cloneRelationMatches(matches)}}, true
}

func cloneRelationMatches(values [][]uint64) [][]uint64 {
	result := make([][]uint64, len(values))
	for index, row := range values {
		result[index] = append([]uint64(nil), row...)
	}
	return result
}

func SelectorWriteSurface[K ~uint32 | ~uint64](receipt SchemaSelectWriteReceipt, ref Ref[K], targets []Ref[K], relations []SelectorRelation) (RuleWriteSurface, bool) {
	if !receipt.Valid() || receipt.fence.authority == nil || ref.bindingAuthority != receipt.fence.authority || uint64(ref.raw) == ^uint64(0) || len(targets) != int(receipt.candidateCount) {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() || ref.factorKey != factor || len(relations) == 0 {
		return RuleWriteSurface{}, false
	}
	targetSurfaces := make([]equation.Surface, len(targets))
	for index, target := range targets {
		if target.bindingAuthority != receipt.fence.authority || target.factorKey != factor || uint64(target.raw) == ^uint64(0) {
			return RuleWriteSurface{}, false
		}
		targetSurfaces[index] = equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: uint64(target.raw) + 1, Mode: equation.TargetModeStrong}
	}
	targetDependencies := 0
	for index := uint64(0); ; index++ {
		_, target, ok := receipt.fence.schema.ruleWriteDependencyAt(receipt.fence.rule, receipt.write, index)
		if !ok {
			break
		}
		if target {
			targetDependencies++
		}
	}
	if len(relations) != targetDependencies {
		return RuleWriteSurface{}, false
	}
	resolved := make([]equation.CandidateRelation, len(relations))
	for index, relation := range relations {
		if !relation.receipt.Valid() || relation.receipt.fence.authority != receipt.fence.authority || relation.receipt.write != receipt.write || relation.receipt.fence.rule != receipt.fence.rule {
			return RuleWriteSurface{}, false
		}
		resolved[index] = equation.CandidateRelation{Prior: relation.value.Prior, Matches: cloneRelationMatches(relation.value.Matches)}
	}
	return RuleWriteSurface{value: equation.Surface{Factor: factor, Form: equation.SurfaceWriteSelect, Local: uint64(ref.raw) + 1, Mode: equation.TargetModeStrong, Semantic: receipt.semantic}, targets: targetSurfaces, relations: resolved, authority: ref.bindingAuthority, selector: &receipt}, true
}

func RouteWriteSurface[K ~uint32 | ~uint64](receipt SchemaRouteWriteReceipt, ref Ref[K]) (RuleWriteSurface, bool) {
	if !receipt.Valid() || receipt.fence.authority == nil || ref.bindingAuthority != receipt.fence.authority || uint64(ref.raw) == ^uint64(0) {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() || ref.factorKey != factor {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Local: uint64(ref.raw) + 1}, authority: ref.bindingAuthority, route: &receipt}, true
}
