package outputowners

import "testing"

func TestOutputOwnerBuildValidatesEveryGeneratedRelationOwner(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(nil); err == nil {
		t.Fatal("owner evidence validated without the canonical relation denominator")
	}
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Rows) == 0 || current.Digest == "" {
		t.Fatalf("current owner evidence = %#v, want rows and digest", current)
	}
}
