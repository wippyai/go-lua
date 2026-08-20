package binaryprimitive

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/position"
	"github.com/wippyai/go-lua/analysis/program/flow/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

type binaryPrimitiveFixture struct {
	sourceView source.View
	flow       authored.View
	executable *executable.Result
	causal     *causal.Result
	staticID   identity.ContentID
	moduleID   identity.ContentID

	staticView  staticquery.View
	flowFinal   authored.Finalizer
	moduleFinal imports.Finalizer
}

func openBinaryPrimitiveFixture(t *testing.T, comparisonOp flowkind.BinaryOp) *binaryPrimitiveFixture {
	t.Helper()
	const binaryCount = 19
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	trueBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	falseBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	nilCount := uint32(20)
	returnTrue := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	returnFalse := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	valuesTrue := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	valuesFalse := keyspace.MakeTerm(keyspace.FamilyValues, 2)

	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 3
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyBinary] = binaryCount
	counts[keyspace.FamilyNil] = nilCount
	counts[keyspace.FamilyReturn] = 2
	counts[keyspace.FamilyValues] = 2

	name := "binaryprimitive-real.lua"
	sourceInput := source.Input{Name: name}
	sourceInput.Families = flowtest.FamilySpans(name, counts)
	sourceInput.Nil = make([]source.NilLiteral, nilCount)
	for index := range sourceInput.Nil {
		sourceInput.Nil[index] = source.NilLiteral{Owner: body}
	}
	bodyRoots := []keyspace.Term{branch}
	flowBinaries := make([]authored.Binary, binaryCount)
	ops := []flowkind.BinaryOp{
		flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul,
		flowkind.BinaryDiv, flowkind.BinaryIDiv, flowkind.BinaryMod,
		flowkind.BinaryPow, flowkind.BinaryConcat, flowkind.BinaryBitAnd,
		flowkind.BinaryBitOr, flowkind.BinaryBitXor, flowkind.BinaryShiftLeft,
		flowkind.BinaryShiftRight, flowkind.BinaryEqual, flowkind.BinaryNotEqual,
		flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater,
		flowkind.BinaryGreaterEqual,
	}
	ops[13] = comparisonOp
	for index, op := range ops {
		left, right := keyspace.Term(0), keyspace.Term(0)
		switch {
		case index == 0:
			left, right = keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)
		case index < 13:
			left, right = keyspace.MakeTerm(keyspace.FamilyBinary, uint32(index)), keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+2))
		case index == 13:
			left, right = keyspace.MakeTerm(keyspace.FamilyBinary, 13), keyspace.MakeTerm(keyspace.FamilyBinary, 19)
		case index == 14:
			left, right = keyspace.MakeTerm(keyspace.FamilyNil, 15), keyspace.MakeTerm(keyspace.FamilyNil, 16)
		default:
			left, right = keyspace.MakeTerm(keyspace.FamilyBinary, uint32(index)), keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+2))
		}
		flowBinaries[index] = authored.Binary{Owner: body, Op: op, Left: left, Right: right}
	}
	sourceInput.Bodies = []source.BodySource{
		{Body: body, Terms: bodyRoots},
		{Body: trueBody, Terms: []keyspace.Term{returnTrue}},
		{Body: falseBody, Terms: []keyspace.Term{returnFalse}},
	}

	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinal, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinal.Preimage()

	staticInput := static.Input{Counts: [keyspace.FamilyCount]uint32{}}
	staticInput.Counts[keyspace.FamilyBody] = counts[keyspace.FamilyBody]
	_, staticView, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinal.Abort()
		t.Fatalf("static.Build: %v", err)
	}

	flowInput := authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: trueBody}, {Owner: falseBody}}},
		Control: authored.ControlInput{
			Returns:  []authored.Return{{Owner: trueBody, Values: valuesTrue}, {Owner: falseBody, Values: valuesFalse}},
			Branches: []authored.Branch{{Owner: body, Condition: keyspace.MakeTerm(keyspace.FamilyBinary, 14), WhenTrue: trueBody, WhenFalse: falseBody}},
		},
		Operators: authored.OperatorsInput{Binaries: flowBinaries},
	}
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinal, err := flowDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinal.View()

	bodies, err := flowbody.Seal(preimage, flowView, staticView, body)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, body)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinal, err := moduleDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinal.View()
	staticID, moduleID := staticView.ContentID(), moduleView.ContentID()
	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, body)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, moduleFinal)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, moduleFinal)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, moduleFinal)
		t.Fatalf("outcome.Seal: %v", err)
	}
	ports, err := evaluation.SealPorts(preimage.Identity(), flowView, forest, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, moduleFinal)
		t.Fatalf("evaluation.SealPorts: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, body, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, flowFinal, moduleFinal)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinal.Commit(indexInput)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	graph, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, body, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	cellRoles := sourceView.CellRoles()
	if !cellRoles.Matches(sourceView) {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatal("source.CellRoles: unavailable")
	}
	certificate, certificateErr := semanticpath.Seal(cellRoles, sourceView, flowView, bodies, bindingResult, forest, outcomes, flowView.ContentID(), staticID, moduleID)
	if certificateErr != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("semanticpath.Seal: %v", certificateErr)
	}
	vertexPaths, pathsOK := certificate.VertexCatalog(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	vertexLease, vertexErr := graph.InstallVertexCatalogLease(bodies, vertexPaths)
	if !pathsOK || vertexErr != nil || vertexLease == nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatal("sourcecontrol.InstallVertexCatalog: no exact path view")
	}
	defer graph.ReleaseVertexCatalog(vertexLease)
	outcomePaths, outcomePathsOK := certificate.OutcomePhases(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	outcomePhases, outcomeErr := graph.BuildOutcomePhases(sourceView, flowView, bodies, outcomes, outcomePaths)
	if !outcomePathsOK || outcomeErr != nil || outcomePhases == nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatal("sourcecontrol.BuildOutcomePhases: unavailable")
	}
	executableResult, err := executable.Seal(sourceView, flowView, bodies, forest, graph, staticID, moduleID, certificate)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("executable.Seal: %v", err)
	}
	entries, err := runtimeentry.Seal(sourceView, flowView, graph, ports, executableResult, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("runtimeentry.Seal: %v", err)
	}
	causalPaths, pathsOK := certificate.Causal(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	if !pathsOK {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatal("semanticpath.Causal: view unavailable")
	}
	preparation, err := causal.PrepareRoutePlanWithStructuralPaths(sourceView, flowView, bodies, forest, outcomes, graph, ports, executableResult, entries, causalPaths, outcomePhases, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("causal.PrepareRoutePlan: %v", err)
	}
	causalResult, err := preparation.Seal()
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinal, moduleFinal)
		t.Fatalf("causal.Preparation.Seal: %v", err)
	}
	fixture := &binaryPrimitiveFixture{sourceView: sourceView, flow: flowView, executable: executableResult, causal: causalResult, staticID: staticID, moduleID: moduleID, staticView: staticView, flowFinal: flowFinal, moduleFinal: moduleFinal}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(source.Finalizer{}, fixture.flowFinal, fixture.moduleFinal)
	})
	return fixture
}

