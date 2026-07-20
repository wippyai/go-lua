package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDynamicIndexPlanHandlerPublishesOneOrderedAtomicBoundaryEffect(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	table, keyParam, valueParam := symbol.ID(9301), symbol.ID(9302), symbol.ID(9303)
	tablePath := pathdom.NewPath(table, "table")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9302, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9303, HasExpr: true}
	input := factflow.FactsInput{
		PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
			point: factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(factflow.NewDynamicIndexTarget(tablePath, keySource, nil)),
		},
		DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
			point: factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
				dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			keySource.ExprRef:   pathdom.NewPath(keyParam, "key"),
			valueSource.ExprRef: pathdom.NewPath(valueParam, "value"),
		},
	}
	plan := operationplan.New(graph, input).WithBoundaryParams([]symbol.ID{table, keyParam, valueParam})
	builder := NewBuilder(reg, Shape{Params: 3}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
		locals: make(map[symbol.ID]ValueTerm), expressions: make(map[factflow.ExprRef][]ValueTerm),
	}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 3}); err != nil {
		t.Fatal(err)
	}
	var steps []rowStep
	ctx.rowSteps = &steps
	n3 := dynamicIndexPlanHandler{kind: operationplan.PathDescendantInvalidation}
	n4 := dynamicIndexPlanHandler{kind: operationplan.DynamicIndexWrite}
	if err := n3.Preflight(ctx, point); err != nil {
		t.Fatal(err)
	}
	if err := n4.Preflight(ctx, point); err != nil {
		t.Fatal(err)
	}
	if err := n3.Lower(ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatal("N3 published before the atomic N4 boundary")
	}
	if err := n4.Lower(ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].kind != rowStepEffect || builder.EffectArena().Kind(steps[0].effect) != EffectIndexMutation {
		t.Fatalf("ordered steps = %#v, want one index mutation", steps)
	}
	node := builder.EffectArena().nodes[steps[0].effect]
	if node.site.Owner != uint64(table) || node.site.Ordinal != uint32(point) ||
		node.keyPath == 0 || node.valuePath == 0 || !node.invalidation.PreserveStructuralWitness ||
		!node.invalidation.PreserveDynamicValueMemberships {
		t.Fatalf("index mutation lost boundary provenance: %#v", node)
	}
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	valueValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	cursor, err := NewBindingCursor(Shape{Params: 3},
		[]product.Value{product.Top(), keyValue, valueValue},
		[]pathdom.Path{pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1), pathdom.NewPlaceholder(2)})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := builder.EffectArena().resolve(steps[0].effect, cursor, SpecializationContext{})
	if !ok || resolved.Kind != EffectIndexMutation ||
		!resolved.Mutation.Table.Equal(pathdom.NewPlaceholder(0)) ||
		!resolved.Mutation.KeyPath.Equal(pathdom.NewPlaceholder(1)) ||
		!resolved.Mutation.ValuePath.Equal(pathdom.NewPlaceholder(2)) {
		t.Fatalf("resolved boundary mutation = %#v/%v", resolved, ok)
	}
}

