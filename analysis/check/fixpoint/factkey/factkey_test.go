package factkey

import (
	"encoding/base64"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func encode(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }

// TestDeclaredFamiliesResolveTheirSubjects pins that each declared shape names
// what its family states a fact about, in whichever position states it.
func TestDeclaredFamiliesResolveTheirSubjects(t *testing.T) {
	identity := "sealed-table/m/op-00000001"
	for _, item := range []struct {
		key          string
		term         string
		allocation   string
		wantTerm     bool
		wantAllocate bool
	}{
		{key: "heap/table-identity/path/sym2/op-00000001", term: "path/sym2", wantTerm: true},
		{key: "heap/table-closed/" + encode(identity) + "/op-00000001", allocation: identity, wantAllocate: true},
		{key: "heap/member/" + encode(identity) + "/LmE/op-00000001", allocation: identity, wantAllocate: true},
		{key: "heap/member-origin/path/sym2/LmE/op-00000001", term: "path/sym2", wantTerm: true},
		{key: "heap/index-presence/identity/" + encode(identity) + "/" + encode("path/sym7") + "/op-1", allocation: identity, wantAllocate: true},
		{key: "heap/index-presence/term/" + encode("path/sym2") + "/" + encode("path/sym7") + "/op-1", term: "path/sym2", wantTerm: true},
		{key: "heap/length-floor/term/" + encode("path/sym2") + "/op-1", term: "path/sym2", wantTerm: true},
		{key: "heap/index-upper/" + encode("path/sym7") + "/" + encode("path/sym2") + "/op-1", term: "path/sym2", wantTerm: true},
		{key: "heap/index-lower/" + encode("path/sym7") + "/op-1", term: "path/sym7", wantTerm: true},
	} {
		if item.wantTerm {
			anchored, declared := AnchoredAt(item.key, item.term)
			if !declared || !anchored {
				t.Errorf("%q did not resolve term %q (declared=%v)", item.key, item.term, declared)
			}
		}
		if item.wantAllocate {
			allocations, declared := Allocations(item.key)
			if !declared || len(allocations) != 1 || string(allocations[0]) != item.allocation {
				t.Errorf("%q did not resolve allocation %q, got %q (declared=%v)", item.key, item.allocation, allocations, declared)
			}
		}
	}
}

// TestTermSubjectIsNoAllocation pins the distinction the tagged spelling exists
// to make: a container named by path is not an allocation identity, so a walk
// following allocations must not follow it.
func TestTermSubjectIsNoAllocation(t *testing.T) {
	key := "heap/length-floor/term/" + encode("path/sym2") + "/op-1"
	allocations, declared := Allocations(key)
	if !declared || len(allocations) != 0 {
		t.Fatalf("a term-spelled subject resolved as an allocation: %q (declared=%v)", allocations, declared)
	}
}

// TestUndeclaredAndMalformedKeysReportNothing pins the fail-closed edge. A
// family this schema does not declare, and a key that does not have its declared
// family's shape, are both left to the consumer's own rule rather than being
// read as something they are not.
func TestUndeclaredAndMalformedKeysReportNothing(t *testing.T) {
	for _, key := range []string{
		"unknown/path/sym2/op-00000001",
		"heap/table-closed/op-00000001",
		"heap/member/" + encode("x") + "/op-00000001",
		"heap/index-presence/other/" + encode("x") + "/" + encode("y") + "/op-1",
		"heap/table-identity//op-00000001",
	} {
		if _, declared := AnchoredAt(key, "path/sym2"); declared {
			t.Errorf("%q was read as a declared shape", key)
		}
		if _, declared := Allocations(key); declared {
			t.Errorf("%q was read as a declared shape for allocations", key)
		}
	}
}

// TestBranchGuardAndProofShareOneDeclaration pins that the guard a proof states
// and the guard a decision carries are the same string, built once.
func TestBranchGuardAndProofShareOneDeclaration(t *testing.T) {
	proof, ok := ParseBranchProof("branch-proof/2f2f/op-00000003/true")
	if !ok {
		t.Fatal("a well-formed branch proof did not parse")
	}
	if proof.Body != "2f2f" || proof.Name != "op-00000003" || !proof.TrueEdged() {
		t.Fatalf("branch proof resolved to %+v", proof)
	}
	if got, want := proof.Key().String(), "branch-proof/2f2f/op-00000003/true"; got != want {
		t.Fatalf("branch proof key = %q, want %q", got, want)
	}
	guard, ok := ParseBranchGuard(proof.Encoding())
	if !ok || guard != proof.BranchGuard {
		t.Fatalf("the guard a proof encodes did not read back: %+v (%v)", guard, ok)
	}
	for _, bad := range []string{
		"branch-proof/2f2f/op-00000003/maybe",
		"branch-proof//op-00000003/true",
		"branch-proof/2f2f/true",
		"branch-proof/2f2f/a/b/true",
	} {
		if _, ok := ParseBranchProof(bad); ok {
			t.Errorf("%q parsed as a branch proof", bad)
		}
	}
	for _, bad := range []string{"front/branch/op-1/maybe", "front/branch/true", "other/op-1/true"} {
		if _, ok := ParseBranchGuard(bad); ok {
			t.Errorf("%q parsed as a branch guard", bad)
		}
	}
}

func TestBuildKeyOwnsDeclaredFamilySpellings(t *testing.T) {
	identity := []byte("sealed-table/m/op-00000001")
	for _, test := range []struct {
		family Family
		parts  []Part
		want   string
	}{
		{HeapTableIdentity, []Part{TermPart("path/sym2")}, "heap/table-identity/path/sym2/op-1"},
		{HeapTableClosed, []Part{IdentityPart(identity)}, "heap/table-closed/" + encode(string(identity)) + "/op-1"},
		{HeapMember, []Part{IdentityPart(identity), EncodedOpaquePart(".a")}, "heap/member/" + encode(string(identity)) + "/LmE/op-1"},
		{HeapIndexPresence, []Part{TaggedTermPart([]byte("path/sym2")), EncodedTermPart([]byte("path/sym7"))}, "heap/index-presence/term/" + encode("path/sym2") + "/" + encode("path/sym7") + "/op-1"},
		{HeapIndexUpper, []Part{EncodedTermPart([]byte("path/sym7")), EncodedTermPart([]byte("path/sym2"))}, "heap/index-upper/" + encode("path/sym7") + "/" + encode("path/sym2") + "/op-1"},
		{Value, []Part{TermPart("path/sym2")}, "value/path/sym2/op-1"},
		{CallResult, []Part{CoordinatePart("op-1")}, "call-result/op-1/op-1"},
		{CallArgument, []Part{CoordinatePart("op-1")}, "call-argument/op-1/op-1"},
		{LocalCallResult, []Part{TermPart("path/sym2")}, "local-call-result/path/sym2/op-1"},
		{Type, []Part{TermPart("path/sym2")}, "type/path/sym2/op-1"},
		{DeclaredType, []Part{TermPart("path/sym2")}, "declared-type/path/sym2/op-1"},
		{SummaryType, []Part{TermPart("path/sym2")}, "summary-type/path/sym2/op-1"},
		{MethodReturnSummary, []Part{TermPart("path/sym2")}, "method-return-summary/path/sym2/op-1"},
		{IteratorElement, []Part{TermPart("path/sym2")}, "iterator-element/path/sym2/op-1"},
		{IteratorKey, []Part{TermPart("path/sym2")}, "iterator-key/path/sym2/op-1"},
		{IteratorKeySource, []Part{TermPart("path/sym2")}, "iterator-key-source/path/sym2/op-1"},
		{NativeConstantValue, []Part{OpaquePart("2f2f")}, "constant_value/2f2f/op-1"},
		{NativePublicationIdentity, []Part{OpaquePart("2f2f")}, "publication_identity/2f2f/op-1"},
		{NativeBranchPartition, []Part{OpaquePart("2f2f")}, "branch_partition/2f2f/op-1"},
		{NativeTruthinessClass, []Part{OpaquePart("2f2f")}, "truthiness_class/2f2f/op-1"},
		{BranchResidueClass, []Part{EncodedTermPart([]byte("path/sym2"))}, "branch-residue-class/" + encode("path/sym2") + "/op-1"},
		{NativeConcatSite, []Part{OpaquePart("2f2f")}, "concat_site/2f2f/op-1"},
		{NativeBuiltinCall, []Part{OpaquePart("2f2f"), OpaquePart("op-7"), OpaquePart("contract-revocation")}, "builtin_call/2f2f/op-7/contract-revocation/op-1"},
		{HeapAllocationDisplay, []Part{IdentityPart(identity)}, "heap/allocation-display/" + encode(string(identity)) + "/op-1"},
		{NativeAliasDisjoint, []Part{TermPart("path/sym2"), IdentityPart(identity)}, "alias_disjoint/path/sym2/" + encode(string(identity)) + "/op-1"},
	} {
		key := BuildKey(test.family, test.parts, "op-1")
		if key.String() != test.want {
			t.Errorf("%s key = %q, want %q", test.family.Prefix, key.String(), test.want)
			continue
		}
		parsed, ok := test.family.ParseKey(key.String())
		if !ok || parsed.Occurrence != "op-1" || parsed.QualifierCount() != len(test.parts)-1 {
			t.Errorf("%s did not parse its built key: %+v (%v)", test.family.Prefix, parsed, ok)
		}
	}
}

func TestBuildKeyAcceptsStructuralPathKey(t *testing.T) {
	space := keyspace.New()
	path, ok := space.FromStateKey(pathdom.PathKey("sym2"))
	if !ok {
		t.Fatal("could not intern path subject")
	}
	key := BuildKey(HeapTableIdentity, []Part{PathPart(path)}, "op-1")
	if got, want := key.String(), "heap/table-identity/path/sym2/op-1"; got != want {
		t.Fatalf("structural path key = %q, want %q", got, want)
	}
}

func TestBuildKeyProducesTypedPrefixes(t *testing.T) {
	identity := []byte("heap")
	prefix := BuildKey(HeapMember, []Part{IdentityPart(identity)}, "")
	if got, want := prefix.String(), "heap/member/aGVhcA/"; got != want {
		t.Fatalf("subject prefix = %q, want %q", got, want)
	}
	if family, ok := prefix.Family(); !ok || family.ID != FamilyHeapMember {
		t.Fatalf("prefix family = %+v (%v)", family, ok)
	}
}

func TestTerminalTermSubjectOwnsNestedTermSyntax(t *testing.T) {
	key := BuildKey(Value, []Part{TermPart(`scalar/claim/claim-kind/3/"any"`)}, "op-1")
	if got, want := key.String(), `value/scalar/claim/claim-kind/3/"any"/op-1`; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
	parsed, ok := Value.ParseKey(key.String())
	if !ok || parsed.Subject.Spelling() != `scalar/claim/claim-kind/3/"any"` || parsed.Occurrence != "op-1" {
		t.Fatalf("parsed = %+v, %v", parsed, ok)
	}
}

func TestFamiliesAreCompleteRecords(t *testing.T) {
	if len(families) != 44 {
		t.Fatalf("declared families = %d, want 44", len(families))
	}
	for _, family := range families {
		if family.ID == 0 || family.Prefix == "" || family.RevocationSet == nil ||
			family.PayloadKind > PayloadRelation {
			t.Errorf("incomplete family record: %+v", family)
		}
		if declared, ok := Lookup(family.Prefix + "subject/op"); !ok || declared.ID != family.ID {
			t.Errorf("%s is absent from the family lookup", family.Prefix)
		}
		for _, revoker := range family.RevocationSet {
			if _, ok := byID[revoker]; !ok {
				t.Errorf("%s names undeclared revoker %d", family.Prefix, revoker)
			}
		}
	}
}
