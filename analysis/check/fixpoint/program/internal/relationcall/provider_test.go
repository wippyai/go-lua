package relationcall

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestOutcomeProviderSuppliesCanonicalDynamicReadSpecialization(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	self, name, node := symbol.ID(7101), symbol.ID(7102), symbol.ID(7103)
	readRef, keyRef := factflow.ExprRef(7201), factflow.ExprRef(7202)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	keySource, _ := factflow.NewExpressionValueSource(keyRef, int(assign), 0, 0, shape)
	readSource, _ := factflow.NewExpressionValueSource(readRef, int(assign), 0, 0, shape)
	returnSource, _ := factflow.NewPathValueSource(pathdom.NewPath(node, "node_id").Key(), int(ret), 0, 0, shape)
	dynamic, ok := factflow.NewDynamicIndexExpression(pathdom.NewPath(self, "self").Field("references"), keySource)
	if !ok {
		t.Fatal("dynamic expression rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, node, pathdom.NewPath(node, "node_id"), readSource),
		},
		Returns:                 map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{returnSource})},
		ExpressionPaths:         map[factflow.ExprRef]pathdom.Path{keyRef: pathdom.NewPath(name, "name")},
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{readRef: dynamic},
	}).WithBoundaryParams([]symbol.ID{self, name})
	relation := transformer.NewPlanCompiler().Compile(reg, graph, plan, transformer.Shape{Params: 2})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("dynamic relation contextual: %s", reason)
	}
	cell := transformer.CellRef{Function: 7100}
	snapshot, err := transformer.FreezeAcyclicRelation(context.Background(), cell, relation)
	if err != nil {
		t.Fatal(err)
	}

	ks := keyspace.New()
	rootID := identity.ID{Kind: "table", Site: "graph", Index: 1}
	referencesID := identity.ID{Kind: "table", Site: "references", Index: 2}
	rootValue, referencesValue := identityvalue.Present(reg, rootID), identityvalue.Present(reg, referencesID)
	want := typevalue.LiteralString(reg, "node-42")
	memberKey := func(seg segment.Segment) keyspace.Key {
		key, found := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{seg})
		if !found {
			t.Fatalf("member key %#v", seg)
		}
		return key
	}
	in := state.State{}.
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{memberKey(segment.Segment{Kind: segment.SegmentField, Name: "references"}): referencesValue},
		})).
		WriteHeapTableObject(reg, referencesID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: referencesValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{memberKey(segment.Segment{Kind: segment.SegmentIndexString, Name: "route"}): want},
		}))
	before := in.Snapshot()
	nameValue := typevalue.LiteralString(reg, "route")
	callerSelf := pathdom.NewPath(symbol.ID(8101), "graph")
	types := typevalue.NewCache()
	key := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(7100)))
	provider := OutcomeProvider(Config{
		Relations: snapshot,
		TargetFor: func(transfer.NodeContext, factflow.CallSiteView) (Target, bool) {
			return Target{Cell: cell, SummaryKey: key}, true
		},
		Bindings: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State, transformer.Shape) (transformer.BindingCursor, bool) {
			cursor, bindErr := transformer.NewBindingCursor(transformer.Shape{Params: 2},
				[]product.Value{rootValue, nameValue}, []pathdom.Path{callerSelf, pathdom.NewPath(symbol.ID(8102), "route_name")})
			return cursor, bindErr == nil
		},
		Specialization: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (transformer.SpecializationContext, bool) {
			return transformer.SpecializationContext{DynamicRead: func(path pathdom.Path, table, index product.Value) (product.Value, bool) {
				return sourcevalue.ReadBoundDynamicIndexValue(reg, types, ks, nil, 0, path, table, index, in)
			}}, true
		},
		Adapter: callresult.ProviderConfig{KeySpace: ks, TypeValues: types},
	})
	outcome := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), in, nil)
	if len(outcome.Results) != 1 || outcome.Results[0].Index != 0 || !product.Equal(reg, outcome.Results[0].Value, want) {
		t.Fatalf("relational outcome = %#v, want canonical dynamic member %#v", outcome.Results, want)
	}
	direct, exact := sourcevalue.ReadBoundDynamicIndexValue(reg, types, ks, nil, 0, callerSelf.Field("references"), rootValue, nameValue, in)
	if !exact || !product.Equal(reg, direct, outcome.Results[0].Value) {
		t.Fatalf("production/canonical dynamic read differs: %#v vs %#v/%v", outcome.Results[0].Value, direct, exact)
	}
	if !state.Domain(reg).Equal(in, before) {
		t.Fatal("production relation specialization mutated one of the 17 caller State lanes")
	}
}
