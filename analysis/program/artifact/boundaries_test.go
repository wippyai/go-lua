package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestBoundaryRowsRequireOwnerAndCapturePosition(t *testing.T) {
	owner := identity.ContentID{1}
	id := identity.ContentID{2}
	if row := (BoundaryRow{kind: BoundaryReturn, id: id, owner: owner, position: 1}); row.Available() {
		t.Fatal("return boundary admitted a capture position")
	}
	row := BoundaryRow{kind: BoundaryCapture, id: id, owner: owner, position: 2, eligible: true}
	if !row.Available() || row.Kind() != BoundaryCapture {
		t.Fatal("valid capture boundary unavailable")
	}
	if position, ok := row.Position(); !ok || position != 2 || !row.Eligible() {
		t.Fatal("capture boundary lost position or eligibility")
	}
}
