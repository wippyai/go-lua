package composition

import "testing"

func TestColdWriteSelectorCandidatesAreFrozenIdentity(t *testing.T) {
	factor, rule, selector := coldKey(41), coldKey(42), coldKey(43)
	seal := func(candidate uint64) *Composition {
		sealed, ok := Seal(Candidate{
			Factors: []Factor{{Key: factor, Forms: []FactorForm{{Kind: FactorSelectorWrite, Semantic: selector}}}},
			Rules: []Rule{{
				Key: rule, OperandFamily: coldKey(249), Admission: coldAdmission(), OutputKind: FactorOutput, Output: factor, Inputs: 1,
				Reads: []Read{
					{Kind: ReadExact, Input: 0, Factor: factor},
					{Kind: ReadExact, Input: 0, Factor: factor},
				},
				Writes: []Write{
					{Kind: WriteExact, Factor: factor},
					{Kind: WriteSelect, Factor: factor, Semantic: selector, Candidates: []uint64{candidate}, Dependencies: []Dependency{{Target: false, Index: 0}}},
				},
			}},
		})
		if !ok || sealed == nil {
			t.Fatal("selector candidate cold seal")
		}
		return sealed
	}
	first, second := seal(0), seal(1)
	if first.ID() == second.ID() {
		t.Fatal("ordered selector candidate read was omitted from cold identity")
	}
	rules := first.Rules()
	if len(rules) != 1 || len(rules[0].Writes) != 2 || len(rules[0].Writes[1].Candidates) != 1 || rules[0].Writes[1].Candidates[0] != 0 {
		t.Fatal("selector candidate read was not retained")
	}
	rules[0].Writes[1].Candidates[0] = 1
	if again := first.Rules(); again[0].Writes[1].Candidates[0] != 0 {
		t.Fatal("selector candidate vector escaped immutable cold schema")
	}
}
