package typeauthority_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestArtifactAuthorityResolvesUnresolvedReferenceAsUnknown(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
if true then
  type LocalPoint = {x: number}
end
local p: LocalPoint = {x = 1}
return p
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeReference || row.Resolution() != uint8(staticrefs.Unresolved) {
			continue
		}
		found++
		if _, targetOK := row.ReferenceTarget(); targetOK {
			t.Fatal("unresolved reference retained a target child")
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
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeReference || row.Resolution() != uint8(staticrefs.Declaration) {
			continue
		}
		found++
		childID, childOK := row.ReferenceTarget()
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
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}

	var found [2]bool
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeTypeFunction {
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
	receipt, ok := composite.Global()
	if !ok || !receipt.Available() {
		t.Fatal("global program schema unavailable")
	}
	artifact, ok := composite.CompileArtifact(program, receipt)
	if !ok || artifact == nil || !artifact.Available() {
		t.Fatal("ProgramArtifact compilation failed")
	}
	return artifact
}

func authorityProgram(artifact *programartifact.Artifact) programschema.Program {
	if artifact == nil {
		return programschema.Program{}
	}
	return artifact.Program()
}

func authorityStaticNodeCount(artifact *programartifact.Artifact) int {
	count, _ := authorityProgram(artifact).StaticTypeNodeCount()
	return count
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
			authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
			if err != nil {
				t.Fatal(err)
			}
			values := make(map[string]typ.Type, len(testCase.graph))
			rows := make(map[string]programschema.StaticTypeNode, len(testCase.graph))
			for index := 0; index < authorityStaticNodeCount(artifact); index++ {
				row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
				if !ok || row.Kind() != programschema.StaticNodeAlias {
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

// A Static coordinate is one artifact node, not one declaration. Every node
// inside a recursive declaration is separately addressable, so resolution that
// enters the cycle at an interior node must bind the same fixed point at that
// entry instead of failing for want of a declaration host.
func TestArtifactAuthorityResolvesInteriorNodeOfRecursiveDeclaration(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type Counter = {
    count: number,
    increment: (self: Counter) -> number
}
local c: Counter = {
    count = 0,
    increment = function(self: Counter): number
        return self.count
    end
}
return c
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	interior := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeReference || row.Resolution() == uint8(staticrefs.Unresolved) {
			continue
		}
		interior++
		value, resolved := authority.Resolve(row.ID())
		if !resolved || value == nil {
			t.Fatalf("interior reference row %d of a recursive declaration did not resolve", index)
		}
		recursive, recursiveOK := value.(*typ.Recursive)
		if !recursiveOK || recursive.Body == nil {
			t.Fatalf("interior reference row %d = %T, want a bound *typ.Recursive", index, value)
		}
	}
	if interior == 0 {
		t.Fatal("fixture published no resolved reference rows")
	}
}

// A formal annotation node and every reference to it are lawfully open: the
// closed-declaration recurrence law adjudicates whole declarations, while
// Static admits an open node as a symbolic result at its own boundary.
func TestAuthorityMaterializesOpenFormalNodes(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
local function identity<T>(x: T): T
    return x
end
return identity
`)
	authority, err := typeauthority.SealProgramRows(artifact.CompileKey().ProgramID(), []programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	formals := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeTypeParam {
			continue
		}
		formals++
		ref, refOK := authority.FindByReferenceID(row.ID())
		if !refOK {
			t.Fatalf("formal row %d has no artifact reference", index)
		}
		value, resolved := authority.Resolve(ref)
		if !resolved {
			t.Fatalf("formal row %d did not materialize", index)
		}
		formal, formalOK := value.(*typ.TypeParam)
		if !formalOK || formal.Name != row.Name() {
			t.Fatalf("formal row %d = %T/%v, want the free formal %q", index, value, value, row.Name())
		}
	}
	if formals == 0 {
		t.Fatal("fixture published no formal rows")
	}
}

// An application whose base names no declaration is complete and targetless,
// exactly like the reference it applies: Unknown carries the missing
// information, and no generic binder is fabricated to stand in for it.
func TestArtifactAuthorityResolvesApplicationOfTargetlessReferenceAsUnknown(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
local function route(primary: Missing<number>): number
    return 1
end
return route
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	applications := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeGeneric {
			continue
		}
		applications++
		value, resolved := authority.Resolve(row.ID())
		if !resolved || value != typ.Unknown {
			t.Fatalf("application row %d = %T/%v resolved=%t, want typ.Unknown", index, value, value, resolved)
		}
	}
	if applications != 1 {
		t.Fatalf("fixture published %d application rows, want 1", applications)
	}
}

// An application whose base names a declaration that binds no parameters has
// no generic to apply. The artifact holds a complete, concrete target, so the
// application is malformed rather than targetless and no value is issued.
func TestArtifactAuthorityRejectsApplicationOfConcreteNonGenericBase(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type Plain = number
local function route(primary: Plain<number>): number
    return 1
end
return route
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	applications := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeGeneric {
			continue
		}
		applications++
		value, resolved := authority.Resolve(row.ID())
		if resolved || value != nil {
			t.Fatalf("application row %d = %T/%v resolved=%t, want no value", index, value, value, resolved)
		}
	}
	if applications != 1 {
		t.Fatalf("fixture published %d application rows, want 1", applications)
	}
}

// One fixed point is one type, and the binder that names it belongs to the
// cycle rather than to the node resolution happened to enter through. A cycle
// running through two named declarations is re-entered at an unnamed interior
// node from either entry, so both entries must carry one canonical identity.
func TestArtifactAuthorityMutualDeclarationCycleHasOneCanonicalIdentity(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type A = { next: B }
type B = { next: A }
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		presentation string
		encoded      []byte
	}
	var entries []entry
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeRecord {
			continue
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved {
			t.Fatalf("interior row %d did not resolve", index)
		}
		encoded, encodeErr := typ.EncodeCanonical(context.Background(), value)
		if encodeErr != nil || len(encoded) == 0 {
			t.Fatalf("interior row %d has no canonical identity: %v", index, encodeErr)
		}
		entries = append(entries, entry{presentation: value.String(), encoded: encoded})
	}
	if len(entries) != 2 {
		t.Fatalf("fixture published %d interior rows of the mutual cycle, want 2", len(entries))
	}
	if !bytes.Equal(entries[0].encoded, entries[1].encoded) {
		t.Fatalf("two entries of one fixed point carry two identities:\n  %s\n  %s", entries[0].presentation, entries[1].presentation)
	}
}

// One fixed point is one type. A recursive declaration is reachable from its
// declaration row and from the interior row that carries its structure; both
// entries close the same cycle, so both must carry one canonical identity.
func TestArtifactAuthorityRecursiveFixedPointHasOneCanonicalIdentity(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type Counter = {
    count: number,
    increment: (self: Counter) -> number
}
local c: Counter = {
    count = 0,
    increment = function(self: Counter): number
        return self.count
    end
}
return c
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	identities := make(map[programschema.StaticNodeKind]string, 2)
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || (row.Kind() != programschema.StaticNodeAlias && row.Kind() != programschema.StaticNodeRecord) {
			continue
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved {
			t.Fatalf("row %d (%v) did not resolve", index, row.Kind())
		}
		encoded, encodeErr := typ.EncodeCanonical(context.Background(), value)
		if encodeErr != nil || len(encoded) == 0 {
			t.Fatalf("row %d (%v) has no canonical identity: %v", index, row.Kind(), encodeErr)
		}
		if previous, seen := identities[row.Kind()]; seen && previous != string(encoded) {
			t.Fatalf("two %v rows carry different identities", row.Kind())
		}
		identities[row.Kind()] = string(encoded)
	}
	if len(identities) != 2 {
		t.Fatalf("fixture published %d of the two declaration/interior rows", len(identities))
	}
	if identities[programschema.StaticNodeAlias] != identities[programschema.StaticNodeRecord] {
		t.Fatalf("declaration and interior entries of one fixed point carry two identities:\n  alias  = %q\n  record = %q",
			identities[programschema.StaticNodeAlias], identities[programschema.StaticNodeRecord])
	}
}

// The binder of a cycle belongs to the cycle, not to the rotation the walk
// entered through. Static classifies its concrete rows by the structural
// canonical bytes, which carry the binder name, while Runtime identifies the
// same fixed point by the name-erased encoding. One fixed point re-entered at
// two of its rows must therefore present one binder name: otherwise Static
// admits two classes that Runtime maps onto one row, and the static plane
// seal rejects the pair.
func TestArtifactAuthorityMutualCycleEntriesShareOneBinderName(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type Text = { kind: "text", value: string }
type Group = { kind: "group", children: {Node} }
type Node = Text | Group

local n: Node = {kind = "text", value = "a"}
return n
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	structural := make(map[string]map[string]struct{})
	recursions := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok {
			continue
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved || value == nil || !typ.IsGraphClosed(value) || typ.ContainsTypeParam(value) {
			continue
		}
		if _, isRecursive := value.(*typ.Recursive); isRecursive {
			recursions++
		}
		erased, erasedErr := typ.EncodeCanonicalFormals(context.Background(), value, nil)
		if erasedErr != nil || len(erased) == 0 {
			continue
		}
		exact, exactErr := typ.EncodeCanonical(context.Background(), value)
		if exactErr != nil || len(exact) == 0 {
			t.Fatalf("row %d has no structural canonical identity: %v", index, exactErr)
		}
		if structural[string(erased)] == nil {
			structural[string(erased)] = make(map[string]struct{})
		}
		structural[string(erased)][string(exact)] = struct{}{}
	}
	if recursions == 0 {
		t.Fatal("fixture published no recursive fixed point")
	}
	for erased, exact := range structural {
		if len(exact) == 1 {
			continue
		}
		split := make([]string, 0, len(exact))
		for bytes := range exact {
			split = append(split, bytes)
		}
		t.Fatalf("one fixed point carries %d structural identities:\n  erased = %q\n  %q", len(exact), erased, split)
	}
}

// A member of a mutually recursive declaration group is only closed for the
// entry it was materialized from: its binder sits at that entry. A resolution
// that reaches the group through two different members therefore has to
// materialize each of them at its own entry, otherwise the second member
// carries a binder occurrence whose binder lives under the first and the
// composed value is an open term that no consumer can encode and read back.
func TestArtifactAuthorityGroupReachedAtTwoInteriorEntriesStaysClosed(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type A = { next: B }
type B = { next: A }
type Pair = { left: A, right: B }
local p: Pair = {left = {next = {next = nil}}, right = {next = nil}}
return p
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	standalone := make(map[string][]byte, 2)
	var pair *typ.Record
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok {
			continue
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved {
			continue
		}
		if row.Kind() == programschema.StaticNodeAlias && (row.Name() == "A" || row.Name() == "B") {
			encoded, encodeErr := typ.EncodeCanonical(context.Background(), value)
			if encodeErr != nil {
				t.Fatalf("declaration %q has no canonical identity: %v", row.Name(), encodeErr)
			}
			standalone[row.Name()] = encoded
		}
		if record, isRecord := value.(*typ.Record); isRecord && len(record.Fields) == 2 && record.Fields[0].Name == "left" {
			pair = record
		}
	}
	if pair == nil || len(standalone) != 2 {
		t.Fatalf("fixture published pair=%v declarations=%d, want the group and both declarations", pair != nil, len(standalone))
	}
	for _, field := range pair.Fields {
		if !typ.IsGraphClosed(field.Type) {
			t.Fatalf("field %q is not a closed graph: %s", field.Name, field.Type)
		}
	}
	left, leftErr := typ.EncodeCanonical(context.Background(), pair.Fields[0].Type)
	right, rightErr := typ.EncodeCanonical(context.Background(), pair.Fields[1].Type)
	if leftErr != nil || rightErr != nil {
		t.Fatalf("pair members have no canonical identity: %v / %v", leftErr, rightErr)
	}
	if !bytes.Equal(left, standalone["A"]) {
		t.Fatalf("member reached first carries an identity its declaration does not:\n  %s\n  %s",
			pair.Fields[0].Type, string(standalone["A"]))
	}
	if !bytes.Equal(right, standalone["B"]) {
		t.Fatalf("member reached second carries an identity its declaration does not:\n  %s\n  %s",
			pair.Fields[1].Type, string(standalone["B"]))
	}
	if !typ.TypeEquals(pair.Fields[0].Type, pair.Fields[1].Type) {
		t.Fatalf("two entries of one fixed point are not one type:\n  left  = %s\n  right = %s",
			pair.Fields[0].Type, pair.Fields[1].Type)
	}
}

// A reference is a transparent edge of the type graph: it owns no value and
// never carries a binder. Resolution entered at an interior reference and
// resolution entered at the declaration it names are therefore one type - one
// binder spelling and one canonical identity across resolutions, and one node
// within a resolution.
func TestArtifactAuthorityInteriorEntryPointsNameOneType(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type Counter = {
    count: number,
    increment: (self: Counter) -> number
}
local c: Counter = {
    count = 0,
    increment = function(self: Counter): number
        return self.count
    end
}
return c
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	var declaration identity.ContentID
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if ok && row.Kind() == programschema.StaticNodeAlias && row.Name() == "Counter" {
			declaration = row.ID()
		}
	}
	declared, ok := authority.Resolve(declaration)
	if !ok {
		t.Fatal("declaration row did not resolve")
	}
	declaredBytes, declaredErr := typ.EncodeCanonical(context.Background(), declared)
	if declaredErr != nil {
		t.Fatalf("declaration has no canonical identity: %v", declaredErr)
	}
	references := 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok || row.Kind() != programschema.StaticNodeReference {
			continue
		}
		target, targetOK := row.ReferenceTarget()
		if !targetOK || target != declaration {
			continue
		}
		references++
		value, resolved := authority.Resolve(row.ID())
		if !resolved {
			t.Fatalf("reference row %d did not resolve", index)
		}
		if value.String() != declared.String() {
			t.Fatalf("interior entry spells a second binder:\n  reference   = %s\n  declaration = %s", value, declared)
		}
		encoded, encodeErr := typ.EncodeCanonical(context.Background(), value)
		if encodeErr != nil || !bytes.Equal(encoded, declaredBytes) {
			t.Fatalf("interior entry carries a second identity: %v", encodeErr)
		}
	}
	if references == 0 {
		t.Fatal("fixture published no reference to the recursive declaration")
	}
	recursive, recursiveOK := declared.(*typ.Recursive)
	if !recursiveOK {
		t.Fatalf("declaration = %T, want *typ.Recursive", declared)
	}
	record, recordOK := recursive.Body.(*typ.Record)
	if !recordOK {
		t.Fatalf("declaration body = %T, want *typ.Record", recursive.Body)
	}
	occurrences := 0
	for _, field := range record.Fields {
		function, functionOK := field.Type.(*typ.Function)
		if !functionOK {
			continue
		}
		for _, param := range function.Params {
			if param.Type == typ.Type(recursive) {
				occurrences++
			}
		}
	}
	if occurrences == 0 {
		t.Fatal("interior reference did not materialize as the declaration's own node")
	}
}

// A declaration group that instantiates a generic declared in the same group
// still publishes one sealed declaration. No value reachable from a resolution
// carries an unbound binder or an unwritten generic body.
func TestArtifactAuthorityPublishesNoUnwrittenBody(t *testing.T) {
	artifact := compileArtifactForAuthorityTest(t, `
type List<T> = { head: T, tail: List<T> }
local x: List<number> = {head = 1}
return x
`)
	authority, err := typeauthority.SealPrograms([]programschema.Program{authorityProgram(artifact)})
	if err != nil {
		t.Fatal(err)
	}
	generics, instantiations := 0, 0
	for index := 0; index < authorityStaticNodeCount(artifact); index++ {
		row, ok := authorityProgram(artifact).StaticTypeNodeAt(index)
		if !ok {
			continue
		}
		value, resolved := authority.Resolve(row.ID())
		if !resolved {
			continue
		}
		seen := make(map[typ.Type]bool)
		var walk func(typ.Type)
		walk = func(current typ.Type) {
			if current == nil || seen[current] {
				return
			}
			seen[current] = true
			switch node := current.(type) {
			case *typ.Generic:
				if node.Body == nil {
					t.Fatalf("row %d publishes generic %q with an unwritten body", index, node.Name)
				}
				generics++
				for _, param := range node.TypeParams {
					walk(param)
				}
				walk(node.Body)
			case *typ.Recursive:
				if node.Body == nil {
					t.Fatalf("row %d publishes an unbound binder %q", index, node.Name)
				}
				walk(node.Body)
			case *typ.Instantiated:
				instantiations++
				walk(node.Generic)
				for _, arg := range node.TypeArgs {
					walk(arg)
				}
			case *typ.Record:
				for _, field := range node.Fields {
					walk(field.Type)
				}
			case *typ.Function:
				for _, param := range node.Params {
					walk(param.Type)
				}
				for _, result := range node.Returns {
					walk(result)
				}
			case *typ.TypeParam:
				walk(node.Constraint)
			case *typ.Alias:
				walk(node.Target)
			}
		}
		walk(value)
	}
	if generics == 0 || instantiations == 0 {
		t.Fatalf("fixture published generics=%d instantiations=%d, want both", generics, instantiations)
	}
}
