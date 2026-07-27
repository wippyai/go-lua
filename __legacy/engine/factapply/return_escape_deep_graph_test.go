package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// TestReturnEscapeAppliesOwnedHeapToDeeplyNestedFreshGraph pins
// invariants.md #2: returning a fresh allocation applies the return escape
// transition to the complete reachable fresh graph, so the result is owned
// heap and never stack. The literal graph here is three levels deep
// (root -> child -> grandchild, plus a scalar leaf) so the recursive N5
// source inventory and the escape-transition graph walk are both exercised
// past a single hop, matching the "complete reachable fresh graph" wording
// rather than only its direct member.
func TestReturnEscapeAppliesOwnedHeapToDeeplyNestedFreshGraph(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8201)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8201, HasExpr: true}
	childSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8202, HasExpr: true}
	grandchildSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8203, HasExpr: true}
	scalarSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8204, HasExpr: true}

	rootID := testTableLiteralID(rootSource.ExprRef)
	childID := testTableLiteralID(childSource.ExprRef)
	grandchildID := testTableLiteralID(grandchildSource.ExprRef)

	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(childID))
	grandchildValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(grandchildID))
	scalarValue := presentValue(reg)

	facts := factflow.NewFacts(factflow.FactsInput{
		Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{rootSource})},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("child"), childSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(rootID),
			childSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("grandchild"), grandchildSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(childID),
			grandchildSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("value"), scalarSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(grandchildID),
		},
	})

	plan, ok := PlanReturnTransaction(facts, point)
	if !ok || plan.SourceCount() != 4 {
		t.Fatalf("recursive source inventory = %d/%v, want 4/true", plan.SourceCount(), ok)
	}

	values := map[factflow.ValueSource]product.Value{
		rootSource: rootValue, childSource: childValue, grandchildSource: grandchildValue, scalarSource: scalarValue,
	}
	resolved := make([]product.Value, plan.SourceCount())
	for index := range resolved {
		source, ok := plan.Source(index)
		if !ok {
			t.Fatalf("source %d missing from frozen plan", index)
		}
		value, present := values[source]
		if !present {
			t.Fatalf("no fixture value for source %#v", source)
		}
		resolved[index] = value
	}
	transaction, ok := plan.Bind(reg, resolved)
	if !ok {
		t.Fatal("return bind failed")
	}

	resolver := visibility.NewResolver(visibility.NewTable(nil))
	authority := NewReturnAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts)
	input := state.Reachable(state.State{})
	got, err := authority.Apply(context.Background(), reg, transaction, input, input)
	if err != nil {
		t.Fatalf("resolved return apply: %v", err)
	}

	assertHeapStaticMember(t, reg, resolver.KeySpace(), got, rootSource.ExprRef, ".child", childValue)
	assertHeapStaticMember(t, reg, resolver.KeySpace(), got, childSource.ExprRef, ".grandchild", grandchildValue)
	assertHeapStaticMember(t, reg, resolver.KeySpace(), got, grandchildSource.ExprRef, ".value", scalarValue)

	for _, id := range []identity.ID{rootID, childID, grandchildID} {
		if gotPlacement := got.ReadPlacement(id); gotPlacement == placement.Stack {
			t.Fatalf("returned fresh graph placement[%v] = stack, want heap", id)
		}
		assertPlacement(t, got, id, placement.OwnedHeap)
	}

	if product.Equal(reg, got.ReadValue(reg, key.ReturnSlot(0)), product.Bottom(reg)) {
		t.Fatal("return slot 0 was not published")
	}
}
