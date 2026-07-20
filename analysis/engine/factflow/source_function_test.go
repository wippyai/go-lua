package factflow

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestSourceContainsFunctionTraversesCanonicalExpressionDAG(t *testing.T) {
	const (
		functionRef ExprRef = iota + 1
		refinementRef
		operationRef
		objectRef
		dynamicRef
		cycleRef
	)
	function := symbol.ID(7101)
	source := func(ref ExprRef) ValueSource {
		return ValueSource{Kind: ValueSourceExpression, ExprRef: ref, HasExpr: true}
	}
	operation, ok := NewUnaryExpressionOperation("not", source(refinementRef))
	if !ok {
		t.Fatal("unary expression operation rejected")
	}
	literal := NewObjectLiteral([]ObjectEntry{NewObjectEntryWithMetadata(
		pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "callback"}}},
		source(operationRef), SourceSpan{}, "",
	)})
	shape, _ := NewValueSourceShape(true, false, false, false)
	key, ok := NewStringLiteralValueSource("callback", 0, NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("literal key source rejected")
	}
	dynamic, ok := NewDynamicIndexExpressionFromSource(source(objectRef), key)
	if !ok {
		t.Fatal("dynamic expression rejected")
	}
	cycle, ok := NewUnaryExpressionOperation("not", source(cycleRef))
	if !ok {
		t.Fatal("cyclic operation descriptor rejected")
	}
	facts := NewFacts(FactsInput{
		ExpressionFunctions: map[ExprRef]symbol.ID{functionRef: function},
		ExpressionRefinements: map[ExprRef]ExpressionRefinement{
			refinementRef: NewExpressionRefinement(source(functionRef), product.Top()),
		},
		ExpressionOperations:    map[ExprRef]ExpressionOperation{operationRef: operation, cycleRef: cycle},
		ObjectLiterals:          map[ExprRef]ObjectLiteral{objectRef: literal},
		DynamicIndexExpressions: map[ExprRef]DynamicIndexExpression{dynamicRef: dynamic},
	})
	if !facts.SourceContainsFunction(source(dynamicRef), function) {
		t.Fatal("canonical refinement/operation/object/dynamic source DAG lost its lexical function")
	}
	if facts.SourceContainsFunction(source(dynamicRef), function+1) {
		t.Fatal("source DAG invented a foreign lexical function")
	}
	if facts.SourceContainsFunction(source(cycleRef), function) {
		t.Fatal("cyclic source DAG invented a lexical function")
	}
}
