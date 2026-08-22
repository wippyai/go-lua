package transform

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestCloneForcesFreshCompositeAndPreservesPrimitive(t *testing.T) {
	original := typ.NewArray(typ.String)
	cloned, ok := Clone(original).(*typ.Array)
	if !ok || cloned == original {
		t.Fatalf("Clone did not allocate a fresh array: %#v", cloned)
	}
	if cloned.Element != typ.String {
		t.Fatalf("Clone copied immutable primitive: %p / %p", cloned.Element, typ.String)
	}
}

func TestClonePreservesDAGSharing(t *testing.T) {
	shared := typetable.NewRecord().Field("value", typ.String).Build()
	original := typ.NewTuple(shared, shared)
	cloned := Clone(original).(*typ.Tuple)
	left := cloned.Elements[0].(*typ.Record)
	right := cloned.Elements[1].(*typ.Record)
	if left == shared || right == shared {
		t.Fatal("Clone retained source DAG node")
	}
	if left != right {
		t.Fatal("Clone expanded one shared source node into two copies")
	}
}

func TestClonePreservesDistinctBinderOccurrences(t *testing.T) {
	leftParam := typ.NewTypeParam("T", nil)
	rightParam := typ.NewTypeParam("T", nil)
	left := typ.Func().TypeParamRef(leftParam).Param("value", leftParam).Returns(leftParam).Build()
	right := typ.Func().TypeParamRef(rightParam).Param("value", rightParam).Returns(rightParam).Build()
	original := typ.NewTuple(left, right)
	cloned := Clone(original).(*typ.Tuple)
	leftClone := cloned.Elements[0].(*typ.Function)
	rightClone := cloned.Elements[1].(*typ.Function)
	if leftClone == left || rightClone == right || leftClone == rightClone {
		t.Fatal("Clone collapsed distinct binder occurrences")
	}
	if leftClone.TypeParams[0] == leftParam || rightClone.TypeParams[0] == rightParam {
		t.Fatal("Clone retained source binder")
	}
	if leftClone.Params[0].Type != leftClone.TypeParams[0] || leftClone.Returns[0] != leftClone.TypeParams[0] {
		t.Fatal("left binder references were not rebound")
	}
	if rightClone.Params[0].Type != rightClone.TypeParams[0] || rightClone.Returns[0] != rightClone.TypeParams[0] {
		t.Fatal("right binder references were not rebound")
	}
}

func TestClonePreservesRecursiveBackedge(t *testing.T) {
	original := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewArray(self)
	})
	cloned := Clone(original).(*typ.Recursive)
	if cloned == original {
		t.Fatal("Clone retained recursive source")
	}
	body, ok := cloned.Body.(*typ.Array)
	if !ok || body.Element != cloned {
		t.Fatalf("Clone lost recursive backedge: %#v", cloned.Body)
	}
}

func TestClonePreservesMutualRecursiveBackedges(t *testing.T) {
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(typetable.NewRecord().Field("right", right).Build())
	right.SetBody(typetable.NewRecord().Field("left", left).Build())

	cloned := Clone(left).(*typ.Recursive)
	leftBody := cloned.Body.(*typ.Record)
	rightClone := leftBody.Fields[0].Type.(*typ.Recursive)
	rightBody := rightClone.Body.(*typ.Record)
	if rightBody.Fields[0].Type != cloned {
		t.Fatal("Clone lost mutual recursive backedge")
	}
	if rightClone == right || cloned == left {
		t.Fatal("Clone retained mutual recursive source")
	}
}

func TestClonePreservesGenericBinderAndSelfInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Node", []*typ.TypeParam{param}, nil)
	application := typ.Instantiate(generic, typ.String)
	generic.SetBody(typ.NewTuple(application, param))

	cloned := Clone(generic).(*typ.Generic)
	if cloned == generic || cloned.TypeParams[0] == param {
		t.Fatal("Clone retained generic source or binder")
	}
	body := cloned.Body.(*typ.Tuple)
	clonedApplication := body.Elements[0].(*typ.Instantiated)
	if clonedApplication.Generic != cloned {
		t.Fatal("Clone lost generic self-instantiation backedge")
	}
	if body.Elements[1] != cloned.TypeParams[0] {
		t.Fatal("Clone did not rebind generic body formal")
	}
}

func TestCloneMutationIsolation(t *testing.T) {
	original := typetable.NewRecord().Field("value", typ.NewArray(typ.String)).Build()
	cloned := Clone(original).(*typ.Record)
	clonedArray := cloned.Fields[0].Type.(*typ.Array)
	clonedArray.Element = typ.Number
	if original.Fields[0].Type.(*typ.Array).Element != typ.String {
		t.Fatal("mutating Clone changed source graph")
	}
}

func TestClonePreservesAnnotatedWrapperAndOwnsAnnotations(t *testing.T) {
	original := typ.NewAnnotated(typ.NewArray(typ.String), []annotation.Annotation{{Name: "min", Arg: annotation.Int64Arg(1)}})
	cloned, ok := Clone(original).(*typ.Annotated)
	if !ok || cloned == original {
		t.Fatalf("Clone dropped or retained annotation wrapper: %#v", cloned)
	}
	if cloned.Inner == original.(*typ.Annotated).Inner {
		t.Fatal("Clone retained annotated inner composite")
	}
	if len(cloned.Annotations) != 1 || !cloned.Annotations[0].Equal(original.(*typ.Annotated).Annotations[0]) {
		t.Fatal("Clone changed annotation payload")
	}
	cloned.Annotations[0].Name = "max"
	if original.(*typ.Annotated).Annotations[0].Name != "min" {
		t.Fatal("mutating Clone annotation changed source")
	}
}

func TestClonePreservesAliasWrapperAndClonesTarget(t *testing.T) {
	original := typ.NewAlias("Value", typ.NewArray(typ.String))
	cloned, ok := Clone(original).(*typ.Alias)
	if !ok || cloned == original {
		t.Fatalf("Clone dropped or retained alias wrapper: %#v", cloned)
	}
	if cloned.Target == original.Target {
		t.Fatal("Clone retained alias target composite")
	}
}

func TestCloneOwnsMutableRefFields(t *testing.T) {
	original := typ.NewRef("module", "Value")
	cloned, ok := Clone(original).(*typ.Ref)
	if !ok || cloned == original {
		t.Fatalf("Clone retained Ref source: %#v", cloned)
	}
	cloned.Module = "other"
	cloned.Name = "Changed"
	if original.Module != "module" || original.Name != "Value" {
		t.Fatal("mutating Clone Ref changed source")
	}
}
