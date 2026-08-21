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
	writes  []ruleWriteSurface
	carries uint64
}

func placeSchemaRuleSurfaces[K ~uint32 | ~uint64, V, O any](cell *schemaRuleBindingCellImpl[K, V, O], semantic identity.SemanticKey, anchor ruleSurfaceAnchor, operand O) ([]RuleReadSurface, []ruleWriteSurface, uint64, bool) {
	if cell == nil || !cell.sealedRuleComplete() || !semantic.Available() || cell.impl == nil {
		return nil, nil, 0, false
	}
	hot := cell.impl
	state := cell.state
	authority := state.authority
	ordinal := cell.ordinal
	readCount := uint64(len(hot.reads))
	if hot.writeMode == 0 || (hot.writeMode != directRuleWriteExact && hot.writeMode != directRuleWriteRoute) {
		return nil, nil, 0, false
	}
	reads := make([]RuleReadSurface, readCount)
	readRows := make([]*schemaRuleReadRow, readCount)
	writes := make([]ruleWriteSurface, 1)
	for index := uint64(0); index < readCount; index++ {
		if index >= uint64(len(hot.reads)) || hot.reads[index] == nil {
			return nil, nil, 0, false
		}
		row := hot.reads[index].readRow()
		if row == nil || row.owner != cell || row.ownerOrdinal != ordinal || row.readOrdinal != index {
			return nil, nil, 0, false
		}
		readRows[index] = row
		switch row.kind {
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
			deps := make([]RuleReadSurface, len(row.dependencies))
			for dep, depIndex := range row.dependencies {
				if depIndex >= index || depIndex >= uint64(len(reads)) || !reads[depIndex].value.Available() {
					return nil, nil, 0, false
				}
				deps[dep] = reads[depIndex]
			}
			surface, surfaceOK := anchoredSelectedReadSurface(state, authority, semantic, anchor, row, readRows, deps, reads)
			if !surfaceOK || !surface.value.Available() {
				return nil, nil, 0, false
			}
			reads[index] = surface
		case composition.ReadSummary:
			surface, surfaceOK := readSummarySurface(state, authority, row, operand)
			if !surfaceOK || !surface.value.Available() {
				return nil, nil, 0, false
			}
			reads[index] = surface
		default:
			return nil, nil, 0, false
		}
	}
	var carries uint64
	if hot.carryPresent {
		carries = 1
	}
	switch hot.writeMode {
	case directRuleWriteExact:
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
	case directRuleWriteRoute:
		readRow := (*schemaRuleReadRow)(nil)
		if hot.routeRead > 0 && hot.routeRead-1 < uint64(len(readRows)) {
			readRow = readRows[hot.routeRead-1]
		}
		surface, surfaceOK := anchoredRouteWriteSurface(state, authority, semantic, anchor, ordinal, 0, hot.routeRead, hot.output.schemaFactorSemanticKey(), readRow)
		if !surfaceOK || !surface.value.Available() {
			return nil, nil, 0, false
		}
		writes[0] = surface
		return reads, writes, carries, true
	default:
		return nil, nil, 0, false
	}
}
