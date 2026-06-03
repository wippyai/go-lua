package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestReconcilePathFactWithDeclaredRead_KeepsRecursiveEvidenceRefinement(t *testing.T) {
	declared := typ.NewOptional(typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	}))
	narrowed := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("recursive path evidence should reconcile with its declared family")
	}
	if got != narrowed {
		t.Fatalf("reconciled type = %v, want narrowed recursive evidence", got)
	}
}

func TestReconcilePathFactWithDeclaredRead_RejectsDiscriminantMismatch(t *testing.T) {
	declared := typ.NewRecord().
		Field("kind", typ.LiteralString("suite")).
		Field("name", typ.String).
		Build()
	narrowed := typ.NewRecord().
		Field("kind", typ.LiteralString("case")).
		Field("name", typ.String).
		Build()

	if got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared); ok {
		t.Fatalf("discriminant mismatch reconciled to %v", got)
	}
}

func TestReconcilePathFactWithDeclaredRead_RejectsNonRecursiveArrayMismatch(t *testing.T) {
	narrowed := typ.NewArray(typ.String)
	declared := typ.NewArray(typ.Number)

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if ok || got != nil {
		t.Fatalf("ReconcilePathFactWithDeclaredRead(array mismatch) = (%v, %v), want rejected", got, ok)
	}
}

func TestReconcilePathFactWithDeclaredRead_KeepsNonNilSolvedSubtypeOfOptionalDeclaredRead(t *testing.T) {
	narrowed := typ.NewTuple(typ.LiteralInt(1), typ.LiteralInt(2), typ.LiteralInt(3))
	declared := typ.NewOptional(typ.NewArray(typ.Any))

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("non-nil solved tuple should reconcile with optional declared array read")
	}
	if !typ.TypeEquals(got, narrowed) {
		t.Fatalf("reconciled type = %v, want solved tuple %v", got, narrowed)
	}
}

func TestReconcilePathFactWithDeclaredRead_KeepsConcreteOverAny(t *testing.T) {
	got, ok := ReconcilePathFactWithDeclaredRead(typ.String, typ.Any)
	if !ok {
		t.Fatal("concrete path fact should reconcile with dynamic any declaration")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("reconciled type = %v, want string", got)
	}
}

func TestReconcilePathFactWithDeclaredRead_KeepsClosedSolvedOverOpenTypeParam(t *testing.T) {
	declared := typ.NewTypeParam("T", nil)

	got, ok := ReconcilePathFactWithDeclaredRead(typ.LiteralInt(10), declared)
	if !ok {
		t.Fatal("closed path fact should reconcile with open type-param declaration")
	}
	if !typ.TypeEquals(got, typ.LiteralInt(10)) {
		t.Fatalf("reconciled type = %v, want literal integer", got)
	}
}

func TestSelectPathObservation_KeepsClosedSolvedOverOpenTypeParamDeclaredRead(t *testing.T) {
	declared := typ.NewTypeParam("T", nil)

	got, ok := SelectPathObservation(typ.LiteralInt(10), nil, declared)
	if !ok {
		t.Fatal("path observation should select closed solved value")
	}
	if !typ.TypeEquals(got, typ.LiteralInt(10)) {
		t.Fatalf("selected observation = %v, want literal integer", got)
	}
}

func TestSelectPathObservation_KeepsSolvedProductOverPlaceholderProof(t *testing.T) {
	record := typ.NewRecord().
		Field("pid", typ.String).
		Field("terminating", typ.Boolean).
		Build()

	got, ok := SelectPathObservation(record, typ.Any, typ.Any)
	if !ok {
		t.Fatal("path observation should select solved product")
	}
	if !typ.TypeEquals(got, record) {
		t.Fatalf("selected observation = %v, want solved record %v", got, record)
	}
}

func TestSelectPathObservation_UsesConditionProofWhenItRefinesSolvedFlow(t *testing.T) {
	proof := typ.LiteralString("ready")

	got, ok := SelectPathObservation(typ.String, proof, typ.Any)
	if !ok {
		t.Fatal("path observation should select proof refinement")
	}
	if !typ.TypeEquals(got, proof) {
		t.Fatalf("selected observation = %v, want proof literal %v", got, proof)
	}
}

