package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

func weakTargetKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func TestEquationRejectsStrongSurfaceAsWeakTarget(t *testing.T) {
	factor := weakTargetKey(1)
	rule := weakTargetKey(2)
	operandFamily := weakTargetKey(3)
	query := weakTargetKey(4)
	cold, coldOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Rules: []composition.Rule{{
			Key:           rule,
			OperandFamily: operandFamily,
			OutputKind:    composition.FactorOutput,
			Output:        factor,
			Inputs:        1,
			Reads:         []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Writes:        []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
		}},
		Queries: []composition.QueryFamily{{
			Key: query, Freezer: weakTargetKey(6), Population: queryschema.PopulationKindSelectedPoint,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !coldOK || cold == nil {
		t.Fatal("cold composition")
	}

	scope := EmptyScope()
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(weakTargetKey(7), scope, TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operand, operandOK := batch.AdmitOperand(occurrence, weakTargetKey(8))
	if !siteOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("source batch")
	}

	read := Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}
	strong := Surface{Factor: factor, Form: SurfaceWriteExact, Local: 1, Mode: TargetModeStrong}
	boundary := BoundaryInput(site, site, weakTargetKey(9), TrueExpr(), IdentityReindex(scope), TrueExpr())
	topology, sealed := SealTopology(cold, TopologySpec{
		Batch: batch,
		Rules: []RuleInstance{{
			Schema: rule, OperandFamily: operandFamily, Occurrence: occurrence, Operand: operand,
			Reads: []ResolvedRead{{Index: 0, Surface: read}}, Writes: []ResolvedWrite{{Index: 0, Surface: strong}},
		}},
		Points:      []PointSpec{{Site: site}},
		Groups:      []Group{{Members: []RuleRef{RuleAt(0)}, Output: PointAt(0), Inputs: []Input{boundary}}},
		Queries:     []QueryInstance{{Context: boundaryContext(10), Family: query, Point: PointAt(0), Surfaces: []Surface{read}}},
		WeakTargets: []WeakTargetMapping{{Surface: strong, Candidates: []Surface{read}}},
	})
	if sealed || topology != nil {
		t.Fatal("strong exact surface was accepted as weak coverage")
	}
}
