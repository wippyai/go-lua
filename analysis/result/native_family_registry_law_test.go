package result

import (
	"sort"
	"testing"
)

// TestNativeFamilyVocabularyIsPartitionedLaw states the register's shape: the
// implemented half is exactly the closed enum, the declared half names an owner
// and a reason for every row, and no family stands in both halves. A family in
// both would let a name read as answered and as owed at once, which is the
// ambiguity the partition exists to remove.
func TestNativeFamilyVocabularyIsPartitionedLaw(t *testing.T) {
	implemented := NativeFamiliesImplemented()
	if len(implemented) == 0 {
		t.Fatal("implemented native family vocabulary is empty")
	}
	seen := make(map[string]bool, len(implemented))
	for _, name := range implemented {
		if name == "" {
			t.Fatal("implemented native family vocabulary holds an unspelled member")
		}
		if seen[name] {
			t.Fatalf("implemented native family %q is spelled twice", name)
		}
		seen[name] = true
		if !NativeFamilyImplemented(name) {
			t.Fatalf("implemented native family %q does not answer as implemented", name)
		}
		if _, declared := NativeFamilyDeclaredNotImplemented(name); declared {
			t.Fatalf("native family %q is both implemented and declared-not-implemented", name)
		}
	}
	// The enum's extent is stated once; a member added past it would silently
	// leave the read model, so the first ordinal beyond the stated last must
	// have no spelling.
	if beyond := (nativeFamilyImplementedLast + 1).String(); beyond != "" {
		t.Fatalf("native family enum holds the unlisted member %q past its stated extent", beyond)
	}

	declared := NativeFamiliesDeclaredNotImplemented()
	if len(declared) == 0 {
		t.Fatal("declared-not-implemented native family register is empty")
	}
	registered := make(map[string]bool, len(declared))
	for _, row := range declared {
		if row.Family == "" || row.Owner == "" || row.Reason == "" {
			t.Fatalf("declared native family row is incomplete: %+v", row)
		}
		if registered[row.Family] {
			t.Fatalf("declared native family %q is registered twice", row.Family)
		}
		registered[row.Family] = true
		if NativeFamilyImplemented(row.Family) {
			t.Fatalf("declared native family %q is implemented by the closed enum", row.Family)
		}
	}
	if !sort.SliceIsSorted(declared, func(left, right int) bool { return declared[left].Family < declared[right].Family }) {
		t.Fatal("declared native family read model is not in family order")
	}
}

// TestNativeFamilyAnswerClassifiesEveryNameLaw states that the classification
// is total and single-valued: an implemented family answers implemented with no
// register row, a declared family answers declared with its owner, and a name
// in neither half answers unknown rather than quietly resolving to one of them.
func TestNativeFamilyAnswerClassifiesEveryNameLaw(t *testing.T) {
	for _, name := range NativeFamiliesImplemented() {
		status, row := NativeFamilyAnswer(name)
		if status != NativeFamilyStatusImplemented || row != (NativeFamilyDeclared{}) {
			t.Fatalf("implemented family %q answered status=%d row=%+v", name, status, row)
		}
	}
	for _, declared := range NativeFamiliesDeclaredNotImplemented() {
		status, row := NativeFamilyAnswer(declared.Family)
		if status != NativeFamilyStatusDeclared || row != declared {
			t.Fatalf("declared family %q answered status=%d row=%+v", declared.Family, status, row)
		}
	}
	for _, name := range []string{"", "no_such_family", "constant_value_", "CONSTANT_VALUE"} {
		if status, row := NativeFamilyAnswer(name); status != NativeFamilyUnknown || row != (NativeFamilyDeclared{}) {
			t.Fatalf("unregistered family %q answered status=%d row=%+v", name, status, row)
		}
	}
}

// TestNativeFamilyImplementedMatchesPublishedSpellingLaw pins the read model to
// the spelling a published row actually carries. The register's implemented
// half is only an answer about publication if it is the same string the row's
// Family accessor renders.
func TestNativeFamilyImplementedMatchesPublishedSpellingLaw(t *testing.T) {
	implemented := make(map[string]bool, int(nativeFamilyImplementedLast))
	for _, name := range NativeFamiliesImplemented() {
		implemented[name] = true
	}
	for family := nativePublicationFamilyInvalid + 1; family <= nativeFamilyImplementedLast; family++ {
		name := family.String()
		if name == "" {
			t.Fatalf("native family ordinal %d is unspelled inside the closed enum", family)
		}
		if !implemented[name] {
			t.Fatalf("published native family spelling %q is absent from the implemented read model", name)
		}
		if _, ok := family.semanticID(); !ok {
			t.Fatalf("native family %q derives no semantic identity", name)
		}
	}
	if nativePublicationFamilyInvalid.String() != "" {
		t.Fatal("the invalid native family ordinal carries a spelling")
	}
}
