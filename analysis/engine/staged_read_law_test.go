package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// stagedLawUnits issues real attached opaque Units solely for route-order
// laws. No Factor key or typed payload crosses into Selection.
func stagedLawUnits(t testing.TB) []carrier.Unit {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := factbinding.Admit(uint64(2), uint64(0), lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return ^uint64(0) },
		Equal:    func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join:     func(left, right uint64) uint64 { return left | right },
		Widen:    func(left, right uint64) uint64 { return left | right },
	}, func(uint64, uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("staged route algebra")
	}
	units := make([]carrier.Unit, 2)
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for key := range units {
			unit, declared := binding.DeclareExact(uint64(key))
			if !declared {
				return false
			}
			units[key] = unit
		}
		return true
	})
	if !ok {
		t.Fatal("staged route binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("staged route prepare")
	}
	if _, ok := prepared.Attach(); !ok {
		t.Fatal("staged route attach")
	}
	return units
}

func TestStagedReadCanonicalizesSemanticRoutesAndPhysicalUnits(t *testing.T) {
	units := stagedLawUnits(t)
	session := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	routes := []stagedRoute[uint64]{
		{unit: units[1], tag: 9},
		{unit: units[0], tag: 7},
		{unit: units[1], tag: 3},
		{unit: units[0], tag: 2},
	}
	physical, indexes, ok := session.indexRoutes(routes)
	if !ok || len(physical) != 2 || len(indexes) != 4 {
		t.Fatalf("canonical routes = physical:%#v indexes:%#v ok:%t", physical, indexes, ok)
	}
	if !routes[0].unit.Same(units[0]) || routes[0].tag != 2 || !routes[1].unit.Same(units[0]) || routes[1].tag != 7 ||
		!routes[2].unit.Same(units[1]) || routes[2].tag != 3 || !routes[3].unit.Same(units[1]) || routes[3].tag != 9 {
		t.Fatalf("observable route order = %#v", routes)
	}
	if !physical[0].Same(units[0]) || !physical[1].Same(units[1]) || indexes[0] != 0 || indexes[1] != 0 || indexes[2] != 1 || indexes[3] != 1 {
		t.Fatalf("physical route partition = physical:%#v indexes:%#v", physical, indexes)
	}

	duplicates := []stagedRoute[uint64]{{unit: units[0], tag: 1}, {unit: units[0], tag: 1}}
	if _, _, accepted := session.indexRoutes(duplicates); accepted {
		t.Fatal("duplicate (Unit, tag) route was accepted")
	}
	distinctTags := []stagedRoute[uint64]{{unit: units[0], tag: 8}, {unit: units[0], tag: 1}}
	physical, indexes, ok = session.indexRoutes(distinctTags)
	if !ok || len(physical) != 1 || len(indexes) != 2 || distinctTags[0].tag != 1 || distinctTags[1].tag != 8 || indexes[0] != 0 || indexes[1] != 0 {
		t.Fatalf("same Unit distinct tags lost semantic routes: physical:%#v indexes:%#v routes:%#v ok:%t", physical, indexes, distinctTags, ok)
	}
}

func stagedSelectionFixture(t testing.TB) (Selection[uint64, uint64], Access[uint64, ruleUnit], Row, *productSession) {
	t.Helper()
	const epoch = uint64(1)
	rule := &boundRule[uint64, ruleUnit]{rule: &ruleSchema{}}
	execution := &ruleExecution{owner: rule, epoch: epoch}
	execution.active.Store(epoch)
	values := &typedStagedSelectionSession[uint64, uint64, uint64]{values: [][]stagedSelectionValue[uint64, uint64]{
		{{tag: 2, value: 20}},
		{{tag: 3, value: 30}},
	}}
	product := &productSession{
		execution: execution,
		values:    []productRow{{}, {}},
		reads:     []readRuntime{nil},
		sessions:  []readSession{values},
		columns:   [][]uint64{{1, 2}},
		live:      true,
		ready:     true,
		current:   0,
	}
	execution.product = product
	selection, ok := resolveTypedSelection[uint64, uint64, uint64](product, 0, 1)
	if !ok {
		t.Fatal("resolve staged selection")
	}
	return selection, Access[uint64, ruleUnit]{execution: execution, owner: rule, epoch: epoch}, Row{session: product, epoch: epoch, index: 0}, product
}

