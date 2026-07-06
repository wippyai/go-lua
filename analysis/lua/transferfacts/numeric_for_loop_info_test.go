package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func TestNumericForLoopInfoFromFact(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantFloor int64
	}{
		{
			name:      "positive step uses init floor and limit array",
			src:       "function f(xs: {string}) for i = 1, #xs do end end",
			wantFloor: 1,
		},
		{
			name:      "negative step uses limit floor and init array",
			src:       "function f(xs: {string}) for i = #xs, 1, -1 do end end",
			wantFloor: 1,
		},
		{
			name: "array path can be known without a positive floor",
			src:  "function f(xs: {string}, start: number) for i = start, #xs do end end",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn, bindings, built, result := parseSemanticFunction(t, tc.src)
			fact := firstNumericForCheckFact(t, built, result)
			iPath := path.NewPath(fact.Symbol, "i")
			wantArray := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "xs")
			info, ok := numericForLoopInfoFromFact(fact, bindings)
			if !ok {
				t.Fatal("missing loop info")
			}
			if !info.indexPath.Equal(iPath) {
				t.Fatalf("index path = %v, want %v", info.indexPath, iPath)
			}
			if !info.hasArrayPath || !info.arrayPath.Equal(wantArray) {
				t.Fatalf("array path = %v/%v, want %v", info.arrayPath, info.hasArrayPath, wantArray)
			}
			if tc.wantFloor == 0 {
				if info.hasIndexFloor {
					t.Fatalf("unexpected index floor: %#v", info)
				}
				return
			}
			if !info.hasIndexFloor || info.indexFloor != tc.wantFloor {
				t.Fatalf("index floor = %d/%v, want %d", info.indexFloor, info.hasIndexFloor, tc.wantFloor)
			}
		})
	}
}

func firstNumericForCheckFact(t *testing.T, built *cfgbuild.Result, result *semantics.Result) cfgfacts.NumericForFact {
	t.Helper()
	for _, point := range built.Graph.RPO() {
		fact, ok := result.NumericFor(point)
		if ok && fact.Role == cfgfacts.NumericForRoleCheck {
			return fact
		}
	}
	t.Fatal("missing numeric-for check fact")
	return cfgfacts.NumericForFact{}
}
