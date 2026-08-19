package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Solver-side binding-surface value constructors; the placement envelope that
// accumulates these surfaces lives in runtime_rule_admit.go.

type summarySurfaceBinding interface {
	boundTopologySummarySurface() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool)
}

func validateSummarySurface(binding summarySurfaceBinding, state *schemaBindingState, authority *schemaBindingAuthority, surface equation.Surface) bool {
	bindingState, bindingAuthority, factor, normalizer, ok := binding.boundTopologySummarySurface()
	return ok && bindingState == state && bindingAuthority == authority && surface.Available() && surface.Factor == factor && surface.Form == equation.SurfaceReadSummary && surface.Semantic == normalizer && surface.Normalizer == normalizer && surface.Mode == equation.TargetModeNone
}

// RuleReadSurface and RuleWriteSurface are owner-issued exact coordinate
// receipts. The Ref factory preserves the originating Binding authority and
// the source transaction rejects foreign/equal-but-distinct refs.
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
	binding summarySurfaceBinding
	surface equation.Surface
	keys    []uint64
}

type RuleWriteSurface struct {
	value     equation.Surface
	authority *schemaBindingAuthority
	route     *schemaRouteWrite
	anchored  bool
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

// SummaryReadSurface consumes a sealed ClosedRefs vector and the exact
// implementation-issued summary proof. Semantic/normalizer identity is read
// from Schema, never supplied by the caller. The refs digest is the surface
// coordinate at full width, so two distinct key vectors always name two
// distinct summary surfaces.
func SummaryReadSurface[K ~uint32 | ~uint64](receipt schemaSummaryRead, refs *ClosedRefs[K]) (RuleReadSurface, bool) {
	if !receipt.Valid() || refs == nil || !refs.closed || !refs.receipt.valid() || refs.receipt.authority != receipt.fence.authority {
		return RuleReadSurface{}, false
	}
	surface := equation.Surface{Factor: refs.receipt.semantic, Form: equation.SurfaceReadSummary, Content: refs.digest, Semantic: receipt.semantic, Normalizer: receipt.semantic}
	if !surface.Available() {
		return RuleReadSurface{}, false
	}
	return RuleReadSurface{
		value:     surface,
		authority: refs.receipt.authority,
		summary: &ruleSummaryMapping{
			binding: receipt,
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
func SelectedReadSurface[K ~uint32 | ~uint64](receipt schemaSelectedRead, ref Ref[K], dependencies []RuleReadSurface) (RuleReadSurface, bool) {
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
		if !ok || !shapeOK || dependency.authority != receipt.fence.authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || !dependency.value.LocalAvailable() || !validSelectedDependencySurface(shape, dependency.value) {
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

// anchoredSelectedContent mints the content coordinate of one mounted
// selected read. The preimage is length-framed under its own domain, so no
// pair of distinct anchors shares an encoding.
func anchoredSelectedContent(occurrence equation.Occurrence, operand equation.Operand, receipt schemaSelectedRead) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredSelectedSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(receipt.fence.rule) != nil || writer.Uint(receipt.read) != nil ||
		writer.Finish() != nil {
		return [32]byte{}, false
	}
	content := writer.Sum()
	return content, content != [32]byte{}
}

// anchoredRouteContent is the route-write sibling of anchoredSelectedContent.
func anchoredRouteContent(occurrence equation.Occurrence, operand equation.Operand, receipt schemaRouteWrite) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(anchoredRouteSurfaceDomain, anchoredSurfaceVersion) != nil ||
		!writeAnchor(&writer, occurrence, operand) ||
		writer.Uint(receipt.fence.rule) != nil || writer.Uint(receipt.write) != nil || writer.Uint(receipt.read) != nil ||
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

func RouteWriteSurface[K ~uint32 | ~uint64](receipt schemaRouteWrite, ref Ref[K]) (RuleWriteSurface, bool) {
	if !receipt.Valid() || receipt.fence.authority == nil || ref.bindingAuthority != receipt.fence.authority || uint64(ref.raw) == ^uint64(0) {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() || ref.factorKey != factor {
		return RuleWriteSurface{}, false
	}
	return RuleWriteSurface{value: equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Local: uint64(ref.raw) + 1}, authority: ref.bindingAuthority, route: &receipt}, true
}
