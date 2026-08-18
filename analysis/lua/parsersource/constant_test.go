package parsersource

import "testing"

func TestASTConstantDiscoveryResolvesZeroAndNonZeroDiscriminants(t *testing.T) {
	root := moduleRoot(t)
	rows, err := DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("AST constant discovery returned no decidable constants")
	}
	var zero, nonZero bool
	for _, row := range rows {
		if row.Name == "AttrKeyUnknown" && row.Zero {
			zero = true
		}
		if row.Name == "AttrKeyDot" && !row.Zero {
			nonZero = true
		}
	}
	if !zero || !nonZero {
		t.Fatalf("constant rows omit expected zero/non-zero lexer discriminants: %#v", rows)
	}
}

func TestAdmittedFamiliesAreTheClosedConstantTypes(t *testing.T) {
	root := moduleRoot(t)
	constants, err := DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	counted := make(map[string]int)
	for _, constant := range constants {
		if constant.Type != "" {
			counted[constant.Type]++
		}
	}
	if len(counted) == 0 {
		t.Fatal("no discovered constant states a named type, so no carrier can be refined")
	}
	families := DiscriminantEnums(constants)
	admitted := make(map[string]DiscriminantEnum, len(families))
	for _, family := range families {
		admitted[family.Type] = family
	}
	for name, members := range counted {
		family, isAdmitted := admitted[name]
		if members < 2 {
			if isAdmitted {
				t.Fatalf("family %s is admitted with %d member, so a carrier of it would be refined by a choice it never makes", name, members)
			}
			continue
		}
		if !isAdmitted {
			t.Fatalf("named constant type %s declares %d members and is not admitted", name, members)
		}
		if len(family.Members) != members {
			t.Fatalf("family %s states %d members, the declarations state %d", name, len(family.Members), members)
		}
	}
	if len(admitted) != 3 {
		t.Fatalf("the AST declares %d closed constant families, want 3: %v", len(admitted), families)
	}
	keySyntax, known := admitted["AttrKeySyntax"]
	if !known || keySyntax.Zero != "AttrKeyUnknown" {
		t.Fatalf("AttrKeySyntax zero member = %q/%v, want AttrKeyUnknown", keySyntax.Zero, known)
	}
	memberKind, known := admitted["InterfaceMemberKind"]
	if !known || memberKind.Zero != "" {
		t.Fatalf("InterfaceMemberKind zero member = %q/%v, want none", memberKind.Zero, known)
	}
}
