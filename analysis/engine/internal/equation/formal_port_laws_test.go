package equation

import "testing"

func formalPortRead(slot, factor byte, local uint64) PortRead {
	return PortRead{Role: boundaryKey(slot), Surface: Surface{Factor: boundaryKey(factor), Form: SurfaceReadExact, Local: local}}
}

func sealedFormalPortBatch(t testing.TB, reversed bool) (*Batch, FormalPort, FormalPort, FormalPort) {
	t.Helper()
	batch := NewBatch()
	reads := []PortRead{formalPortRead(41, 51, 1), formalPortRead(42, 52, 2)}
	if reversed {
		reads[0], reads[1] = reads[1], reads[0]
	}
	input, inputOK := batch.AdmitFormalPort(boundaryKey(31), PortImport, reads)
	output, outputOK := batch.AdmitFormalPort(boundaryKey(32), PortExport, nil)
	both, bothOK := batch.AdmitFormalPort(boundaryKey(33), PortImportExport, nil)
	if !inputOK || !outputOK || !bothOK || !batch.Seal() {
		t.Fatal("formal Batch")
	}
	return batch, input, output, both
}

func sealedConcretePortBatch(t testing.TB, withDecision bool) (*Batch, Site, Site) {
	t.Helper()
	scope := EmptyScope()
	if withDecision {
		var ok bool
		scope, ok = NewScope(boundaryDecision(t, 61))
		if !ok {
			t.Fatal("concrete scope")
		}
	}
	batch := NewBatch()
	left, leftOK := batch.AdmitSite(boundaryKey(62), scope, FalseExpr(), InitAbsent)
	right, rightOK := batch.AdmitSite(boundaryKey(63), scope, FalseExpr(), InitAbsent)
	if !leftOK || !rightOK || !batch.Seal() {
		t.Fatal("concrete Batch")
	}
	return batch, left, right
}

func exactFormalPortActuals(input, output, both FormalPort, left, right Site) []FormalPortActual {
	return []FormalPortActual{
		{Role: input, Site: left, Reads: []PortRead{formalPortRead(42, 52, 202), formalPortRead(41, 51, 101)}},
		{Role: output, Site: right},
		// Distinct formal roles may intentionally share one concrete endpoint;
		// uniqueness is one assignment per formal capability, not injectivity of
		// the caller's Point relation.
		{Role: both, Site: left},
	}
}

func TestFormalPortIdentityIsStableAndOwnerFenced(t *testing.T) {
	leftBatch, leftInput, leftOutput, leftBoth := sealedFormalPortBatch(t, false)
	rightBatch, rightInput, rightOutput, rightBoth := sealedFormalPortBatch(t, true)
	if leftBatch.Key() != rightBatch.Key() || leftInput.Site().Key() != rightInput.Site().Key() ||
		leftOutput.Site().Key() != rightOutput.Site().Key() || leftBoth.Site().Key() != rightBoth.Site().Key() {
		t.Fatal("equivalent formal replay changed canonical identity")
	}
	if leftInput.Same(rightInput) || leftBatch.OwnsSite(rightInput.Site()) || rightBatch.OwnsSite(leftInput.Site()) {
		t.Fatal("equal replay crossed exact Batch ownership")
	}
	if leftInput.Mode() != PortImport || leftInput.ReadCount() != 2 || leftInput.Role() != boundaryKey(31) {
		t.Fatal("formal ABI projection")
	}
	first, firstOK := leftInput.ReadAt(0)
	second, secondOK := leftInput.ReadAt(1)
	if !firstOK || !secondOK || first.Role != boundaryKey(41) || second.Role != boundaryKey(42) {
		t.Fatal("formal reads were not canonicalized by role")
	}
	formalSite := leftInput.Site()
	init, disposition, initOK := formalSite.Init()
	port, portOK := formalSite.FormalPort()
	if !initOK || disposition != InitAbsent || !init.IsFalse() || formalSite.Scope().Count() != 0 || !portOK || !port.Same(leftInput) {
		t.Fatal("formal Site did not retain its closed empty boundary")
	}
	formalInput := BoundaryInput(leftInput.Site(), leftOutput.Site(), boundaryKey(34), TrueExpr(), IdentityReindex(EmptyScope()), TrueExpr())
	if !formalInput.Available() {
		t.Fatal("ordinary Input grammar rejected same-Batch formal Sites")
	}
}

func TestTemplateBindingRequiresTotalExactForeignSafeAssignment(t *testing.T) {
	formals, input, output, both := sealedFormalPortBatch(t, false)
	actuals, left, right := sealedConcretePortBatch(t, false)
	values := exactFormalPortActuals(input, output, both, left, right)
	binding, ok := SealTemplateBinding(formals, actuals, values)
	if !ok || !binding.Available() || !binding.Key().Available() {
		t.Fatal("exact total binding")
	}
	if replay := (TemplateBinding{data: binding.data}); !replay.Same(binding) {
		t.Fatal("copied binding lost exact authority")
	}
	if _, accepted := SealTemplateBinding(formals, actuals, values[:2]); accepted {
		t.Fatal("partial formal assignment accepted")
	}
	duplicate := append([]FormalPortActual(nil), values...)
	duplicate[2].Role = input
	if _, accepted := SealTemplateBinding(formals, actuals, duplicate); accepted {
		t.Fatal("duplicate formal assignment accepted")
	}
	foreignFormals, foreignInput, _, _ := sealedFormalPortBatch(t, false)
	foreign := append([]FormalPortActual(nil), values...)
	foreign[0].Role = foreignInput
	if _, accepted := SealTemplateBinding(formals, actuals, foreign); accepted || foreignFormals == formals {
		t.Fatal("foreign equal formal capability accepted")
	}
	foreignActuals, foreignLeft, _ := sealedConcretePortBatch(t, false)
	foreign = append([]FormalPortActual(nil), values...)
	foreign[0].Site = foreignLeft
	if _, accepted := SealTemplateBinding(formals, actuals, foreign); accepted || foreignActuals == actuals {
		t.Fatal("foreign equal concrete capability accepted")
	}
	formalActuals, formalLeft, formalRight, formalBoth := sealedFormalPortBatch(t, false)
	if _, accepted := SealTemplateBinding(formals, formalActuals, exactFormalPortActuals(input, output, both, formalLeft.Site(), formalRight.Site())); accepted || !formalBoth.Available() {
		t.Fatal("unresolved formal Site accepted as a concrete actual")
	}
}