func TestBinaryPrimitiveSealAllExplicitCategoriesAndRealComparison(t *testing.T) {
	fixture := openBinaryPrimitiveFixture(t, flowkind.BinaryEqual)
	candidateResult, err := candidates.Seal(fixture.sourceView.Identity(), fixture.flow, fixture.executable, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	result, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("binaryprimitive.Seal: %v", err)
	}
	if got, want := result.Arithmetic().Count(), 7; got != want {
		t.Fatalf("arithmetic count = %d, want %d", got, want)
	}
	if got, want := result.Bitwise().Count(), 5; got != want {
		t.Fatalf("bitwise count = %d, want %d", got, want)
	}
	if got, want := result.Equality().Count(), 2; got != want {
		t.Fatalf("equality count = %d, want %d", got, want)
	}
	if got, want := result.Order().Count(), 4; got != want {
		t.Fatalf("order count = %d, want %d", got, want)
	}
	if _, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 8)); ok {
		t.Fatal("Concat entered the primitive projection")
	}
	primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 14))
	if !ok {
		t.Fatal("equality Branch condition was not retained")
	}
	comparison, ok := primitive.Comparison()
	if !ok || comparison.Branch != keyspace.MakeTerm(keyspace.FamilyBranch, 1) || comparison.TrueBody != keyspace.MakeTerm(keyspace.FamilyBody, 2) || comparison.FalseBody != keyspace.MakeTerm(keyspace.FamilyBody, 3) || comparison.Invert {
		t.Fatalf("real causal comparison = %#v/%v", comparison, ok)
	}
	branchless, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
	if !ok {
		t.Fatal("branchless primitive was not retained")
	}
	if _, ok := branchless.Comparison(); ok {
		t.Fatal("branchless primitive exposed a comparison")
	}
}

