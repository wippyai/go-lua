package database_test

import (
	"testing"

	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

// A database root must fence both contribution owners with the mounted
// runtime authority. The owners are part of the root, not a side store.
func TestDatabaseBootstrapOwnsFencedContributionRoots(t *testing.T) {
	fixture := testfixture.New(t)
	root := fixture.Base()
	directory := root.ContributionDirectory()
	state := root.ContributionState()
	if !directory.Available() || !state.Available() {
		t.Fatal("bootstrap did not create contribution owners")
	}
	if !directory.Fence().Same(root.Fence()) || !state.Fence().Same(root.Fence()) {
		t.Fatal("contribution owners escaped the database runtime fence")
	}
	if directory.Len() != 0 || state.Len() != 0 {
		t.Fatal("bootstrap contribution owners were not empty")
	}
}