func TestSelectPathObservation_KeepsNilRefinementOfOptionalDeclaredRead(t *testing.T) {
	declared := typ.NewOptional(typ.String)

	got, ok := SelectPathObservation(typ.Nil, nil, declared)
	if !ok {
		t.Fatal("path observation should keep explicit nil refinement of nilable declaration")
	}
	if !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("selected observation = %v, want nil", got)
	}
}

func TestSelectSourceProjection_UsesCallpointProjectionOverStaleNilRead(t *testing.T) {
	projected := typ.NewRecord().Field("answer", typ.String).Build()

	got := SelectSourceProjection(typ.Nil, projected)
	if !typ.TypeEquals(got, projected) {
		t.Fatalf("source projection = %v, want callpoint projection %v", got, projected)
	}
}

func TestSelectSourceProjection_PreservesStrictlyMorePreciseSolvedRead(t *testing.T) {
	projected := typ.String
	solved := typ.LiteralString("ready")

	got := SelectSourceProjection(solved, projected)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("source projection = %v, want solved refinement %v", got, solved)
	}
}

func TestReconcilePathFactWithDeclaredRead_UsesDeclaredNonNilWhenFlowIsBroader(t *testing.T) {
	declared := typ.NewOptional(typ.Func().Returns(typ.String).Build())
	narrowed := typ.Func().Returns(typ.Any).Build()

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("function path fact should reconcile with non-nil declared function")
	}
	if _, ok := got.(*typ.Function); !ok {
		t.Fatalf("reconciled type = %T, want function", got)
	}
	if !typ.TypeEquals(got.(*typ.Function).Returns[0], typ.String) {
		t.Fatalf("reconciled function return = %v, want string", got.(*typ.Function).Returns[0])
	}
}

func TestReconcilePathFactWithDeclaredRead_KeepsSolvedFunctionReturnWhenDeclaredReadIsBare(t *testing.T) {
	result := typ.NewRecord().
		Field("answer", typ.String).
		Build()
	declared := typ.Func().Build()
	narrowed := typ.Func().Returns(result).Build()

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("function path fact should reconcile with bare declared function")
	}
	if !typ.TypeEquals(got, narrowed) {
		t.Fatalf("reconciled type = %v, want solved function %v", got, narrowed)
	}
}

func TestReconcilePathFactWithDeclaredRead_UsesDeclaredRecordWhenFlowCarriesInitWitness(t *testing.T) {
	declared := typ.NewRecord().
		Field("run_with", typ.Func().Param("self", typ.Any).Param("db", typ.String).Returns(typ.Any).Build()).
		Build()
	initWitness := typ.NewRecord().Build()
	narrowed := typ.NewUnion(declared, initWitness)

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("annotated record read should reconcile with its declared contract")
	}
	if !typ.TypeEquals(got, declared) {
		t.Fatalf("reconciled type = %v, want declared record %v", got, declared)
	}
}

func TestReconcilePathFactWithDeclaredRead_PreservesAnnotatedOptionalFieldsMissingFromInitWitness(t *testing.T) {
	declaredData := typ.NewRecord().
		OptField("max_tokens", typ.Integer).
		OptField("output_tokens", typ.Integer).
		OptField("dimensions", typ.Any).
		Build()
	declared := typ.NewRecord().
		Field("data", typ.NewOptional(declaredData)).
		Build()
	narrowed := typ.NewRecord().
		Field("data", typ.NewRecord().
			Field("max_tokens", typ.Integer).
			Build()).
		Build()
	want := typ.NewRecord().
		Field("data", typ.NewRecord().
			Field("max_tokens", typ.Integer).
			OptField("output_tokens", typ.Integer).
			OptField("dimensions", typ.Any).
			Build()).
		Build()

	got, ok := ReconcilePathFactWithDeclaredRead(narrowed, declared)
	if !ok {
		t.Fatal("annotated optional record fields should reconcile with initializer witness")
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("reconciled type = %v, want %v", got, want)
	}
}

