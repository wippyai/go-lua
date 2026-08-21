package functionboundary_test

import "testing"

func TestBoundaryContextHashesRemainCanonicalAndDistinct(t *testing.T) {
	left := lowerFunctionBoundary(t, "local function f(a) return a end return f(1)")
	right := lowerFunctionBoundary(t, "local function f(a) return a end return f(1)")
	leftView, rightView := left.Flow(), right.Flow()
	leftBoundaries, rightBoundaries := leftView.FunctionBoundaries(), rightView.FunctionBoundaries()
	if leftBoundaries.Count() == 0 || leftBoundaries.Count() != rightBoundaries.Count() {
		t.Fatalf("boundary counts = %d/%d", leftBoundaries.Count(), rightBoundaries.Count())
	}
	for index := 0; index < leftBoundaries.Count(); index++ {
		leftBoundary, leftOK := leftBoundaries.At(index)
		rightBoundary, rightOK := rightBoundaries.At(index)
		if !leftOK || !rightOK || !leftBoundary.ContextID().Available() || leftBoundary.ContextID() != rightBoundary.ContextID() || !leftBoundary.Equal(rightBoundary) {
			t.Fatalf("boundary[%d] context = %v/%v equal=%v", index, leftBoundary.ContextID(), rightBoundary.ContextID(), leftBoundary.Equal(rightBoundary))
		}
	}
	root, rootOK := leftBoundaries.Root()
	if !rootOK || !root.ContextID().Available() {
		t.Fatal("root Body context was not published")
	}
	rootBody, rootBodyOK := root.Body()
	if !rootBodyOK {
		t.Fatal("root Body was not published")
	}
	resolved, resolvedOK := leftBoundaries.ResolveBodyContextID(root.ContextID())
	canonical, canonicalOK := leftBoundaries.ForBody(rootBody)
	if !resolvedOK || !canonicalOK || !resolved.Equal(canonical) {
		t.Fatal("Body context inverse did not resolve the canonical root")
	}
}
