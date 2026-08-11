package call

import "testing"

// TestCallAlgebraRetainsItsExactLinkAuthority proves that the Call algebra
// exposes the issuing Link only as an owner fence.  Two separately sealed
// links with the same source content are not interchangeable: their opaque
// Call source-sum keys remain local to the issuing Link.
func TestCallAlgebraRetainsItsExactLinkAuthority(t *testing.T) {
	left, _ := callSource(t, "call_link_fence", `local function invoke(f) f() end`, nil)
	contract, ok := left.Boundary().Target()
	if !ok || contract == nil {
		t.Fatal("left target")
	}
	right, _ := callSource(t, "call_link_fence", `local function invoke(f) f() end`, contract)
	if left == right || left.ContentID() != right.ContentID() {
		t.Fatal("independent equal-content links")
	}
	leftAlgebra, leftOK := New(left)
	rightAlgebra, rightOK := New(right)
	if !leftOK || !rightOK || leftAlgebra.Link() != left || rightAlgebra.Link() != right {
		t.Fatal("algebra link provenance")
	}
	leftApplication, applicationOK := left.Project().Applications().Calls().At(0)
	if !applicationOK {
		t.Fatal("left call application")
	}
	if _, accepted := rightAlgebra.KeyForApplication(leftApplication); accepted {
		t.Fatal("foreign application crossed Call Link fence")
	}
}