func TestReconcilePathFactWithDeclaredRead_OverlaysPartialOpenRecordOnDeclaredProduct(t *testing.T) {
	declared := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build()
	overlay := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()
	want := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		Field("prefix", typ.String).
		Build()

	got, ok := ReconcilePathFactWithDeclaredRead(overlay, declared)
	if !ok {
		t.Fatal("partial structured overlay should reconcile with declared receiver product")
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("reconciled type = %v, want %v", got, want)
	}
}

func TestReconcilePathFactWithDeclaredRead_PreservesDeclaredAliasOnProductOverlay(t *testing.T) {
	declaredRecord := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build()
	declared := typ.NewAlias("Builder", declaredRecord)
	overlay := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()

	got, ok := ReconcilePathFactWithDeclaredRead(overlay, declared)
	if !ok {
		t.Fatal("partial structured overlay should reconcile with declared alias product")
	}
	alias, ok := got.(*typ.Alias)
	if !ok || alias.Name != "Builder" {
		t.Fatalf("reconciled type = %T %v, want Builder alias", got, got)
	}
	target, ok := alias.Target.(*typ.Record)
	if !ok || target.GetField("build") == nil {
		t.Fatalf("reconciled alias target lost declared product fields: %v", alias.Target)
	}
	prefix := target.GetField("prefix")
	if prefix == nil || prefix.Optional {
		t.Fatalf("reconciled alias target should make prefix present, got %v", alias.Target)
	}
}

func TestReconcilePathFactWithDeclaredRead_PreservesRecursiveAliasOnProductOverlay(t *testing.T) {
	declaredFamily := typ.NewRecursive("HandlerBuilder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			OptField("name", typ.String).
			OptField("prefix", typ.String).
			Field("prefix_with", typ.Func().Param("self", self).Param("prefix", typ.String).Returns(self).Build()).
			Field("build", typ.Func().Param("self", self).Returns(typ.Func().Returns(typ.String).Build()).Build()).
			Build()
	})
	declared := typ.NewAlias("Builder", declaredFamily)
	overlay := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()

	got, ok := ReconcilePathFactWithDeclaredRead(overlay, declared)
	if !ok {
		t.Fatal("partial structured overlay should reconcile with declared recursive alias product")
	}
	alias, ok := got.(*typ.Alias)
	if !ok || alias.Name != "Builder" {
		t.Fatalf("reconciled type = %T %v, want Builder alias", got, got)
	}
	if !typ.TypeEquals(alias.Target, declaredFamily) {
		t.Fatalf("recursive alias target changed: got %v want %v", alias.Target, declaredFamily)
	}
}

func TestReconcilePathFactWithDeclaredRead_RejectsPartialOpenOverlayWithUndeclaredFieldOnClosedProduct(t *testing.T) {
	declared := typ.NewRecord().
		Field("build", typ.Func().Returns(typ.String).Build()).
		Build()
	overlay := typ.NewRecord().
		SetOpen(true).
		Field("unknown_field", typ.String).
		Build()

	if got, ok := ReconcilePathFactWithDeclaredRead(overlay, declared); ok {
		t.Fatalf("undeclared partial overlay reconciled to %v", got)
	}
}

func TestReconcileDeclaredBoundary_RejectsNilableActualAgainstNonNilDeclared(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("x", typ.String).
			Build(),
		typ.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("y", typ.String).
			Build(),
	)

	if got, ok := ReconcileDeclaredBoundary(typ.NewOptional(action), action); ok {
		t.Fatalf("nilable actual crossed non-nil declared boundary as %v", got)
	}
	if got, ok := ReconcileDeclaredBoundary(typ.Nil, action); ok {
		t.Fatalf("nil crossed non-nil declared boundary as %v", got)
	}
}

func TestReconcileDeclaredBoundary_AcceptsReconciledProductWitnessWhenNilabilityPreserved(t *testing.T) {
	declared := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build()
	overlay := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()

	got, ok := ReconcileDeclaredBoundary(overlay, declared)
	if !ok {
		t.Fatal("consistent reconciled product witness should cross declared boundary")
	}
	if !typ.TypeEquals(got, declared) && !typ.TypeEquals(got, typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		Field("prefix", typ.String).
		Build()) {
		t.Fatalf("reconciled boundary witness = %v, want declared-compatible product", got)
	}
}
