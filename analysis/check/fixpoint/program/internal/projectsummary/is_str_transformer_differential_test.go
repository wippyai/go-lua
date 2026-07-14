package projectsummary

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestParsedIsStrTransformerMatchesCanonicalConcreteProjection(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `function is_str(value: any): boolean
		return type(value) == "string" and (value :: string) ~= ""
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	shape := transformer.Shape{Params: uint32(len(params)), Globals: uint32(len(plan.BoundaryGlobals()))}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("is_str relation compiled contextually: %s", reason)
	}
	requirements, ok := plan.ObservationRequirements()
	if !ok {
		t.Fatal("is_str observation requirements are not sealed")
	}
	if got := len(requirements.Entries(false)); got != 10 {
		t.Fatalf("is_str selected requirements = %d, want 10", got)
	}
	surface, ok := plan.CallSurface()
	if !ok {
		t.Fatal("is_str call surface is not sealed")
	}
	view, err := evaluated.SealProjectionView(requirements, false)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable}
	identity := evaluated.Identity{
		Body: surface.Owner(), Relation: unavailable, Entry: unavailable, Lineage: unavailable, Registry: unavailable,
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(), Inventory: requirements.ConsumerInventoryID(),
		View: view, PointCount: uint32(prepared.Graph().Size()),
	}
	for _, test := range []struct {
		name  string
		value product.Value
	}{
		{name: "nonempty-string", value: typevalue.LiteralString(reg, "value")},
		{name: "empty-string", value: typevalue.LiteralString(reg, "")},
		{name: "number", value: typevalue.LiteralNumber(reg, 7)},
		{name: "top", value: product.Top()},
	} {
		t.Run(test.name, func(t *testing.T) {
			bindings := make([]product.Value, shape.ValueCount())
			bindings[0] = test.value
			for i := 1; i < len(bindings); i++ {
				bindings[i] = product.Top()
			}
			cursor, cursorErr := transformer.NewBindingCursor(shape, bindings, nil)
			if cursorErr != nil {
				t.Fatal(cursorErr)
			}
			got, exact := relation.Specialize(cursor, nil, nil)
			if !exact {
				t.Fatal("symbolic specialization failed")
			}
			concrete, solveErr := body.SolvePrepared(prepared, body.SolveConfig{
				EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), test.value),
			})
			if solveErr != nil {
				t.Fatal(solveErr)
			}
			want := summary.Normalize(reg, FromResult(concrete))
			if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
				t.Fatalf("symbolic/canonical concrete summary differs\n got=%#v\nwant=%#v", got, want)
			}
			root, rootErr := relation.EvaluateSparseRoot(context.Background(), transformer.EvaluatedRootRequest{
				Identity: identity, ExpectedIdentity: identity, Requirements: requirements, CallSurface: surface,
			}, cursor, transformer.SpecializationContext{})
			if rootErr != nil {
				t.Fatal(rootErr)
			}
			if root.Authoritative() {
				t.Fatal("shadow is_str root became authoritative")
			}
			if coverage := root.Coverage(); coverage.Required != 10 || coverage.Points != 7 || coverage.Boundaries != 1 || coverage.Edges != 2 || !coverage.Complete() {
				t.Fatalf("is_str coverage = %#v, want exact 7/1/2 = 10", coverage)
			}
			if !summary.EqualNormalized(reg, root.Summary(), want) {
				t.Fatal("evaluated root owner summary differs from concrete Result")
			}
			for _, point := range root.Points() {
				if gotReachable, wantReachable := point.Worlds.Root != evaluated.DecisionFalse, concrete.PointReachable(point.Point); gotReachable != wantReachable {
					t.Fatalf("point %d reachable = %v, want %v", point.Point, gotReachable, wantReachable)
				}
			}
			for _, edge := range root.Edges() {
				if gotNormal, wantNormal := edge.Worlds.Root != evaluated.DecisionFalse, concrete.EdgeCanCompleteNormally(edge.From, edge.To); gotNormal != wantNormal {
					t.Fatalf("edge %d->%d normal = %v, want %v", edge.From, edge.To, gotNormal, wantNormal)
				}
			}
			boundaries := root.Boundaries()
			if len(boundaries) != 1 {
				t.Fatalf("Return boundaries = %d", len(boundaries))
			}
			var returns []product.Value
			for _, fragment := range boundaries[0].Fragments {
				if fragment.Worlds.Root == evaluated.DecisionFalse {
					continue
				}
				for _, value := range fragment.Values {
					for len(returns) <= int(value.Index) {
						returns = append(returns, product.Bottom(reg))
					}
					returns[value.Index] = summary.JoinReturnValue(reg, returns[value.Index], value.Value)
				}
			}
			if len(returns) != len(want.Returns) {
				t.Fatalf("Return boundary arity = %d, want %d", len(returns), len(want.Returns))
			}
			for index := range returns {
				if !product.Equal(reg, returns[index], want.Returns[index]) {
					t.Fatalf("Return boundary slot %d differs", index)
				}
			}
		})
	}
}
