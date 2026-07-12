package body

import (
	"errors"
	"os"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/poc/semanticprogram"
)

func validateGraphSemanticProgramFixture(t testing.TB) (*Static, []semanticprogram.LayerDecl) {
	t.Helper()
	src, err := os.ReadFile("../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	stmts := parseChunk(t, string(src))
	chunkBindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	var targetOrigin bind.FunctionOrigin
	for _, origin := range chunkBindings.FunctionOrigins() {
		if origin.Func.Line() == 743 {
			targetOrigin = origin
			break
		}
	}
	if targetOrigin.Func == nil {
		t.Fatal("compiler.validate_graph function at line 743 is missing")
	}
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	prepared, err := PrepareBoundFunction(targetOrigin.Func, chunkBindings, Config{
		Registry: standard.Registry(), Globals: []string{"uuid"},
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
		Schedule:      transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if prepared.operationPlan == nil {
		t.Fatal("prepared function has no factflow operation plan")
	}

	var layers []semanticprogram.LayerDecl
	for point, generic := range prepared.genericFors {
		family, role, owner := semanticprogram.GenericForCheck, semanticprogram.Sidecar, semanticprogram.GenericForVariable
		if generic.Role == GenericForRoleVariable {
			family, role, owner = semanticprogram.GenericForVariable, semanticprogram.Executable, ""
		}
		layers = append(layers, semanticprogram.LayerDecl{
			Point: point, Family: family, Role: role, Owner: owner,
			Payload: semanticprogram.PayloadRef{Store: "body.generic-for", Key: uint64(point)},
		})
	}
	observations := compileObservationPlan(prepared.cfg.Graph, prepared.facts, true)
	for _, point := range observations.boundaryPoints {
		layers = append(layers, semanticprogram.LayerDecl{Point: point, Family: semanticprogram.BoundaryObservation, Role: semanticprogram.Observation,
			Payload: semanticprogram.PayloadRef{Store: "observation.boundary", Key: uint64(point)}})
	}
	for _, point := range observations.nodePoints {
		layers = append(layers, semanticprogram.LayerDecl{Point: point, Family: semanticprogram.NodeObservation, Role: semanticprogram.Observation,
			Payload: semanticprogram.PayloadRef{Store: "observation.node", Key: uint64(point)}})
	}
	for _, edge := range observations.edgeReachability {
		layers = append(layers, semanticprogram.LayerDecl{Point: edge.from, To: edge.to, Family: semanticprogram.EdgeObservation, Role: semanticprogram.Observation,
			Payload: semanticprogram.PayloadRef{Store: "observation.edge", Key: uint64(edge.from)<<32 | uint64(edge.to)}})
	}

	// Expression dependencies stay in the immutable factflow store and are
	// referenced once by (family, ExprRef), never copied into point rows.
	refs := make(map[factflow.ExprRef]struct{})
	add := func(family semanticprogram.Family, ref factflow.ExprRef) {
		refs[ref] = struct{}{}
		layers = append(layers, semanticprogram.LayerDecl{Point: prepared.cfg.Graph.Entry(), Family: family, Role: semanticprogram.Dependency,
			Payload: semanticprogram.PayloadRef{Store: "factflow.expression", Key: uint64(ref)}})
	}
	prepared.facts.ForEachObjectLiteral(func(ref factflow.ExprRef, _ factflow.ObjectLiteralView) bool {
		add("source.object-literal", ref)
		return true
	})
	prepared.facts.ForEachExpressionValue(func(ref factflow.ExprRef, _ product.Value) bool { add("source.expression-value", ref); return true })
	prepared.facts.ForEachExpressionOperation(func(ref factflow.ExprRef, _ factflow.ExpressionOperation) bool {
		add("source.expression-operation", ref)
		return true
	})
	prepared.facts.ForEachExpressionRefinement(func(ref factflow.ExprRef, _ factflow.ExpressionRefinement) bool {
		add("source.expression-refinement", ref)
		return true
	})
	prepared.facts.ForEachExpressionPath(func(ref factflow.ExprRef, _ pathdom.Path) bool { add("source.expression-path", ref); return true })
	prepared.facts.ForEachDynamicIndexExpression(func(ref factflow.ExprRef, _ factflow.DynamicIndexExpression) bool {
		add("source.dynamic-index-expression", ref)
		return true
	})
	prepared.facts.ForEachExpressionCondition(func(ref factflow.ExprRef, _ factflow.ExpressionCondition) bool {
		add("source.expression-condition", ref)
		return true
	})
	for ref := range refs {
		if _, ok := prepared.facts.ExpressionFunction(ref); ok {
			add(semanticprogram.ExpressionFunctionDep, ref)
		}
	}
	return prepared, layers
}

func TestSemanticProgramCompilerFailsClosedOnValidateGraphGenericFor(t *testing.T) {
	prepared, layers := validateGraphSemanticProgramFixture(t)
	program, compileErr := semanticprogram.Compile(prepared.cfg.Graph, prepared.operationPlan, layers, nil)
	var missing semanticprogram.MissingError
	if !errors.As(compileErr, &missing) {
		t.Fatalf("Compile error = %v, want MissingError", compileErr)
	}
	if len(missing.Families) != 1 || missing.Families[0] != semanticprogram.GenericForVariable {
		t.Fatalf("missing families = %v, want only %s", missing.Families, semanticprogram.GenericForVariable)
	}
	if len(program.Dependencies) == 0 || len(program.Observations) == 0 {
		t.Fatalf("compiled dependencies=%d observations=%d, want both represented", len(program.Dependencies), len(program.Observations))
	}
	if got := prepared.IdentityDigest(); got == 0 {
		t.Fatal("target body identity is zero")
	} else {
		t.Logf("compiler.validate_graph current body identity=%d points=%d generic=%d dependencies=%d observations=%d", got, prepared.cfg.Graph.Size(), len(prepared.genericFors), len(program.Dependencies), len(program.Observations))
	}
}

func TestSemanticProgramValidateGraphAdmitsConcreteGenericForAndMatchesASTFreeSolve(t *testing.T) {
	prepared, layers := validateGraphSemanticProgramFixture(t)
	if _, err := concreteflow.Compile(prepared.cfg.Graph, prepared.operationPlan, prepared.wtoPlan); err != nil {
		t.Fatalf("concrete semantic program rejected: %v", err)
	}
	program, err := semanticprogram.Compile(prepared.cfg.Graph, prepared.operationPlan, layers, map[semanticprogram.Family]bool{
		semanticprogram.GenericForVariable: true,
	})
	if err != nil || len(program.Missing) != 0 {
		t.Fatalf("Compile with concrete generic-for: program missing=%v err=%v", program.Missing, err)
	}
	want, err := SolvePrepared(prepared, SolveConfig{Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("baseline SolvePrepared: %v", err)
	}
	// The semantic program references the immutable operation store. Prove the
	// concrete solve no longer consults its source-facing GenericForFact AST.
	prepared.genericFors = nil
	got, err := SolvePrepared(prepared, SolveConfig{Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("AST-free SolvePrepared: %v", err)
	}
	domain := state.Domain(prepared.registry)
	for _, point := range prepared.cfg.Graph.RPO() {
		wantState, wantOK := want.StateAt(point)
		gotState, gotOK := got.StateAt(point)
		if wantOK != gotOK || (wantOK && !domain.Equal(wantState, gotState)) {
			t.Fatalf("compiler.validate_graph state differs at point %d", point)
		}
	}
}

func BenchmarkSemanticProgramValidateGraph(b *testing.B) {
	prepared, layers := validateGraphSemanticProgramFixture(b)
	if prepared.concreteFlow == nil {
		b.Fatal("compiler.validate_graph has no concrete semantic program")
	}
	b.Run("compile-fail-closed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = semanticprogram.Compile(prepared.cfg.Graph, prepared.operationPlan, layers, nil)
		}
	})
	b.Run("current-body-solve", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := SolvePrepared(prepared, SolveConfig{Schedule: transfer.ScheduleWTO}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("semantic-program-concrete", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, err := prepared.solveWithFlow(SolveConfig{Schedule: transfer.ScheduleWTO}, func(config transfer.Config) (*bodyFlowTransaction, error) {
				config.ConcreteFlow = prepared.concreteFlow
				config.CanonicalConcreteTransactions = true
				config.FuseConcreteIdentity = true
				flow, err := transfer.TryRun(config)
				return &bodyFlowTransaction{flow: flow}, err
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
