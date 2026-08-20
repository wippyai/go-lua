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
	if mapping == nil {
		return false
	}
	var bindingState *schemaBindingState
	var bindingAuthority *schemaBindingAuthority
	var factor, normalizer composition.Key
	var ok bool
	if mapping.proof != nil {
		if !mapping.proof.summaryReadAt(mapping.read) {
			return false
		}
		shape, shapeOK := mapping.proof.schema.ruleReadShapeAt(mapping.proof.ordinal, mapping.read)
		bindingState, bindingAuthority, factor, normalizer, ok = mapping.proof.state, mapping.proof.bindingAuthority, shape.Factor, shape.Semantic, shapeOK
	} else if mapping.state != nil {
		bindingState, bindingAuthority, factor, normalizer = mapping.state, mapping.authority, mapping.factor, mapping.normalizer
		ok = factor.Available() && normalizer.Available()
	}
	surface := mapping.surface
	return ok && bindingState == state && bindingAuthority == authority && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceReadSummary && surface.Semantic == normalizer && surface.Normalizer == normalizer && surface.Mode == equation.TargetModeNone
}

// RuleReadSurface and RuleWriteSurface are owner-issued sealed coordinate
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
	proof      *ruleRuntimeProof
	read       uint64
	surface    equation.Surface
	keys       []uint64
}

type RuleWriteSurface struct {
	value     equation.Surface
	authority *schemaBindingAuthority
	proof     *ruleRuntimeProof
	write     uint64
	anchored  bool
}

func ExactReadSurface[K ~uint32 | ~uint64](ref Ref[K]) (RuleReadSurface, bool) {
	if !ref.binding.valid() || uint64(ref.raw) >= ref.binding.keyLimit() {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{value: equation.Surface{Factor: ref.binding.semanticKey(), Form: equation.SurfaceReadExact, Local: uint64(ref.raw) + 1}, authority: ref.binding.authority}, true
}

func ExactWriteSurface[K ~uint32 | ~uint64](ref Ref[K]) (RuleWriteSurface, bool) {
	if !ref.binding.valid() || uint64(ref.raw) >= ref.binding.keyLimit() {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: equation.Surface{Factor: ref.binding.semanticKey(), Form: equation.SurfaceWriteExact, Local: uint64(ref.raw) + 1, Mode: equation.TargetModeStrong}, authority: ref.binding.authority}, true
}

// SummaryReadSurface consumes a sealed ClosedRefs vector and the exact
// implementation-issued summary proof. Semantic/normalizer identity is read
// from Schema, never supplied by the caller. The refs digest is the surface
// coordinate at full width, so two distinct key vectors always name two
// distinct summary surfaces.
func SummaryReadSurface[K ~uint32 | ~uint64](proof *ruleRuntimeProof, read uint64, refs *ClosedRefs[K]) (RuleReadSurface, bool) {
	if proof == nil || !proof.summaryReadAt(read) || refs == nil || !refs.closed || !refs.binding.valid() || refs.binding.authority != proof.bindingAuthority {
		return RuleReadSurface{}, false
	}
	shape, shapeOK := proof.schema.ruleReadShapeAt(proof.ordinal, read)
	if !shapeOK || refs.binding.semanticKey() != shape.Factor {
		return RuleReadSurface{}, false
	}
	surface := equation.Surface{Factor: refs.binding.semanticKey(), Form: equation.SurfaceReadSummary, Content: refs.digest, Semantic: shape.Semantic, Normalizer: shape.Semantic}
	if !surface.Available() {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{
		value:     surface,
		authority: refs.binding.authority,
		summary: &ruleSummaryMapping{
			proof:   proof,
			read:    read,
			surface: surface,
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
func SelectedReadSurface[K ~uint32 | ~uint64](proof *ruleRuntimeProof, read uint64, ref Ref[K], dependencies []RuleReadSurface) (RuleReadSurface, bool) {
	if proof == nil || !proof.selectedReadAt(read) || proof.bindingAuthority == nil || ref.binding.authority != nil && ref.binding.authority != proof.bindingAuthority || !ref.binding.valid() || uint64(ref.raw) >= ref.binding.keyLimit() {
		return RuleReadSurface{}, false
	}
	readShape, shapeOK := proof.schema.ruleReadShapeAt(proof.ordinal, read)
	if !shapeOK || len(dependencies) != int(readShape.DependencyCount) || ref.binding.semanticKey() != readShape.Factor {
		return RuleReadSurface{}, false
	}
	for index, dependency := range dependencies {
		readIndex, ok := proof.schema.ruleReadDependencyAt(proof.ordinal, read, uint64(index))
		shape, shapeOK := proof.schema.ruleReadShapeAt(proof.ordinal, readIndex)
		if !ok || !shapeOK || dependency.authority != proof.bindingAuthority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || !dependency.value.LocalAvailable() || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, false
		}
	}
	return RuleReadSurface{value: equation.Surface{Factor: readShape.Factor, Form: equation.SurfaceReadSelect, Local: uint64(ref.raw) + 1, Semantic: readShape.Factor}, authority: ref.binding.authority}, true
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

// anchoredSelectedContent mints the content coordinate of one mounted
// selected read. The preimage is length-framed under its own domain, so no
// pair of distinct anchors shares an encoding.
func anchoredSelectedContent(occurrence equation.Occurrence, operand equation.Operand, proof *ruleRuntimeProof, read uint64) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredSelectedSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(proof.ordinal) != nil || writer.Uint(read) != nil ||
		writer.Finish() != nil {
		return [32]byte{}, false
	}
	content := writer.Sum()
	return content, content != [32]byte{}
}

// anchoredRouteContent is the route-write sibling of anchoredSelectedContent.
func anchoredRouteContent(occurrence equation.Occurrence, operand equation.Operand, proof *ruleRuntimeProof, write, read uint64) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredRouteSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(proof.ordinal) != nil || writer.Uint(write) != nil || writer.Uint(read) != nil ||
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