func TestTemplateBindingPreservesDirectionAndReadABI(t *testing.T) {
	formals, input, output, both := sealedFormalPortBatch(t, false)
	actuals, left, right := sealedConcretePortBatch(t, true)
	values := exactFormalPortActuals(input, output, both, left, right)
	binding, ok := SealTemplateBinding(formals, actuals, values)
	if !ok {
		t.Fatal("binding")
	}
	resolved, reads, ingress, importOK := binding.ResolveImport(input)
	if !importOK || !resolved.Same(left) || len(reads) != 2 || reads[0].Role != boundaryKey(41) || reads[0].Surface.Local != 101 ||
		!sameScope(ingress.Source(), left.Scope()) || !sameScope(ingress.Target(), input.Site().Scope()) || ingress.Count() != left.Scope().Count() {
		t.Fatal("import resolution/read substitution")
	}
	for index := 0; index < ingress.Count(); index++ {
		mapping, present := ingress.At(index)
		if !present || mapping.Disposition != DecisionForget {
			t.Fatal("import scope was not explicitly projected")
		}
	}
	if _, _, _, accepted := binding.ResolveImport(output); accepted {
		t.Fatal("export-only port resolved as import")
	}
	if _, _, accepted := binding.ResolveExport(input); accepted {
		t.Fatal("import-only port resolved as export")
	}
	if resolved, egress, accepted := binding.ResolveExport(output); !accepted || !resolved.Same(right) ||
		!sameScope(egress.Source(), output.Site().Scope()) || !sameScope(egress.Target(), right.Scope()) || egress.Count() != 0 {
		t.Fatal("export resolution")
	}
	if _, _, _, accepted := binding.ResolveImport(both); !accepted {
		t.Fatal("import-export port rejected as import")
	}
	if _, _, accepted := binding.ResolveExport(both); !accepted {
		t.Fatal("import-export port rejected as export")
	}

	wrongFactor := exactFormalPortActuals(input, output, both, left, right)
	wrongFactor[0].Reads[0].Surface.Factor = boundaryKey(99)
	if _, accepted := SealTemplateBinding(formals, actuals, wrongFactor); accepted {
		t.Fatal("read Factor substitution accepted")
	}
	wrongSlot := exactFormalPortActuals(input, output, both, left, right)
	wrongSlot[0].Reads[0].Role = boundaryKey(98)
	if _, accepted := SealTemplateBinding(formals, actuals, wrongSlot); accepted {
		t.Fatal("read-slot rename accepted")
	}
}

func TestFormalBindingDoesNotRelaxInputOrRetainConcreteSite(t *testing.T) {
	formals, input, output, both := sealedFormalPortBatch(t, false)
	actuals, left, right := sealedConcretePortBatch(t, true)
	formalKey, formalSource, formalScope := input.Site().Key(), input.Site().Source(), input.Site().Scope().Key()
	binding, ok := SealTemplateBinding(formals, actuals, exactFormalPortActuals(input, output, both, left, right))
	if !ok {
		t.Fatal("binding")
	}
	if input.Site().Key() != formalKey || input.Site().Source() != formalSource || input.Site().Scope().Key() != formalScope || input.Site().Scope().Count() != 0 {
		t.Fatal("formal Site retained or absorbed concrete Site state")
	}
	if boundary := BoundaryInput(input.Site(), left, boundaryKey(71), TrueExpr(), IdentityReindex(EmptyScope()), TrueExpr()); boundary.Available() {
		t.Fatal("TemplateBinding relaxed ordinary cross-Batch Input ownership")
	}
	if validTopologyBatch(formals, TopologySpec{Batch: formals, Points: []PointSpec{{Site: input.Site()}}}) {
		t.Fatal("unresolved formal Site entered an ordinary Topology")
	}
	if resolved, _, ingress, resolvedOK := binding.ResolveImport(input); !resolvedOK || !resolved.Same(left) || resolved.Scope().Count() != 1 || !ingress.Available() {
		t.Fatal("binding did not retain the exact concrete Site separately")
	}
}

func TestFormalPortRejectsDirectionAndABIRewrites(t *testing.T) {
	batch := NewBatch()
	if _, ok := batch.AdmitFormalPort(boundaryKey(81), PortExport, []PortRead{formalPortRead(82, 83, 1)}); ok || batch.Seal() {
		t.Fatal("export-only port admitted an import read")
	}

	batch = NewBatch()
	first, firstOK := batch.AdmitFormalPort(boundaryKey(84), PortImport, nil)
	if !firstOK {
		t.Fatal("initial formal port")
	}
	again, againOK := batch.AdmitFormalPort(boundaryKey(84), PortImport, nil)
	if !againOK || again.row != first.row {
		t.Fatal("exact formal replay did not return one open capability")
	}
	if _, ok := batch.AdmitFormalPort(boundaryKey(84), PortExport, nil); ok || first.Available() || batch.Seal() {
		t.Fatal("same role admitted a second direction")
	}
}
