package typ

import (
	"context"
	"testing"
)

func TestCanonicalGraphReceiptAlphaStableRoot(t *testing.T) {
	leftFormal := NewTypeParam("T", nil)
	rightFormal := NewTypeParam("Value", nil)
	left := Func().TypeParamRef(leftFormal).Param("x", leftFormal).Returns(leftFormal).Build()
	right := Func().TypeParamRef(rightFormal).Param("x", rightFormal).Returns(rightFormal).Build()

	leftReceipt, leftErr := EncodeCanonicalGraph(context.Background(), left)
	if leftErr != nil || !leftReceipt.Valid() {
		t.Fatalf("left graph receipt: %v", leftErr)
	}
	rightReceipt, rightErr := EncodeCanonicalGraph(context.Background(), right)
	if rightErr != nil || !rightReceipt.Valid() {
		t.Fatalf("right graph receipt: %v", rightErr)
	}
	leftDigest, leftOK := leftReceipt.Digest()
	rightDigest, rightOK := rightReceipt.Digest()
	if !leftOK || !rightOK || leftDigest != rightDigest {
		t.Fatalf("alpha-equivalent roots have different identities: %x / %x", leftDigest, rightDigest)
	}
}

func TestCanonicalGraphReceiptOpenAlphaStableAndConstraintSensitive(t *testing.T) {
	leftFormal := NewTypeParam("T", nil)
	rightFormal := NewTypeParam("Value", nil)
	left := NewArray(leftFormal)
	right := NewArray(rightFormal)

	leftGraph, leftErr := EncodeCanonicalGraph(context.Background(), left)
	if leftErr != nil || !leftGraph.Valid() {
		t.Fatalf("left open graph receipt: %v", leftErr)
	}
	rightGraph, rightErr := EncodeCanonicalGraph(context.Background(), right)
	if rightErr != nil || !rightGraph.Valid() {
		t.Fatalf("right open graph receipt: %v", rightErr)
	}
	leftDigest, leftOK := leftGraph.Digest()
	rightDigest, rightOK := rightGraph.Digest()
	if !leftOK || !rightOK || leftDigest != rightDigest {
		t.Fatalf("alpha-equivalent open roots have different identities: %x / %x", leftDigest, rightDigest)
	}
	differentGraph, err := EncodeCanonicalGraph(context.Background(), NewArray(NewTypeParam("T", Number)))
	if err != nil {
		t.Fatal(err)
	}
	differentDigest, differentOK := differentGraph.Digest()
	if !differentOK || differentDigest == leftDigest {
		t.Fatal("open canonical identity erased formal constraint structure")
	}
	root, ok := leftGraph.Root()
	if !ok || root.Closed {
		t.Fatalf("open root was admitted as closed: %+v", root)
	}
	if root.Scope.Formals != 1 || root.Scope.Token == (CanonicalDigest{}) {
		t.Fatalf("open root lost formal scope metadata: %+v", root.Scope)
	}
}

func TestCanonicalGraphReceiptRejectsMutualGenericRecurrence(t *testing.T) {
	firstParam := NewTypeParam("T", nil)
	secondParam := NewTypeParam("U", nil)
	first := NewGeneric("First", []*TypeParam{firstParam}, nil)
	second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
	first.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(second, firstParam)}}}))
	second.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(first, secondParam)}}}))
	if receipt, err := EncodeCanonicalGraph(context.Background(), first); err == nil || receipt.Valid() {
		t.Fatal("mutually recursive generic group received graph identity")
	}
}

func TestCanonicalGraphReceiptPreservesDistinctBinderOccurrences(t *testing.T) {
	leftFormal := NewTypeParam("T", nil)
	rightFormal := NewTypeParam("T", nil)
	left := Func().TypeParamRef(leftFormal).Param("x", leftFormal).Returns(leftFormal).Build()
	right := Func().TypeParamRef(rightFormal).Param("x", rightFormal).Returns(rightFormal).Build()
	receipt, err := EncodeCanonicalGraph(context.Background(), NewTuple(left, right))
	if err != nil {
		t.Fatal(err)
	}
	nodes := receipt.Nodes()
	var functions, formals []CanonicalGraphNode
	for _, node := range nodes {
		if node.Kind == left.Kind() {
			functions = append(functions, node)
		}
		if node.Bound {
			formals = append(formals, node)
		}
	}
	if len(functions) != 2 || len(formals) != 2 {
		t.Fatalf("source occurrence plane collapsed binders: functions=%d formals=%d nodes=%d", len(functions), len(formals), len(nodes))
	}
	if formals[0].Binding.Owner == formals[1].Binding.Owner {
		t.Fatalf("distinct formal occurrences share owner ordinal: %+v / %+v", formals[0].Binding, formals[1].Binding)
	}
	if formals[0].Scope.Token == formals[1].Scope.Token {
		t.Fatalf("distinct binder scopes share occurrence token")
	}
}

