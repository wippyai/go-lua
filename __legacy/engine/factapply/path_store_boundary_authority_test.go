package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPathSemanticAuthorityAppliesCompleteCallbackFreeN4Transaction(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(83)
	container := symbol.ID(883)
	marker := symbol.ID(884)
	target := pathdom.NewPath(container, "container").Field("object")
	leaf := target.Field("leaf")
	staticTarget := target.Field("static")
	assignmentValue := presentValue(reg)
	leafValue := absentValue(reg)
	staticValue := product.Top()
	transaction := ResolvedPathStoreTransaction{
		Point: point,
		Assignment: ResolvedPathStoreWrite{
			Target: target, Value: assignmentValue,
		},
		HasAssignment: true,
		Object: ResolvedPathStoreObject{Entries: []ResolvedPathStoreWrite{{
			Target: leaf, Value: leafValue,
		}}},
		Static: ResolvedPathStoreWrite{
			Target: staticTarget, Value: staticValue,
		},
		HasStatic: true,
	}
	if !transaction.Valid(reg) || !transaction.HasStateSteps() || !transaction.HasPathAssignment() ||
		!transaction.HasObjectLiteral() || !transaction.HasStaticMemberWrite() {
		t.Fatalf("complete N4 transaction is invalid: %#v", transaction)
	}
	builder := visibility.NewBuilder()
	builder.Define(point, container, "container")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	input := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(container), product.Top())
	output := input.WriteValue(reg, statekey.SymbolValue(marker), leafValue)

	want := ApplyResolvedPathStore(ResolvedPathStoreRequest{
		Context:  transfer.NodeContext{Context: context.Background(), Registry: reg, Point: point},
		Resolver: resolver, Input: input, Output: output, Transaction: transaction.Clone(),
	})
	if want.Canceled || !want.AssignmentApplied {
		t.Fatal("established N4 primitive did not apply the test transaction")
	}
	got, err := authority.ApplyResolvedPathStoreOnto(context.Background(), reg, transaction, input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Domain(reg).Equal(got, want.Output) {
		t.Fatal("callback-free path authority differs from the sole concrete N4 executor")
	}
	if !product.Equal(reg, got.ReadValue(reg, statekey.SymbolValue(marker)), leafValue) {
		t.Fatal("N4 transaction did not compose onto evolving N0..N3 Output")
	}
	leafKey := factPathKeyAt(resolver, point, leaf)
	if value := got.ReadPathKey(reg, resolver.KeySpace(), leafKey); !product.Equal(reg, value, leafValue) {
		t.Fatal("object-literal entry was not materialized after its assignment")
	}
	staticKey := factPathKeyAt(resolver, point, staticTarget)
	if value, ok := got.ReadPathStaticMember(resolver.KeySpace(), staticKey); !ok || !product.Equal(reg, value, staticValue) {
		t.Fatal("independent static-member write was not applied after object materialization")
	}
}

func TestResolvedPathStoreTransactionCloneDetachesObjectSyntax(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(89)
	target := pathdom.NewPath(symbol.ID(889), "container").Field("object")
	transaction := ResolvedPathStoreTransaction{
		Point: point,
		Assignment: ResolvedPathStoreWrite{
			Target: target, Value: presentValue(reg),
		},
		HasAssignment: true,
		Object: ResolvedPathStoreObject{
			Entries: []ResolvedPathStoreWrite{{Target: target.Field("leaf"), Value: absentValue(reg)}},
			Heaps: []ResolvedPathStoreHeapObject{{
				Root: presentValue(reg),
				Members: []ResolvedPathStoreHeapMember{{
					Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}, Value: product.Top(),
				}},
			}},
		},
	}
	wantTarget := target.Clone()
	wantLeaf := target.Field("leaf")
	frozen := transaction.Clone()
	transaction.Assignment.Target.Segments[0].Name = "mutated-assignment"
	transaction.Object.Entries[0].Target.Segments[1].Name = "mutated-entry"
	transaction.Object.Heaps[0].Members[0].Suffix[0].Name = "mutated-member"
	if !frozen.Assignment.Target.Equal(wantTarget) || !frozen.Object.Entries[0].Target.Equal(wantLeaf) ||
		frozen.Object.Heaps[0].Members[0].Suffix[0].Name != "member" {
		t.Fatal("sealed N4 transaction retained mutable compiler-owned path storage")
	}
}

func TestPathSemanticAuthorityN4CancellationRollsBackPointInput(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(97)
	container := symbol.ID(897)
	target := pathdom.NewPath(container, "container").Field("value")
	transaction := ResolvedPathStoreTransaction{
		Point: point,
		Assignment: ResolvedPathStoreWrite{
			Target: target, Value: presentValue(reg),
		},
		HasAssignment: true,
	}
	builder := visibility.NewBuilder()
	builder.Define(point, container, "container")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)
	input := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(container), product.Top())
	output := input.WriteValue(reg, statekey.SymbolValue(symbol.ID(898)), absentValue(reg))
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	rolledBack, err := authority.ApplyResolvedPathStoreOnto(ctx, reg, transaction, input, output)
	if err == nil {
		t.Fatal("pre-canceled N4 authority did not report cancellation")
	}
	if !state.Domain(reg).Equal(rolledBack, input) {
		t.Fatal("canceled N4 authority published a prefix instead of immutable point Input")
	}
}
