package typeauthority_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/lower"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestArtifactAuthorityResolvesUnresolvedReferenceAsUnknown(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
if true then
  type LocalPoint = {x: number}
end
local p: LocalPoint = {x = 1}
return p
`)
	authority, err := typeauthority.SealArtifacts([]*programartifact.Artifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		row, ok := artifact.StaticTypeNodeAt(index)
		if !ok || row.Kind() != programartifact.StaticNodeReference || row.Resolution() != uint8(programstatic.TypeRefUnresolved) {
			continue
		}
		found++
		if row.ChildCount() != 0 {
			t.Fatalf("unresolved reference retained %d target children", row.ChildCount())
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved || value != typ.Unknown {
			t.Fatalf("unresolved reference resolved as %T/%v, want typ.Unknown", value, resolved)
		}
		fresh, freshOK := authority.Resolve(row.ID())
		if !freshOK || fresh != typ.Unknown {
			t.Fatal("unresolved reference did not deterministically replay Unknown")
		}
	}
	if found != 1 {
		t.Fatalf("unresolved reference rows=%d, want 1", found)
	}
}

func TestArtifactAuthorityResolvedReferencePreservesSoleTarget(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type DeclaredPoint = {x: number}
local p: DeclaredPoint = {x = 1}
return p
`)
	authority, err := typeauthority.SealArtifacts([]*programartifact.Artifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		row, ok := artifact.StaticTypeNodeAt(index)
		if !ok || row.Kind() != programartifact.StaticNodeReference || row.Resolution() != uint8(programstatic.TypeRefDeclaration) {
			continue
		}
		found++
		if row.ChildCount() != 1 {
			t.Fatalf("declaration reference retained %d target children, want 1", row.ChildCount())
		}
		childID, childOK := row.ChildAt(0)
		if !childOK {
			t.Fatal("declaration reference sole child unavailable")
		}
		value, resolved := authority.Resolve(row.ID())
		childValue, childResolved := authority.Resolve(childID)
		if !resolved || !childResolved || !typ.TypeEquals(value, childValue) {
			t.Fatalf("declaration reference did not preserve its sole target: reference=%T/%v child=%T/%v", value, resolved, childValue, childResolved)
		}
	}
	if found != 1 {
		t.Fatalf("declaration reference rows=%d, want 1", found)
	}
}

