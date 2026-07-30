package engine

import "testing"

// TestStaticIndexWriteContainerNamesTheAddressedSequence pins the container a
// static store's revocation is published against. It is the written path minus
// its final segment, and only where that segment is an integer index: a named
// slot holds no sequence position, and a bare root addresses none either. The
// container is spelled exactly as its own reads spell it, so the revocation and
// the proof it revokes resolve the same subject.
func TestStaticIndexWriteContainerNamesTheAddressedSequence(t *testing.T) {
	for _, testCase := range []struct {
		target    string
		container string
		indexed   bool
	}{
		{target: "path/xs[1]", container: "path/xs", indexed: true},
		{target: "path/box.items[3]", container: "path/box.items", indexed: true},
		{target: "path/grid[2][7]", container: "path/grid[2]", indexed: true},
		{target: "path/box.tag", indexed: false},
		{target: "path/box.items", indexed: false},
		{target: "path/xs", indexed: false},
		{target: "temp/t0", indexed: false},
	} {
		container, indexed := staticIndexWriteContainer([]byte(testCase.target))
		if indexed != testCase.indexed {
			t.Fatalf("%s: indexed=%v, want %v", testCase.target, indexed, testCase.indexed)
		}
		if indexed && string(container) != testCase.container {
			t.Fatalf("%s: container=%s, want %s", testCase.target, container, testCase.container)
		}
	}
}
