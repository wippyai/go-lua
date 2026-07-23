package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestBindExternalCallAccessToDeclaredInputsDerivesUndeclaredPoint(t *testing.T) {
	access := []valueAccessTerm{
		{term: 1, point: 7, hasPoint: true},
		{term: 2, point: 3, hasPoint: true},
		{term: 3},
	}
	got := bindExternalCallAccessToDeclaredInputs(access, []cfg.Point{3, 5})
	if len(got) != len(access) || got[0].term != 1 || got[0].point != 3 || !got[0].hasPoint || got[1] != access[1] || got[2] != access[2] {
		t.Fatalf("bound provider access = %#v", got)
	}
	if access[0].point != 7 {
		t.Fatalf("binding mutated compiler access declaration: %#v", access)
	}
}
