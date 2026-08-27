package definition

import "testing"

// TestDefinitionCloneOwnsRelationCorrespondences is the hostile ownership law:
// cloning a source definition must give each relation its own correspondence
// storage, so edits on either definition cannot alter the other.
func TestDefinitionCloneOwnsRelationCorrespondences(t *testing.T) {
	source := correspondingBase()
	clone := source.Clone()

	if len(source.Relations) != 1 || len(clone.Relations) != 1 {
		t.Fatalf("source/clone relation counts = %d/%d, want one each", len(source.Relations), len(clone.Relations))
	}
	if len(source.Relations[0].Correspondences) != 1 || len(clone.Relations[0].Correspondences) != 1 {
		t.Fatalf("source/clone correspondence counts = %d/%d, want one each", len(source.Relations[0].Correspondences), len(clone.Relations[0].Correspondences))
	}

	clone.Relations[0].Correspondences[0].Member = "correspondent/clone"
	if got := source.Relations[0].Correspondences[0].Member; got != "correspondent/candidates" {
		t.Fatalf("mutating clone correspondence changed source to %q", got)
	}

	source.Relations[0].Correspondences[0].Member = "correspondent/source"
	if got := clone.Relations[0].Correspondences[0].Member; got != "correspondent/clone" {
		t.Fatalf("mutating source correspondence changed clone to %q", got)
	}
}