func TestSelectionIsBoundToItsProvenanceRow(t *testing.T) {
	selection, access, rowA, product := stagedSelectionFixture(t)
	count, ok := SelectionCount(access, rowA, selection)
	if !ok || count != 1 {
		t.Fatalf("row A count = %d/%t", count, ok)
	}
	tag, value, ok := SelectionAt(access, rowA, selection, 0)
	if !ok || tag != 2 || value != 20 {
		t.Fatalf("row A selection = %d/%d/%t", tag, value, ok)
	}

	product.current = 1
	rowB := Row{session: product, epoch: access.epoch, index: 1}
	if _, accepted := SelectionCount(access, rowB, selection); accepted {
		t.Fatal("row A Selection was readable under row B")
	}
	selectionB, resolved := resolveTypedSelection[uint64, uint64, uint64](product, 0, 2)
	if !resolved {
		t.Fatal("resolve row B selection")
	}
	if tag, value, accepted := SelectionAt(access, rowB, selectionB, 0); !accepted || tag != 3 || value != 30 {
		t.Fatalf("row B selection = %d/%d/%t", tag, value, accepted)
	}
}

func TestStagedSelectionScopeRejectsForeignRowAndEscapedAccess(t *testing.T) {
	selection, access, rowA, product := stagedSelectionFixture(t)
	frame := &selectorFrame{
		execution: access.execution,
		epoch:     access.epoch,
		read:      &coldReadSelector{read: 1, depends: []Dependency{{kind: readDependency, index: 0}}},
		product:   product,
		row:       rowA.index,
		current:   -1,
	}
	frame.active.Store(true)
	frame.call.Store(1)
	context := SelectorContext{frame: frame, call: 1}
	if !selection.scopeSelector(product, access.epoch, rowA.index, context.call, 0) {
		t.Fatal("scope row A selection")
	}
	if count, accepted := SelectorSelectionCount(context, selection); !accepted || count != 1 {
		t.Fatalf("scoped row A count = %d/%t", count, accepted)
	}
	frame.row = 1
	if selection.scopeSelector(product, access.epoch, 1, context.call, 0) {
		t.Fatal("scoped row A selection under row B")
	}
	if _, accepted := SelectorSelectionCount(context, selection); accepted {
		t.Fatal("locator read row A selection under row B")
	}

	frame.row = rowA.index
	frame.active.Store(false)
	if _, accepted := SelectorSelectionCount(context, selection); accepted {
		t.Fatal("escaped SelectorContext remained live")
	}
	assertSelectionNoPanic(t, func() {
		if _, accepted := SelectionCount(Access[uint64, ruleUnit]{}, rowA, selection); accepted {
			t.Fatal("zero Access was accepted")
		}
	})
	foreign := &boundRule[uint64, ruleUnit]{rule: access.owner.rule}
	assertSelectionNoPanic(t, func() {
		if _, accepted := SelectionCount(Access[uint64, ruleUnit]{execution: access.execution, owner: foreign, epoch: access.epoch}, rowA, selection); accepted {
			t.Fatal("foreign Access was accepted")
		}
	})
}

func assertSelectionNoPanic(t testing.TB, invoke func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("selection validation panicked: %v", recovered)
		}
	}()
	invoke()
}

// rejectingStagedFactor proves SelectRoute cannot turn a forged Ref into a
// route merely by possessing a live SelectorContext. The actual owner makes
// this decision in stagedUnit; SelectorContext never receives a Unit.
type rejectingStagedFactor struct{}

func (rejectingStagedFactor) stagedUnit(exactRef) (carrier.Unit, bool) { return carrier.Unit{}, false }
func (rejectingStagedFactor) stagedObserve(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[uint64], support.Mask) bool) bool {
	return false
}
func (rejectingStagedFactor) stagedSlot() (shape.Slot, bool) { return 0, false }

func TestSelectRouteRejectsForgedReference(t *testing.T) {
	_, access, row, product := stagedSelectionFixture(t)
	frame := &selectorFrame{execution: access.execution, epoch: access.epoch, read: &coldReadSelector{read: 0}, product: product, row: row.index, current: -1}
	frame.active.Store(true)
	frame.call.Store(1)
	frame.routes = &stagedRouteSink[uint64, uint64]{target: rejectingStagedFactor{}}
	if SelectRoute(SelectorContext{frame: frame, call: 1}, Ref[uint64]{}, uint64(1)) {
		t.Fatal("forged Ref produced a staged route")
	}
}

