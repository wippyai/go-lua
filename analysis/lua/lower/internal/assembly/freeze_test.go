package assembly

import "testing"

func TestAssemblyPublishRejectsIncompleteConstruction(t *testing.T) {
	c := newAssemblyCollector()
	if _, err := c.Publish(); err == nil {
		t.Fatal("Publish accepted an incomplete source construction")
	}
	if _, err := c.Publish(); err == nil {
		t.Fatal("terminal collector accepted a second Publish")
	}
}
