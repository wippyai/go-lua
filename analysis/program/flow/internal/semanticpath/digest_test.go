package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDigestPathsAreDeterministicAndRoleQualified(t *testing.T) {
	parent := identity.ContentID{1}
	other := identity.ContentID{2}
	span := source.Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 7}
	if got, want := digestPath("role", parent, 4, 5, span), digestPath("role", parent, 4, 5, span); got != want {
		t.Fatal("digestPath changed across identical inputs")
	}
	if digestPath("role", parent, 4, 5, span) == digestPath("role", parent, 4, 6, span) {
		t.Fatal("digestPath ignored relation rank")
	}
	if digestPath3("role", parent, 4, 5, 6, span) == digestPath3("role", parent, 4, 5, 7, span) {
		t.Fatal("digestPath3 ignored its extra discriminator")
	}
	if digestBytes("role", parent, other) == digestBytes("role", parent, parent) {
		t.Fatal("digestBytes collapsed distinct child identities")
	}
	if digestOutcome(parent, 1, other) == digestOutcome(parent, 2, other) {
		t.Fatal("digestOutcome ignored outcome kind")
	}
}
