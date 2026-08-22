package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/targetfamily"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// targetFamilyLawProgram compiles one trivial Link program. The law is about
// the target vocabulary every Link shares, so the Link's own rows are minimal.
func targetFamilyLawProgram(t *testing.T) programschema.Program {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "target-family-law.lua", Text: []byte("local value = 1\nreturn value\n")})
	if err != nil || input == nil || !input.Available() {
		t.Fatalf("lower target family fixture: %v", err)
	}
	executionSchema, executionSchemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !executionSchemaOK {
		t.Fatal("target family execution schema")
	}
	artifact, failure := artifactcompiler.CompileDetailed(input, executionSchema, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile target family fixture: %s", failure.Error())
	}
	return artifact.Program()
}

// TestOneSealedTargetFamilySeedsEveryLink is the compute-once law at the seal
// this cut moved. The declaration denominator is a property of the target, so
// it is decoded and canonically encoded once, by the contract that owns it,
// and every Link then seals its own Runtime from that one vocabulary.
//
// The statement is observable rather than a counter: a re-derived vocabulary
// would hand each Link its own construction plane, while a shared one is a
// linear capability that only the first Link could consume. Sealing several
// independent Links off one contract is therefore exactly the claim that no
// Link decodes, clones, or canonically encodes a target type.
func TestOneSealedTargetFamilySeedsEveryLink(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil || target == nil {
		t.Fatalf("seal canonical target: %v", err)
	}
	family, familyOK := targetfamily.Of(target)
	if !familyOK || family.Count() == 0 {
		t.Fatal("the canonical target published no sealed class vocabulary")
	}
	program := targetFamilyLawProgram(t)
	var first []identity.ContentID
	for link := 1; link <= 3; link++ {
		linkID := identity.ContentID{byte(link)}
		types, typesErr := typeauthority.SealProgramRows(linkID, []programschema.Program{program})
		if typesErr != nil || types == nil {
			t.Fatalf("link %d type authority: %v", link, typesErr)
		}
		moduleID := identity.ContentID{byte(link), 9}
		authority, _, sealErr := SealMountedPrograms(MountContext{LinkID: linkID, Target: target}, types,
			[]MountedProgram{{Program: program, ModuleID: moduleID, NamespaceID: moduleID}})
		if sealErr != nil || authority == nil {
			t.Fatalf("link %d Static seal: %v", link, sealErr)
		}
		classes := authority.Classes()
		if classes == nil || !classes.ContentID().Available() {
			t.Fatalf("link %d sealed no ClassSet identity", link)
		}
		observed := make([]identity.ContentID, 0, family.Count())
		for index := 0; index < family.Count(); index++ {
			value, _, rowOK := family.At(index)
			if !rowOK {
				t.Fatalf("link %d: malformed sealed class family row %d", link, index)
			}
			class, classOK := classes.ClassForTarget(target, value)
			if !classOK {
				t.Fatalf("link %d: declared Target type %d has no class", link, index)
			}
			classID, classIDOK := classes.Identity(class)
			if !classIDOK {
				t.Fatalf("link %d: class for Target type %d has no identity", link, index)
			}
			observed = append(observed, classID)
		}
		if link == 1 {
			first = observed
			continue
		}
		// The ClassSet identity itself is Link-scoped by construction. What one
		// sealed vocabulary fixes is the classification: every declared Target
		// type receives the same extensional class in every Link that mounts
		// the target.
		if len(observed) != len(first) {
			t.Fatalf("link %d classified %d declared types, link 1 classified %d", link, len(observed), len(first))
		}
		for index := range observed {
			if observed[index] != first[index] {
				t.Fatalf("link %d classified Target type %d as %s, link 1 classified it as %s",
					link, index, observed[index].String(), first[index].String())
			}
		}
	}
}
