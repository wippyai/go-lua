// runtime_program_admit.go states one sealed rule cell as a declaration. A
// rule publishes two pure projections of an issuance: the canonical operand
// its owner resolves from the neutral coordinates, and the read/carry/write
// surfaces its cold shape places over that operand. Neither reaches
// construction state: the engine mints the source capabilities, hands the
// declaration its sealed anchor, and folds the returned values.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ruleSurfaceAnchor is the sealed source anchor of one issuance. The engine
// mints both capabilities before any declaration is asked for surfaces, so an
// occurrence-anchored surface is derived from an anchor the declaration
// cannot choose.
type ruleSurfaceAnchor struct {
	occurrence equation.Occurrence
	operand    equation.Operand
}

// declaredRuleOperand is one issuance's canonical operand, erased. digest is
// the owner's canonical content identity for it; the engine mints the Operand
// capability from that digest and never inspects the typed value.
type declaredRuleOperand struct {
	value  any
	digest [32]byte
}

func (declared declaredRuleOperand) Available() bool {
	return declared.value != nil && declared.digest != [32]byte{}
}

// declaredRuleSurfaces is one issuance's complete surface declaration, in
// cold shape order.
type declaredRuleSurfaces struct {
	reads   []RuleReadSurface
	writes  []RuleWriteSurface
	carries uint64
}

func (implementation *RuleImplementation[K, V, O]) AdmitsMounted(mount, point, occurrence identity.ContentID) bool {
	if implementation == nil {
		return false
	}
	_, resolved := implementation.resolveOperand(OperandCoords{Mount: mount, Point: point, Occurrence: occurrence})
	return resolved
}

// declareRuleOperand resolves the neutral coordinates to this rule's typed
// operand and canonicalizes it. The typed value never leaves the cell in a
// nameable form: it travels back to declareRuleSurfaces erased.
func (implementation *RuleImplementation[K, V, O]) declareRuleOperand(coords OperandCoords) (declaredRuleOperand, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		return declaredRuleOperand{}, false
	}
	operand, resolved := implementation.resolveOperand(coords)
	if !resolved {
		return declaredRuleOperand{}, false
	}
	_, digest, contentOK := implementation.binding.cell.impl.operandContent(operand)
	if !contentOK || digest == [32]byte{} {
		return declaredRuleOperand{}, false
	}
	return declaredRuleOperand{value: operand, digest: digest}, true
}

// declareRuleSurfaces places the cold shape's surfaces over one issuance. It
// reads only the sealed schema shape, the owner's declared per-slot
// projectors, and the anchor the engine minted.
func (implementation *RuleImplementation[K, V, O]) declareRuleSurfaces(declared declaredRuleOperand, anchor ruleSurfaceAnchor) (declaredRuleSurfaces, bool) {
	operand, typed := declared.value.(O)
	if implementation == nil || !typed || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		return declaredRuleSurfaces{}, false
	}
	semantic, semanticOK := semanticKeyFromComposition(implementation.binding.proof.semantic)
	if !semanticOK {
		return declaredRuleSurfaces{}, false
	}
	reads, writes, carries, ok := implementation.placeSurfaces(semantic, anchor, operand)
	if !ok {
		return declaredRuleSurfaces{}, false
	}
	return declaredRuleSurfaces{reads: reads, writes: writes, carries: carries}, true
}

func (implementation *RuleImplementation[K, V, O]) placeSurfaces(semantic identity.SemanticKey, anchor ruleSurfaceAnchor, operand O) ([]RuleReadSurface, []RuleWriteSurface, uint64, bool) {
	if implementation == nil || !semantic.Available() || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		return nil, nil, 0, false
	}
	hot := implementation.binding.cell.impl
	state := implementation.binding.state
	authority := implementation.binding.authority
	ordinal := implementation.binding.proof.ordinal
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
	if !shapeOK || uint64(len(hot.reads)) != shape.ReadCount || shape.WriteCount != 1 {
		return nil, nil, 0, false
	}
	reads := make([]RuleReadSurface, shape.ReadCount)
	writes := make([]RuleWriteSurface, shape.WriteCount)
	for index := uint64(0); index < shape.ReadCount; index++ {
		readShape, readOK := state.schema.ruleReadShapeAt(ordinal, index)
		if !readOK {
			return nil, nil, 0, false
		}
		switch readShape.Kind {
		case composition.ReadExact:
			local, projected := hot.reads[index].projectLocal(operand)
			factor := hot.reads[index].exactAdmitFactor()
			if !projected || factor == nil {
				return nil, nil, 0, false
			}
			surface, surfaceOK := factor.schemaFactorExactRead(state, authority, local)
			if !surfaceOK || !surface.value.Available() {
				return nil, nil, 0, false
			}
			reads[index] = surface
		case composition.ReadSelect:
			deps := make([]RuleReadSurface, readShape.DependencyCount)
			for dep := uint64(0); dep < readShape.DependencyCount; dep++ {
				depIndex, depOK := state.schema.ruleReadDependencyAt(ordinal, index, dep)
				if !depOK || depIndex >= uint64(index) || !reads[depIndex].value.Available() {
					return nil, nil, 0, false
				}
				deps[dep] = reads[depIndex]
			}
			proofOK := implementation.selectedRead(index)
			surface, surfaceOK := anchoredSelectedReadSurface(state, authority, semantic, anchor, implementation.binding.proof, index, deps, reads)
			if !proofOK || !surfaceOK || !surface.value.Available() {
				return nil, nil, 0, false
			}
			reads[index] = surface
		case composition.ReadSummary:
			proofOK := implementation.summaryRead(index)
			provider, providerOK := hot.reads[index].(interface{ summarySurfaceAdmit() any })
			if !proofOK || !providerOK {
				return nil, nil, 0, false
			}
			project, projectOK := provider.summarySurfaceAdmit().(func(any) (any, bool))
			if !projectOK || project == nil {
				return nil, nil, 0, false
			}
			refs, refsOK := project(operand)
			surface, surfaceOK := readSummarySurface(implementation.binding.proof, index, refs)
			if !refsOK || !surfaceOK || !surface.value.Available() {
				return nil, nil, 0, false
			}
			reads[index] = surface
		default:
			return nil, nil, 0, false
		}
	}
	var carries uint64
	if shape.CarryCount == 1 {
		carries = 1
	} else if shape.CarryCount != 0 {
		return nil, nil, 0, false
	}
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ordinal, 0)
	if !writeOK {
		return nil, nil, 0, false
	}
	switch writeShape.Kind {
	case composition.WriteExact:
		local, projected := hot.projectWrite(operand)
		if !projected || hot.output == nil {
			return nil, nil, 0, false
		}
		surface, surfaceOK := hot.output.schemaFactorExactWrite(state, authority, local)
		if !surfaceOK || !surface.value.Available() {
			return nil, nil, 0, false
		}
		writes[0] = surface
		return reads, writes, carries, true
	case composition.WriteRoute:
		_, proofOK := implementation.routeWrite()
		surface, surfaceOK := anchoredRouteWriteSurface(state, authority, semantic, anchor, implementation.binding.proof, 0)
		if !proofOK || !surfaceOK || !surface.value.Available() {
			return nil, nil, 0, false
		}
		writes[0] = surface
		return reads, writes, carries, true
	default:
		return nil, nil, 0, false
	}
}