func TestCanonicalGraphReceiptReusesOneBinderPointer(t *testing.T) {
	formal := NewTypeParam("T", nil)
	binder := Func().TypeParamRef(formal).Param("x", formal).Returns(formal).Build()
	receipt, err := EncodeCanonicalGraph(context.Background(), NewTuple(binder, binder))
	if err != nil {
		t.Fatal(err)
	}
	nodes := receipt.Nodes()
	functions := 0
	for _, node := range nodes {
		if node.Kind == binder.Kind() {
			functions++
		}
	}
	if functions != 1 {
		t.Fatalf("reused binder pointer was duplicated: %d", functions)
	}
	root, ok := receipt.Root()
	if !ok || len(root.Children) != 2 || root.Children[0] != root.Children[1] {
		t.Fatalf("reused binder edge was not reused: %+v", root)
	}
}

func TestCanonicalGraphReceiptOuterCaptureIsOpen(t *testing.T) {
	outer := NewTypeParam("Outer", nil)
	inner := NewTypeParam("Inner", nil)
	nested := Func().TypeParamRef(inner).Param("captured", outer).Returns(inner).Build()
	root := Func().TypeParamRef(outer).Returns(nested).Build()
	receipt, err := EncodeCanonicalGraph(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	nodes := receipt.Nodes()
	var rootNode, nestedNode *CanonicalGraphNode
	for index := range nodes {
		node := nodes[index]
		if node.Kind != root.Kind() {
			continue
		}
		if rootNode == nil {
			rootNode = &node
		} else {
			nestedNode = &node
		}
	}
	if rootNode == nil || nestedNode == nil {
		t.Fatalf("missing root/nested function nodes")
	}
	if !rootNode.Closed {
		t.Fatalf("root binder with own formal is not closed")
	}
	if nestedNode.Closed {
		t.Fatalf("nested binder capturing outer formal was incorrectly closed")
	}
	if nestedNode.Scope.Token == (CanonicalDigest{}) {
		t.Fatalf("nested capture has no lexical scope token")
	}
}

func TestCanonicalGraphReceiptEdgesAreDefensive(t *testing.T) {
	receipt, err := EncodeCanonicalGraph(context.Background(), NewArray(String))
	if err != nil {
		t.Fatal(err)
	}
	nodes := receipt.Nodes()
	if len(nodes) == 0 || len(nodes[0].Children) == 0 {
		t.Fatal("array graph has no child edge")
	}
	nodes[0].Children[0] = ^uint32(0)
	node, ok := receipt.Node(0)
	if !ok || node.Children[0] == ^uint32(0) {
		t.Fatal("receipt topology aliases defensive view")
	}
}

func TestCanonicalGraphReceiptTransfersMutableSourcePlaneOnce(t *testing.T) {
	function := Func().Returns(String).Build()
	receipt, err := EncodeCanonicalGraph(context.Background(), function)
	if err != nil {
		t.Fatal(err)
	}
	copyOfReceipt := receipt
	digest, digestOK := receipt.Digest()
	root, rootOK := receipt.Root()
	plane, taken := copyOfReceipt.TakeSourcePlane()
	if !digestOK || !rootOK || !taken || len(plane) != len(receipt.Nodes()) {
		t.Fatal("canonical source plane was not transferred")
	}
	var detached *Function
	for _, source := range plane {
		if candidate, ok := source.(*Function); ok {
			detached = candidate
			break
		}
	}
	if detached == nil {
		t.Fatal("detached source plane has no function root")
	}
	detached.Returns[0] = Number
	if second, ok := receipt.TakeSourcePlane(); ok || second != nil {
		t.Fatal("a receipt copy transferred the mutable source plane twice")
	}
	gotDigest, gotDigestOK := receipt.Digest()
	gotRoot, gotRootOK := receipt.Root()
	if !gotDigestOK || gotDigest != digest || !gotRootOK || gotRoot.Identity != root.Identity {
		t.Fatal("mutating the detached source changed the immutable receipt")
	}
}

func TestCanonicalGraphReceiptRecursiveGeneric(t *testing.T) {
	formal := NewTypeParam("T", nil)
	generic := NewGeneric("Node", []*TypeParam{formal}, nil)
	generic.SetBody(NewArray(generic))
	if receipt, err := EncodeCanonicalGraph(context.Background(), generic); err != nil || !receipt.Valid() {
		t.Fatalf("recursive generic graph: %v", err)
	}
}