func TestBinaryPrimitiveSealRealComparisonNormalForms(t *testing.T) {
	cases := []struct {
		name   string
		op     flowkind.BinaryOp
		invert bool
		swap   bool
	}{
		{name: "equal", op: flowkind.BinaryEqual},
		{name: "not-equal", op: flowkind.BinaryNotEqual, invert: true},
		{name: "less", op: flowkind.BinaryLess},
		{name: "less-equal", op: flowkind.BinaryLessEqual},
		{name: "greater", op: flowkind.BinaryGreater, swap: true},
		{name: "greater-equal", op: flowkind.BinaryGreaterEqual, swap: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := openBinaryPrimitiveFixture(t, test.op)
			candidateResult, err := candidates.Seal(fixture.sourceView.Identity(), fixture.flow, fixture.executable, fixture.staticID, fixture.moduleID)
			if err != nil {
				t.Fatalf("candidates.Seal: %v", err)
			}
			result, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, fixture.staticID, fixture.moduleID)
			if err != nil {
				t.Fatalf("binaryprimitive.Seal: %v", err)
			}
			primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 14))
			if !ok {
				t.Fatal("real comparison primitive was not retained")
			}
			operation, ok := primitive.Operation()
			if !ok || operation.Op != test.op {
				t.Fatalf("raw operation = %#v/%v, want %v", operation, ok, test.op)
			}
			comparison, ok := primitive.Comparison()
			if !ok || comparison.Branch != keyspace.MakeTerm(keyspace.FamilyBranch, 1) || comparison.TrueBody != keyspace.MakeTerm(keyspace.FamilyBody, 2) || comparison.FalseBody != keyspace.MakeTerm(keyspace.FamilyBody, 3) || comparison.Invert != test.invert {
				t.Fatalf("comparison metadata = %#v/%v", comparison, ok)
			}
			wantLeft, wantRight := operation.Left, operation.Right
			if test.swap {
				wantLeft, wantRight = wantRight, wantLeft
			}
			if comparison.Left != wantLeft || comparison.Right != wantRight {
				t.Fatalf("comparison operands = %v/%v, want %v/%v", comparison.Left, comparison.Right, wantLeft, wantRight)
			}
		})
	}
}

func TestBinaryPrimitiveSealRejectsForeignOrUnavailableProvenance(t *testing.T) {
	fixture := openBinaryPrimitiveFixture(t, flowkind.BinaryEqual)
	candidateResult, err := candidates.Seal(fixture.sourceView.Identity(), fixture.flow, fixture.executable, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	if _, err := Seal(fixture.sourceView, fixture.flow, nil, fixture.causal, fixture.staticID, fixture.moduleID); err == nil {
		t.Fatal("nil candidate result was admitted")
	}
	if _, err := Seal(fixture.sourceView, fixture.flow, candidateResult, nil, fixture.staticID, fixture.moduleID); err == nil {
		t.Fatal("nil causal result was admitted")
	}
	foreign := fixture.staticID
	foreign[0] ^= 0xff
	if _, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, foreign, fixture.moduleID); err == nil {
		t.Fatal("foreign Static identity was admitted")
	}
	if _, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, identity.ContentID{}, fixture.moduleID); err == nil {
		t.Fatal("unavailable Static identity was admitted")
	}
}