// TestPublicStagedReadEmptyRoutePreservesProductRow is the assembled law for
// an intentionally empty staged exact read.  A successful locator which
// emits no route is a completed, zero-cardinality Selection -- not a failed
// Product, an omitted row, or a cold candidate fallback.  The Rule's transfer
// consumes that same row, its derivation accepts it, and the observable output
// proves the row crossed the full runtime boundary.
func TestPublicStagedReadEmptyRoutePreservesProductRow(t *testing.T) {
	composition := NewComposition()
	control, controlOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_500)), func(*Factor[uint64, uint64]) bool { return true })
	output, outputOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_501)), func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if !controlOK || control == nil || !outputOK || output == nil || !controlReadOK || !controlWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("empty staged-read factors/forms")
	}

	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(94_502), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_503),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		seedWrite, declared = WriteTo(rule, controlWrite)
		return declared
	})

	locatorCalls, transferCalls, evidenceCalls := 0, 0, 0
	var predecessor Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var projectionWrite Write[uint64]
	var staleDerivation RuleDerivation[uint64, ruleUnit]
	var staleDisposition RuleDisposition[uint64]
	var capturedSelectionEvidence bool
	projection, projectionOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(94_504), Output: output.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(94_505), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			if derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, dispositionOK := derivation.DispositionAt(0)
			if !dispositionOK || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 1 {
				return RuleEvidence{}, false
			}
			count, countOK := DerivationDispositionSelectionCount(derivation, disposition, selection)
			if !countOK || count != 0 {
				return RuleEvidence{}, false
			}
			if _, _, selected := DerivationDispositionSelectionAt(derivation, disposition, selection, 0); selected {
				return RuleEvidence{}, false
			}
			if _, accepted := DerivationDispositionSelectionCount(derivation, RuleDisposition[uint64]{}, selection); accepted {
				return RuleEvidence{}, false
			}
			if _, accepted := DerivationDispositionSelectionCount(derivation, disposition, Read[Selection[uint64, OrderedCells[uint64]]]{}); accepted {
				return RuleEvidence{}, false
			}
			staleDerivation, staleDisposition, capturedSelectionEvidence = derivation, disposition, true
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool {
				cells, cellsOK := ReadValue(access, row, predecessor)
				selected, selectedOK := ReadValue(access, row, selection)
				count, countOK := SelectionCount(access, row, selected)
				return cellsOK && waveCCell(cells) == 2 && selectedOK && countOK && count == 0 && StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var predecessorOK, selectionOK, writeOK bool
		predecessor, predecessorOK = ReadFrom(rule, input, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, controlRead, []Dependency{ReadDependency(predecessor)}, func(context SelectorContext, _ ruleUnit) bool {
			locatorCalls++
			cells, readable := SelectorRead(context, predecessor)
			return readable && waveCCell(cells) == 2
		})
		projectionWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && predecessorOK && selectionOK && writeOK
	})

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_506),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				value, present, cellOK := cells.At(0)
				if !readable || !cellOK || !present {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(94_507)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !seedOK || seed == nil || !projectionOK || projection == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("empty staged-read declarations")
	}
	controlRef, controlIssued := control.Ref(0)
	outputRef, outputIssued := output.Ref(0)
	if !controlIssued || !outputIssued {
		t.Fatal("empty staged-read refs")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(94_508).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	projectionSite, projectionSiteOK := batch.AdmitSite(coldKey(94_509).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurred := batch.Relation(seedSite, coldKey(94_510).compositionKey())
	projectionOccurrence, projectionOccurred := batch.Relation(projectionSite, coldKey(94_511).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(94_512)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, controlRef)
	})
	projectionInstance, projectionInstanceOK := NewRuleInstance(projection, ruleUnitForSemantic(coldKey(94_513)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, predecessor, controlRef) &&
			InstanceSelectorRead(binding, selection, controlRead) &&
			InstanceWrite(binding, projectionWrite, outputRef)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	projectionOperand, projectionOperandOK := admitInstanceOperand(batch, projectionOccurrence, projectionInstance)
	if !scope.Available() || !seedSiteOK || !projectionSiteOK || !seedOccurred || !projectionOccurred || !seedInstanceOK || !projectionInstanceOK || !seedOperandOK || !projectionOperandOK || !batch.Seal() {
		t.Fatal("empty staged-read source batch")
	}
	boundary := equation.BoundaryInput(seedSite, projectionSite, coldKey(94_514).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, projectionPoint := admitPoint(assembly, seedSite), admitPoint(assembly, projectionSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		projectionMember := admitInstance(assembly, projectionPoint, projectionOccurrence, projectionOperand, projectionInstance)
		seedGroup := admitGroup(assembly, seedPoint, seedMember)
		projectionGroup := admitGroup(assembly, projectionPoint, projectionMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputToken, outputRef)
		})
		observation := admitQueryAt(assembly, projectionPoint, queryInstance)
		return seedPoint != nil && projectionPoint != nil && seedMember != nil && projectionMember != nil && seedGroup != nil && projectionGroup != nil &&
			queryInstanceOK && observation != nil && admitBoundary(assembly, projectionGroup, boundary)
	})
	if !compiled || solver == nil {
		t.Fatal("empty staged-read solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 9 {
		t.Fatalf("empty staged-read solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if locatorCalls == 0 || transferCalls != 1 || evidenceCalls != 1 {
		t.Fatalf("empty staged-read callbacks = locator:%d transfer:%d evidence:%d", locatorCalls, transferCalls, evidenceCalls)
	}
	if !capturedSelectionEvidence {
		t.Fatal("empty staged-read evidence was not captured")
	}
	if _, live := DerivationDispositionSelectionCount(staleDerivation, staleDisposition, selection); live {
		t.Fatal("empty staged-read selection evidence remained live after admission")
	}
}

// TestCancellationAfterStagedRouteDropsDynamicObservationBeforePublication
// exercises a real dynamic route rather than a static read disguised as one.
// The first Product reaches its selected exact Unit, stages an output, then is
// cancelled before the admission cut. The test observes no admitted evidence
// or result for that epoch; the executor can therefore never reach its later
// demand-replacement publication cut. A fresh epoch then proves the same exact
// route, where the derivation sees both the static trigger and the selected
// dynamic Unit before it accepts the output.
func TestCancellationAfterStagedRouteDropsDynamicObservationBeforePublication(t *testing.T) {
	composition := NewComposition()
	trigger, triggerOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_520)), func(*Factor[uint64, uint64]) bool { return true })
	dynamic, dynamicOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_521)), func(*Factor[uint64, uint64]) bool { return true })
	output, outputOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_522)), func(*Factor[uint64, uint64]) bool { return true })
	triggerRead, triggerReadOK := ExactReadForm(trigger)
	triggerWrite, triggerWriteOK := ExactWriteForm(trigger)
	dynamicRead, dynamicReadOK := ExactReadForm(dynamic)
	dynamicWrite, dynamicWriteOK := ExactWriteForm(dynamic)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if !triggerOK || trigger == nil || !dynamicOK || dynamic == nil || !outputOK || output == nil ||
		!triggerReadOK || !triggerWriteOK || !dynamicReadOK || !dynamicWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("staged cancellation factors/forms")
	}

	var triggerSeedWrite, dynamicSeedWrite Write[uint64]
	triggerSeed, triggerSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(94_523), Output: trigger.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_524),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		triggerSeedWrite, declared = WriteTo(rule, triggerWrite)
		return declared
	})
	dynamicSeed, dynamicSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(94_525), Output: dynamic.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_526),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		dynamicSeedWrite, declared = WriteTo(rule, dynamicWrite)
		return declared
	})

	var cancel context.CancelFunc
	cancelFirst := true
	locatorCalls, transferCalls, evidenceCalls := 0, 0, 0
	var predecessor Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var projectionWrite Write[uint64]
	var selectedRef Ref[uint64]
	projection, projectionOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(94_527), Output: output.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(94_528), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			// The static trigger and the owner-issued selected Unit are the only
			// demand observations visible to the accepted Product.
			if derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, dispositionOK := derivation.DispositionAt(0)
			selected, selectedOK := DerivationDispositionReadValue(derivation, disposition, selection)
			if !dispositionOK || !selectedOK || disposition.Kind() != RuleDispositionStaged || selected.count == nil {
				return RuleEvidence{}, false
			}
			count, countOK := selected.count(disposition.ordinal)
			if !countOK || count != 1 {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool {
				triggerCells, triggerOK := ReadValue(access, row, predecessor)
				selected, selectedOK := ReadValue(access, row, selection)
				count, countOK := SelectionCount(access, row, selected)
				tag, dynamicCells, valueOK := SelectionAt(access, row, selected, 0)
				if !triggerOK || waveCCell(triggerCells) != 1 || !selectedOK || !countOK || count != 1 || !valueOK || tag != 1 || waveCCell(dynamicCells) != 7 || !StageValue(access, row, uint64(9)) {
					return false
				}
				if cancelFirst {
					cancelFirst = false
					cancel()
				}
				return true
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var predecessorOK, selectionOK, writeOK bool
		predecessor, predecessorOK = ReadFrom(rule, input, triggerRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, dynamicRead, []Dependency{ReadDependency(predecessor)}, func(context SelectorContext, _ ruleUnit) bool {
			locatorCalls++
			cells, readable := SelectorRead(context, predecessor)
			return readable && waveCCell(cells) == 1 && SelectRoute(context, selectedRef, uint64(1))
		})
		projectionWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && predecessorOK && selectionOK && writeOK
	})

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_529),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				value, present, cellOK := cells.At(0)
				if !readable || !cellOK || !present {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(94_530)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !triggerSeedOK || triggerSeed == nil || !dynamicSeedOK || dynamicSeed == nil || !projectionOK || projection == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged cancellation declarations")
	}
	triggerRef, triggerIssued := trigger.Ref(0)
	selectedRef, dynamicIssued := dynamic.Ref(0)
	outputRef, outputIssued := output.Ref(0)
	if !triggerIssued || !dynamicIssued || !outputIssued {
		t.Fatal("staged cancellation refs")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(94_531).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	projectionSite, projectionSiteOK := batch.AdmitSite(coldKey(94_532).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	triggerOccurrence, triggerOccurred := batch.Relation(seedSite, coldKey(94_533).compositionKey())
	dynamicOccurrence, dynamicOccurred := batch.Relation(seedSite, coldKey(94_534).compositionKey())
	projectionOccurrence, projectionOccurred := batch.Relation(projectionSite, coldKey(94_535).compositionKey())
	triggerInstance, triggerInstanceOK := NewRuleInstance(triggerSeed, ruleUnitForSemantic(coldKey(94_536)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, triggerSeedWrite, triggerRef)
	})
	dynamicInstance, dynamicInstanceOK := NewRuleInstance(dynamicSeed, ruleUnitForSemantic(coldKey(94_537)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, dynamicSeedWrite, selectedRef)
	})
	projectionInstance, projectionInstanceOK := NewRuleInstance(projection, ruleUnitForSemantic(coldKey(94_538)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, predecessor, triggerRef) &&
			InstanceSelectorRead(binding, selection, dynamicRead) &&
			InstanceWrite(binding, projectionWrite, outputRef)
	})
	triggerOperand, triggerOperandOK := admitInstanceOperand(batch, triggerOccurrence, triggerInstance)
	dynamicOperand, dynamicOperandOK := admitInstanceOperand(batch, dynamicOccurrence, dynamicInstance)
	projectionOperand, projectionOperandOK := admitInstanceOperand(batch, projectionOccurrence, projectionInstance)
	if !scope.Available() || !seedSiteOK || !projectionSiteOK || !triggerOccurred || !dynamicOccurred || !projectionOccurred ||
		!triggerInstanceOK || !dynamicInstanceOK || !projectionInstanceOK || !triggerOperandOK || !dynamicOperandOK || !projectionOperandOK || !batch.Seal() {
		t.Fatal("staged cancellation source batch")
	}
	boundary := equation.BoundaryInput(seedSite, projectionSite, coldKey(94_539).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, projectionPoint := admitPoint(assembly, seedSite), admitPoint(assembly, projectionSite)
		triggerMember := admitInstance(assembly, seedPoint, triggerOccurrence, triggerOperand, triggerInstance)
		dynamicMember := admitInstance(assembly, seedPoint, dynamicOccurrence, dynamicOperand, dynamicInstance)
		projectionMember := admitInstance(assembly, projectionPoint, projectionOccurrence, projectionOperand, projectionInstance)
		triggerGroup := admitGroup(assembly, seedPoint, triggerMember)
		dynamicGroup := admitGroup(assembly, seedPoint, dynamicMember)
		projectionGroup := admitGroup(assembly, projectionPoint, projectionMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputToken, outputRef)
		})
		observation := admitQueryAt(assembly, projectionPoint, queryInstance)
		return seedPoint != nil && projectionPoint != nil && triggerMember != nil && dynamicMember != nil && projectionMember != nil &&
			triggerGroup != nil && dynamicGroup != nil && projectionGroup != nil && queryInstanceOK && observation != nil && admitBoundary(assembly, projectionGroup, boundary)
	})
	if !compiled || solver == nil {
		t.Fatal("staged cancellation solver")
	}
	ctx, cancelContext := context.WithCancel(context.Background())
	cancel = cancelContext
	state, status := solver.Solve(ctx)
	cancelContext()
	if state != nil || status != SolveCanceled || transferCalls != 1 || evidenceCalls != 0 {
		t.Fatalf("cancelled staged route = state:%v status:%v transfer:%d evidence:%d", state, status, transferCalls, evidenceCalls)
	}
	state, status = solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if state == nil || status != SolveComplete || !receiptOK || !readable || result != 9 {
		t.Fatalf("fresh staged route = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if locatorCalls < 2 || transferCalls != 2 || evidenceCalls != 1 {
		t.Fatalf("staged cancellation callbacks = locator:%d transfer:%d evidence:%d", locatorCalls, transferCalls, evidenceCalls)
	}
}
