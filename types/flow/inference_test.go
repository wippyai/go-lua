package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
)

func TestParameterCondition_RewritesParameterPathsAndDropsLocalState(t *testing.T) {
	paramSym := cfg.SymbolID(17)
	localSym := cfg.SymbolID(23)
	cond := constraint.FromConstraints(
		constraint.Truthy{Path: constraint.Path{Root: "args", Symbol: paramSym}.Field("bad")},
		constraint.Truthy{Path: constraint.Path{Root: "tmp", Symbol: localSym}},
	)

	got := ParameterCondition(cond, []ParamInfo{{Name: "args", Symbol: paramSym}})
	want := constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(0).Field("bad")})

	if !got.Equals(want) {
		t.Fatalf("ParameterCondition() = %v, want %v", got, want)
	}
}

func TestFilterParamConstraints(t *testing.T) {
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)
	symZ := cfg.SymbolID(102)
	localX := cfg.SymbolID(103)

	xPath := constraint.Path{Root: "x", Symbol: symX}
	yPath := constraint.Path{Root: "y", Symbol: symY}
	zPath := constraint.Path{Root: "z", Symbol: symZ}
	localXPath := constraint.Path{Root: "x", Symbol: localX}
	unresolvedXPath := constraint.Path{Root: "x"}

	set := constraint.NewConjunction(
		constraint.NotNil{Path: xPath},
		constraint.NotNil{Path: yPath},
		constraint.NotNil{Path: zPath},
		constraint.Truthy{Path: localXPath},
		constraint.Falsy{Path: unresolvedXPath},
	)

	projection := newParameterProjection([]ParamInfo{
		{Name: "x", Symbol: symX},
		{Name: "y", Symbol: symY},
	})

	filtered := filterParamConstraints(set, projection)
	if len(filtered) != 3 {
		t.Errorf("expected 3 filtered constraints, got %d", len(filtered))
	}

	for _, c := range filtered {
		switch v := c.(type) {
		case constraint.NotNil:
			if v.Path.Symbol == symZ {
				t.Error("z should have been filtered out")
			}
		case constraint.Truthy:
			if v.Path.Symbol == localX {
				t.Error("same root with different nonzero symbol must not fall back by name")
			}
		}
	}
}

func TestSubstituteToPlaceholders(t *testing.T) {
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)

	xPath := constraint.Path{Root: "x", Symbol: symX}
	yPath := constraint.Path{Root: "y", Symbol: symY, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "field"}}}

	set := constraint.NewConjunction(
		constraint.NotNil{Path: xPath},
		constraint.HasType{Path: yPath, Type: narrow.BuiltinTypeKey("string")},
	)

	projection := newParameterProjection([]ParamInfo{
		{Name: "x", Symbol: symX},
		{Name: "y", Symbol: symY},
	})

	substituted := substituteToPlaceholders(set, projection)
	if len(substituted) != 2 {
		t.Errorf("expected 2 substituted constraints, got %d", len(substituted))
	}

	for _, c := range substituted {
		switch v := c.(type) {
		case constraint.NotNil:
			if v.Path.Root != "$0" {
				t.Errorf("expected $0, got %s", v.Path.Root)
			}
			if v.Path.Symbol != 0 {
				t.Errorf("expected Symbol=0 for placeholder, got %d", v.Path.Symbol)
			}
		case constraint.HasType:
			if v.Path.Root != "$1" {
				t.Errorf("expected $1, got %s", v.Path.Root)
			}
			if len(v.Path.Segments) != 1 || v.Path.Segments[0].Name != "field" {
				t.Error("expected .field segment preserved")
			}
			if v.Path.Symbol != 0 {
				t.Errorf("expected Symbol=0 for placeholder, got %d", v.Path.Symbol)
			}
		}
	}
}

func TestParameterCondition_DoesNotRewriteSameNameDifferentSymbol(t *testing.T) {
	paramSym := cfg.SymbolID(100)
	localSym := cfg.SymbolID(200)
	cond := constraint.FromConstraints(
		constraint.NotNil{Path: constraint.Path{Root: "x", Symbol: paramSym}},
		constraint.Truthy{Path: constraint.Path{Root: "x", Symbol: localSym}},
	)

	got := ParameterCondition(cond, []ParamInfo{{Name: "x", Symbol: paramSym}})
	want := constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})

	if !got.Equals(want) {
		t.Fatalf("ParameterCondition() = %v, want %v", got, want)
	}
}

func TestParameterCondition_UsesNameOnlyForUnresolvedPath(t *testing.T) {
	paramSym := cfg.SymbolID(100)
	cond := constraint.FromConstraints(
		constraint.NotNil{Path: constraint.Path{Root: "x"}},
	)

	got := ParameterCondition(cond, []ParamInfo{{Name: "x", Symbol: paramSym}})
	want := constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})

	if !got.Equals(want) {
		t.Fatalf("ParameterCondition() = %v, want %v", got, want)
	}
}

func TestSubstitutePathsInConstraint_EqPath(t *testing.T) {
	symX := cfg.SymbolID(100)
	symY := cfg.SymbolID(101)

	c := constraint.EqPath{
		Left:  constraint.Path{Root: "x", Symbol: symX},
		Right: constraint.Path{Root: "y", Symbol: symY},
	}

	projection := newParameterProjection([]ParamInfo{
		{Name: "x", Symbol: symX},
		{Name: "y", Symbol: symY},
	})
	result := substitutePathsInConstraint(c, projection)

	eq, ok := result.(constraint.EqPath)
	if !ok {
		t.Fatal("expected EqPath")
	}

	if (eq.Left.Root != "$0" && eq.Left.Root != "$1") || (eq.Right.Root != "$0" && eq.Right.Root != "$1") {
		t.Errorf("expected $0 and $1, got %s and %s", eq.Left.Root, eq.Right.Root)
	}
	if eq.Left.Symbol != 0 || eq.Right.Symbol != 0 {
		t.Error("expected Symbol=0 for placeholder paths")
	}
}

func TestSubstitutePathsInConstraint_DropsCalleeLocalPathPair(t *testing.T) {
	symX := cfg.SymbolID(100)
	local := cfg.SymbolID(200)
	c := constraint.EqPath{
		Left:  constraint.Path{Root: "x", Symbol: symX},
		Right: constraint.Path{Root: "tmp", Symbol: local},
	}

	projection := newParameterProjection([]ParamInfo{{Name: "x", Symbol: symX}})
	if got := substitutePathsInConstraint(c, projection); got != nil {
		t.Fatalf("expected non-portable callee-local relation to be dropped, got %v", got)
	}
}

func TestSubstitutePathsInConstraint_KeyOfParamReturn(t *testing.T) {
	symTable := cfg.SymbolID(100)
	c := constraint.KeyOf{
		Table: constraint.Path{Root: "tbl", Symbol: symTable},
		Key:   constraint.RetPath(0),
	}

	projection := newParameterProjection([]ParamInfo{{Name: "tbl", Symbol: symTable}})
	got, ok := substitutePathsInConstraint(c, projection).(constraint.KeyOf)
	if !ok {
		t.Fatalf("expected KeyOf substitution, got %T", got)
	}
	if !got.Table.Equal(constraint.ParamPath(0)) {
		t.Fatalf("table = %v, want %v", got.Table, constraint.ParamPath(0))
	}
	if !constraint.IsReturnPath(got.Key) {
		t.Fatalf("key = %v, want return path", got.Key)
	}
}
