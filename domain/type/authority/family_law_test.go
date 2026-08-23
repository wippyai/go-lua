package typeauthority_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// familyLawAuthority seals one empty Link-scoped type authority. The family
// law is about the shared vocabulary, not about a Link's own rows, so the
// Link contributes none.
func familyLawAuthority(t *testing.T) *typeauthority.Authority {
	t.Helper()
	artifact := compileArtifactForAuthorityTest(t, "local value = 1\nreturn value\n")
	authority, err := typeauthority.SealProgramRows(artifact.CompileKey().ProgramID(), []programschema.Program{authorityProgram(artifact)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

// familyLawValues is a small closed vocabulary with structure, so a member is
// a multi-node graph rather than a primitive singleton.
func familyLawValues() []typ.Type {
	return []typ.Type{
		typ.String,
		typ.NewArray(typ.Integer),
		typ.NewArray(typ.NewArray(typ.String)),
	}
}

// TestSealedFamilySeedsManyRuntimeSeals is the compute-once law. A sealed
// family is encoded exactly once by its owner and then seeds any number of
// independent Runtime seals. An unsealed receipt cannot do this: its
// construction plane is a linear capability that the first seal consumes, so
// a second seal of the same receipt fails. Sealing every Link's Runtime from
// one family is therefore the observable statement that no Link re-encodes
// the vocabulary.
func TestSealedFamilySeedsManyRuntimeSeals(t *testing.T) {
	family, err := typeauthority.SealFamily("test/family-law", familyLawValues())
	if err != nil {
		t.Fatal(err)
	}
	if family.Count() != len(familyLawValues()) {
		t.Fatalf("family admitted %d members, the vocabulary declares %d", family.Count(), len(familyLawValues()))
	}
	var first []identityPair
	for seal := 0; seal < 3; seal++ {
		authority := familyLawAuthority(t)
		inputs := make([]typeauthority.RuntimeInput, 0, family.Count())
		for index := 0; index < family.Count(); index++ {
			input, ok := family.Input(index, authority)
			if !ok {
				t.Fatalf("seal %d: family member %d refused binding", seal, index)
			}
			inputs = append(inputs, input)
		}
		runtime, inners, err := typeauthority.SealRuntime(authority, inputs)
		if err != nil {
			t.Fatalf("seal %d: %v", seal, err)
		}
		observed := make([]identityPair, 0, len(inners))
		for index, inner := range inners {
			canonicalID, closed := runtime.CanonicalIdentity(inner)
			declared, declaredOK := family.CanonicalIdentity(index)
			if !closed || !declaredOK || canonicalID != declared {
				t.Fatalf("seal %d: member %d sealed as %v, the family declares %v", seal, index, canonicalID, declared)
			}
			observed = append(observed, identityPair{index: index, id: canonicalID})
		}
		if seal == 0 {
			first = observed
			continue
		}
		if len(observed) != len(first) {
			t.Fatalf("seal %d admitted %d members, the first seal admitted %d", seal, len(observed), len(first))
		}
		for index := range observed {
			if observed[index] != first[index] {
				t.Fatalf("seal %d member %d is %v, the first seal sealed %v", seal, index, observed[index], first[index])
			}
		}
	}
}

type identityPair struct {
	index int
	id    identity.ContentID
}

// TestUnsealedReceiptRemainsLinear fences the sharing to sealed families. An
// ordinary admitted input still transfers its construction plane exactly once,
// so nothing but a sealed family may be seeded into a second Runtime.
func TestUnsealedReceiptRemainsLinear(t *testing.T) {
	authority := familyLawAuthority(t)
	input, ok := authority.RuntimeInputForType(typ.NewArray(typ.Integer))
	if !ok {
		t.Fatal("authority refused a closed array input")
	}
	if _, _, err := typeauthority.SealRuntime(authority, []typeauthority.RuntimeInput{input}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := typeauthority.SealRuntime(familyLawAuthority(t), []typeauthority.RuntimeInput{input}); err == nil {
		t.Fatal("an ordinary receipt seeded a second Runtime; its construction plane is a one-shot capability")
	}
}

// TestSealedFamilyRefusesAnOpenRoot keeps the family a closed vocabulary: an
// open root has no Runtime row, so it may not enter the shared plane.
func TestSealedFamilyRefusesAnOpenRoot(t *testing.T) {
	formal := typ.NewTypeParam("T", nil)
	if _, err := typeauthority.SealFamily("test/family-law", []typ.Type{formal}); err == nil {
		t.Fatal("family admitted an open root")
	}
}

// TestSealedFamilyIdentityIsItsContent states that two owners never mint one
// identity for two vocabularies, and that one vocabulary always mints one.
func TestSealedFamilyIdentityIsItsContent(t *testing.T) {
	left, err := typeauthority.SealFamily("test/family-law", familyLawValues())
	if err != nil {
		t.Fatal(err)
	}
	same, err := typeauthority.SealFamily("test/family-law", familyLawValues())
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != same.ContentID() {
		t.Fatal("one vocabulary sealed two identities")
	}
	other, err := typeauthority.SealFamily("test/family-law", []typ.Type{typ.String})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := typeauthority.SealFamily("test/other-owner", familyLawValues())
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == other.ContentID() || left.ContentID() == renamed.ContentID() {
		t.Fatal("two vocabularies share one identity")
	}
}

// TestSealedFamilyRefusesAForeignAuthority keeps the Link fence: only the
// Authority a member is bound to may seal it.
func TestSealedFamilyRefusesAForeignAuthority(t *testing.T) {
	family, err := typeauthority.SealFamily("test/family-law", familyLawValues())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := family.Input(0, nil); ok {
		t.Fatal("family bound a member to no authority")
	}
	input, ok := family.Input(0, familyLawAuthority(t))
	if !ok {
		t.Fatal("family refused a member binding")
	}
	if _, _, err := typeauthority.SealRuntime(familyLawAuthority(t), []typeauthority.RuntimeInput{input}); err == nil {
		t.Fatal("a member bound to one authority sealed under another")
	}
}
