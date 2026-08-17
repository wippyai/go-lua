package static

import "testing"

func TestStaticRowsAPIRejectsNilOwnerAndUnknownTerms(t *testing.T) {
	var rows *Rows
	if err := rows.InterfaceMembers(1, nil); err == nil {
		t.Fatal("nil Rows accepted interface members")
	}
	if owner, ok := (&Rows{}).TypeFunctionScope(0); ok || owner != 0 {
		t.Fatalf("TypeFunctionScope returned %d/%t for an unknown term", owner, ok)
	}
	if exists := (&Rows{}).PublicationExists(1, 0); exists {
		t.Fatal("PublicationExists reported an unknown publication")
	}
}