func TestArtifactAuthorityResolvesOmittedAndAuthoredEmptyFunctionReturns(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
interface Service
  function omitted<T: string>(value: T)
  function empty<T: string>(value: T): ()
end
`)
	authority, err := typeauthority.SealArtifacts([]*programartifact.Artifact{artifact})
	if err != nil {
		t.Fatal(err)
	}

	var found [2]bool
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		row, ok := artifact.StaticTypeNodeAt(index)
		if !ok || row.Kind() != programartifact.StaticNodeTypeFunction {
			continue
		}
		known := row.ReturnsKnown()
		caseIndex := 0
		if known {
			caseIndex = 1
		}
		if found[caseIndex] {
			t.Fatalf("duplicate TypeFunction row for returns-known=%v", known)
		}
		found[caseIndex] = true

		value, ok := authority.Resolve(row.ID())
		if !ok {
			t.Fatalf("Resolve(TypeFunction returns-known=%v) failed", known)
		}
		function, ok := value.(*typ.Function)
		if !ok {
			t.Fatalf("TypeFunction returns-known=%v = %T, want *typ.Function", known, value)
		}
		if len(function.TypeParams) != 1 || len(function.Params) != 1 {
			t.Fatalf("TypeFunction returns-known=%v binder/params = %d/%d, want 1/1", known, len(function.TypeParams), len(function.Params))
		}
		formal := function.TypeParams[0]
		if formal.Constraint != typ.String || function.Params[0].Type != formal {
			t.Fatalf("TypeFunction returns-known=%v did not preserve nested binder/param identity: formal=%v param=%v", known, formal.Constraint, function.Params[0].Type)
		}
		if len(function.Returns) != 0 {
			t.Fatalf("TypeFunction returns-known=%v returns=%v, want empty", known, function.Returns)
		}

		fresh, ok := authority.Resolve(row.ID())
		if !ok {
			t.Fatalf("second Resolve(TypeFunction returns-known=%v) failed", known)
		}
		freshFunction, ok := fresh.(*typ.Function)
		if !ok || freshFunction == function {
			t.Fatalf("Resolve(TypeFunction returns-known=%v) did not return a fresh function graph", known)
		}
		function.Params[0].Type = typ.Number
		function.TypeParams[0].Constraint = typ.Number
		if freshFunction.Params[0].Type != freshFunction.TypeParams[0] || freshFunction.TypeParams[0].Constraint != typ.String {
			t.Fatalf("Resolve(TypeFunction returns-known=%v) leaked mutation across sessions", known)
		}
	}
	if !found[0] || !found[1] {
		t.Fatalf("found TypeFunction rows omitted/authored-empty = %v", found)
	}
}

func compileArtifactForAuthorityTest(t testing.TB, source string) *programartifact.Artifact {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "artifact-authority.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := programschema.Global()
	if !ok || !receipt.Available() {
		t.Fatal("global program schema unavailable")
	}
	artifact, ok := schemaadapter.Compile(program.TransformerInput(), receipt)
	if !ok || artifact == nil || !artifact.Available() {
		t.Fatal("ProgramArtifact compilation failed")
	}
	return artifact
}

func TestArtifactAuthorityAliasCyclesPreserveInnerAliases(t *testing.T) {
	cases := []struct {
		name   string
		source string
		graph  map[string][]string
		text   map[string]string
	}{
		{
			name: "mutual",
			source: `
type A = B
type B = A
`,
			graph: map[string][]string{"A": {"B"}, "B": {"A"}},
			text:  map[string]string{"A": "μA. B", "B": "μB. A"},
		},
		{
			name: "three-cycle",
			source: `
type A = B
type B = C
type C = A
`,
			graph: map[string][]string{"A": {"B", "C"}, "B": {"C", "A"}, "C": {"A", "B"}},
			text:  map[string]string{"A": "μA. B", "B": "μB. C", "C": "μC. A"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			artifact := compileArtifactForAuthorityTest(t, testCase.source)
			authority, err := typeauthority.SealArtifacts([]*programartifact.Artifact{artifact})
			if err != nil {
				t.Fatal(err)
			}
			values := make(map[string]typ.Type, len(testCase.graph))
			rows := make(map[string]programartifact.StaticTypeNodeRow, len(testCase.graph))
			for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
				row, ok := artifact.StaticTypeNodeAt(index)
				if !ok || row.Kind() != programartifact.StaticNodeAlias {
					continue
				}
				value, resolved := authority.Resolve(row.ID())
				if !resolved {
					t.Fatalf("alias %q did not resolve", row.Name())
				}
				values[row.Name()] = value
				rows[row.Name()] = row
			}
			if len(values) != len(testCase.graph) {
				t.Fatalf("resolved aliases=%v, want %v", values, testCase.graph)
			}
			for name, names := range testCase.graph {
				value := values[name]
				recursive, ok := value.(*typ.Recursive)
				if !ok {
					t.Fatalf("alias %q = %T, want *typ.Recursive", name, value)
				}
				if got := value.String(); got != testCase.text[name] {
					t.Fatalf("alias %q String()=%q, want %q", name, got, testCase.text[name])
				}
				fresh, freshOK := authority.Resolve(rows[name].ID())
				if !freshOK || !typ.TypeEquals(value, fresh) {
					t.Fatalf("alias %q changed TypeEquals across repeated observation", name)
				}
				current := recursive.Body
				for _, wantName := range names {
					alias, aliasOK := current.(*typ.Alias)
					if !aliasOK || alias.Name != wantName {
						t.Fatalf("alias %q graph node=%T/%v, want inner alias %q", name, current, aliasOK, wantName)
					}
					current = alias.Target
				}
				if current != recursive {
					t.Fatalf("alias %q graph did not close on its own recursive root", name)
				}
			}
		})
	}
}
