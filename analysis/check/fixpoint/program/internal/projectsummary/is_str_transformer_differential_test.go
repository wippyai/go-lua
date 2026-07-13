package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
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
		})
	}
}