func TestDynamicIndexPlanHandlerPreservesAliasedLexicalRootRoles(t *testing.T) {
	reg := standard.Registry()
	for _, tc := range []struct {
		name                 string
		table, key, value    symbol.ID
		keyField, valueField string
	}{
		{name: "table-key", table: 9501, key: 9501, value: 9502},
		{name: "table-value", table: 9511, key: 9512, value: 9511},
		{name: "key-value", table: 9521, key: 9522, value: 9522},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := cfg.New()
			point := graph.AddNode(cfg.NodeAssign)
			graph.AddEdge(graph.Entry(), point, false)
			graph.AddEdge(point, graph.Exit(), false)
			tablePath := pathdom.NewPath(tc.table, "table")
			keyPath := pathdom.NewPath(tc.key, "key")
			if tc.keyField != "" {
				keyPath = keyPath.Field(tc.keyField)
			}
			valuePath := pathdom.NewPath(tc.value, "value")
			if tc.valueField != "" {
				valuePath = valuePath.Field(tc.valueField)
			}
			keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
			valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2, HasExpr: true}
			input := factflow.FactsInput{
				PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
					point: factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(factflow.NewDynamicIndexTarget(tablePath, keySource, nil)),
				},
				DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
					point: factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
						dynamicindex.AdmissionUnknown, factflow.DynamicIndexReadbackKeyAndValue),
				},
				ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: keyPath, 2: valuePath},
			}
			params := make([]symbol.ID, 0, 3)
			paramIndex := make(map[symbol.ID]int, 3)
			for _, id := range []symbol.ID{tc.table, tc.key, tc.value} {
				if _, exists := paramIndex[id]; exists {
					continue
				}
				paramIndex[id] = len(params)
				params = append(params, id)
			}
			plan := operationplan.New(graph, input).WithBoundaryParams(params)
			shape := Shape{Params: uint32(len(params))}
			builder := NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), plan)
			ctx := planCompileContext{
				registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
				locals: make(map[symbol.ID]ValueTerm), expressions: make(map[factflow.ExprRef][]ValueTerm),
			}
			if err := bindBoundaryParamTerms(&ctx, shape); err != nil {
				t.Fatal(err)
			}
			var steps []rowStep
			ctx.rowSteps = &steps
			if err := (dynamicIndexPlanHandler{kind: operationplan.DynamicIndexWrite}).Lower(ctx, point, nil); err != nil {
				t.Fatal(err)
			}
			if len(steps) != 1 {
				t.Fatalf("ordered effects = %d, want 1", len(steps))
			}
			values := make([]product.Value, len(params))
			paths := make([]pathdom.Path, len(params))
			for index := range params {
				values[index] = product.Top()
				paths[index] = pathdom.NewPlaceholder(index)
			}
			cursor, err := NewBindingCursor(shape, values, paths)
			if err != nil {
				t.Fatal(err)
			}
			resolved, ok := builder.EffectArena().resolve(steps[0].effect, cursor, SpecializationContext{})
			if !ok {
				t.Fatal("aliased-root index mutation did not resolve")
			}
			wantTable := pathdom.NewPlaceholder(paramIndex[tc.table])
			wantKey := pathdom.NewPlaceholder(paramIndex[tc.key])
			if tc.keyField != "" {
				wantKey = wantKey.Field(tc.keyField)
			}
			wantValue := pathdom.NewPlaceholder(paramIndex[tc.value])
			if tc.valueField != "" {
				wantValue = wantValue.Field(tc.valueField)
			}
			if !resolved.Mutation.Table.Equal(wantTable) || !resolved.Mutation.KeyPath.Equal(wantKey) || !resolved.Mutation.ValuePath.Equal(wantValue) {
				t.Fatalf("resolved aliased roles = table %v key %v value %v, want %v/%v/%v",
					resolved.Mutation.Table, resolved.Mutation.KeyPath, resolved.Mutation.ValuePath, wantTable, wantKey, wantValue)
			}
		})
	}
}

func TestDynamicIndexPlanHandlerFailsClosedOutsideBoundaryPair(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	table := symbol.ID(9401)
	tablePath := pathdom.NewPath(table, "table")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9402, HasExpr: true}
	input := factflow.FactsInput{DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
		point: factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, source, nil), source,
			dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue),
	}}
	plan := operationplan.New(graph, input).WithBoundaryParams([]symbol.ID{table})
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: make(map[symbol.ID]ValueTerm)}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := buildBoundaryDynamicIndexEffect(ctx, point); err == nil {
		t.Fatal("unpaired dynamic write was admitted")
	}
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		capability, handled := dynamicIndexEffectCapability(operationplan.DynamicIndexWrite, lane)
		if !handled || capability == CapabilityUnsupported {
			t.Fatalf("dynamic index lane %q has no catalog-derived compiler verdict", lane)
		}
	}
}
