package projectsummary

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestResolveReferenceRelationMatchesCanonicalProductionPaths(t *testing.T) {
	reg := standard.Registry()
	prepared := prepareResolveReferenceRelationFixture(t)
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 2 {
		t.Fatalf("resolve_reference boundary params = %v, want self/name", params)
	}
	shape := transformer.Shape{Params: 2}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("resolve_reference relation compiled contextually: %s", reason)
	}

	ks := prepared.KeySpace()
	selfID := identity.ID{Kind: "table", Site: "resolve-reference-self", Index: 1}
	referencesID := identity.ID{Kind: "table", Site: "resolve-reference-index", Index: 2}
	selfValue := identityvalue.Present(reg, selfID)
	referencesValue := identityvalue.Present(reg, referencesID)
	referencesKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "references"}})
	nodeKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "present"}})
	falsyKey, _ := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "falsy"}})
	heapState := state.State{}.
		WriteHeapTableObject(reg, selfID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: selfValue, StaticMembers: map[keyspace.Key]product.Value{referencesKey: referencesValue}, StableShape: true,
		})).
		WriteHeapTableObject(reg, referencesID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: referencesValue, StaticMembers: map[keyspace.Key]product.Value{
				nodeKey: typevalue.LiteralString(reg, "node-17"), falsyKey: typevalue.LiteralBool(reg, false),
			}, StableShape: true,
		}))
	tests := []struct {
		name string
		base state.State
		self product.Value
		key  product.Value
	}{
		{name: "nil name", base: state.State{}, self: product.Top(), key: typevalue.Nil(reg)},
		// A present false member exercises the second normalized-falsy edge. A
		// truly absent key is intentionally not aliased to this case: the concrete
		// read/projector currently emits Bottom for return slot 0 where the
		// canonical symbolic read emits explicit nil. That engine discrepancy has
		// its own red-first follow-up.
		{name: "falsy reference", base: heapState, self: selfValue, key: typevalue.LiteralString(reg, "falsy")},
		{name: "present reference", base: heapState, self: selfValue, key: typevalue.LiteralString(reg, "present")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := []product.Value{tc.self, tc.key}
			cursor, err := transformer.NewBindingCursor(shape, values, []pathdom.Path{
				pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1),
			})
			if err != nil {
				t.Fatal(err)
			}
			stats := &body.Stats{}
			context := transformer.SpecializationContext{}
			if tc.name != "nil name" {
				context.DynamicRead = func(tablePath pathdom.Path, owner, key product.Value) (product.Value, bool) {
					return sourcevalue.ReadBoundDynamicIndexValue(reg, typevalue.NewCache(), ks, nil, 0, tablePath, owner, key, tc.base)
				}
			}
			got, exact := relation.SpecializeWithContext(cursor, nil, context)
			if !exact || stats.BodySolves != 0 {
				t.Fatalf("relation specialization exact/solves = %v/%d, want true/0", exact, stats.BodySolves)
			}
			entry := tc.base
			for i, param := range params {
				entry = entry.WriteValue(reg, statekey.SymbolValue(param), values[i])
			}
			concrete, err := body.SolvePrepared(prepared, body.SolveConfig{EntryState: entry, Stats: stats})
			if err != nil {
				t.Fatal(err)
			}
			want := summary.Normalize(reg, FromResult(concrete))
			gotComparable, wantComparable := withoutUnchangedEntryHeap(got), withoutUnchangedEntryHeap(want)
			if !summary.Equal(reg, gotComparable, wantComparable) || summary.NormalizedPayloadDigest(reg, gotComparable) != summary.NormalizedPayloadDigest(reg, wantComparable) {
				t.Fatalf("symbolic/canonical Summary differs\n got=%#v\nwant=%#v", got, want)
			}
			if tc.name == "present reference" {
				found := false
				for _, proof := range got.NormalReturnFacts.BranchProofs {
					if proof.Path.Equal(pathdom.NewPlaceholder(0).Field("references").IndexStr("present")) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("specialized dynamic-path proof missing: %#v", got.NormalReturnFacts.BranchProofs)
				}
			}
			if stats.BodySolves != 1 {
				t.Fatalf("canonical oracle solves = %d, want one", stats.BodySolves)
			}
			assertResolveReferenceProductionComposition(t, prepared, tc.base, values, got, want)
		})
	}
}

func withoutUnchangedEntryHeap(in summary.Summary) summary.Summary {
	out := in.Clone()
	out.HeapTableObjects = nil
	out.HeapKeySpace = nil
	return out
}

func assertResolveReferenceProductionComposition(t *testing.T, prepared *body.Static, base state.State, values []product.Value, got, want summary.Summary) {
	t.Helper()
	reg := standard.Registry()
	caller := parseBranchTransformerFunction(t, `function caller(self, name)
		return resolve_reference(self, name)
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"resolve_reference"}})
	if err != nil {
		t.Fatal(err)
	}
	callerEntry := base
	for i, param := range callerPrepared.OperationPlan().BoundaryParams() {
		callerEntry = callerEntry.WriteValue(reg, statekey.SymbolValue(param), values[i])
	}
	gotResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, got)
	wantResult := solveDynamicIndexCaller(t, callerPrepared, callerEntry, want)
	point := dynamicIndexTransformerCallPoint(t, gotResult)
	wantPoint := dynamicIndexTransformerCallPoint(t, wantResult)
	gotOutcome, gotOK := gotResult.CallOutcomeAt(point)
	wantOutcome, wantOK := wantResult.CallOutcomeAt(wantPoint)
	gotOutcome.HeapTableObjects = nil
	wantOutcome.HeapTableObjects = nil
	if gotOK != wantOK || !reflect.DeepEqual(gotOutcome, wantOutcome) {
		t.Fatalf("prepared CallOutcome differs\n got=%#v\nwant=%#v", gotOutcome, wantOutcome)
	}
	gotState, gotOK := gotResult.StateAtBoundary(point)
	wantState, wantOK := wantResult.StateAtBoundary(wantPoint)
	if gotOK != wantOK {
		t.Fatalf("post-call state presence = %v/%v", gotOK, wantOK)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("state lane catalog = %d, want 17", len(lanes))
	}
	for _, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if gotOK && !domain.Equal(gotState, wantState) {
			t.Fatalf("caller state differs on lane %q", lane)
		}
	}
	gotDiagnostics, _ := json.Marshal(diagnostics.Produce(gotResult))
	wantDiagnostics, _ := json.Marshal(diagnostics.Produce(wantResult))
	if !reflect.DeepEqual(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("diagnostic JSON differs\n got=%s\nwant=%s", gotDiagnostics, wantDiagnostics)
	}
}

func prepareResolveReferenceRelationFixture(t testing.TB) *body.Static {
	t.Helper()
	src, err := os.ReadFile("../../../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := parse.ParseString(string(src), "deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func.Line() == 289 {
			prepared, err := body.PrepareBoundFunction(origin.Func, bindings, body.Config{
				Registry: standard.Registry(), Globals: []string{"uuid"},
				Signatures: signaturelookup.Source{IncludeStdlib: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			return prepared
		}
	}
	t.Fatal("FlowGraph.resolve_reference at line 289 is missing")
	return nil
}
