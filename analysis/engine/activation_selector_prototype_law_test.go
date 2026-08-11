package engine

import (
	"context"
	"testing"
)

// TestActivationPrototypeExecutesImportedReadThenStagedSelector proves the
// activation path uses the ordinary RuleInstance staged-read runtime. The
// prototype imports one exact predecessor through its fixed port ABI, then a
// Rule-owned SelectRead routes to an owner-issued Ref without adding that Ref
// (or a second evaluator) to the activation plan.
func TestActivationPrototypeExecutesImportedReadThenStagedSelector(t *testing.T) {
	composition := NewComposition()
	control := coldFactor(composition, coldKey(982_001))
	target := coldFactor(composition, coldKey(982_002))
	outputSpec := coldFactorSpec(coldKey(982_003))
	outputSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	output, outputOK := DeclareFactor(composition, outputSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	targetRead, targetReadOK := ExactReadForm(target)
	outputRead, outputReadOK := ExactReadForm(output)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	targetWrite, targetWriteOK := ExactWriteForm(target)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if control == nil || target == nil || !outputOK || output == nil || !controlReadOK || !targetReadOK || !outputReadOK || !controlWriteOK || !targetWriteOK || !outputWriteOK {
		t.Fatal("activation selector factors/forms")
	}

	var controlSeedWrite Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(982_004), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](982_104),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		controlSeedWrite, ok = WriteTo(rule, controlWrite)
		return ok
	})
	var targetSeedWrite Write[uint64]
	targetSeed, targetSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(982_005), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: target.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](982_105),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(5)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		targetSeedWrite, ok = WriteTo(rule, targetWrite)
		return ok
	})

	var imported Read[OrderedCells[uint64]]
	var selected Read[Selection[uint64, OrderedCells[uint64]]]
	var resultWrite Write[uint64]
	var selectedRef Ref[uint64]
	locatorCalls, transferCalls := 0, 0
	locatorControl, locatorReadable, locatorRouted := uint64(0), false, false
	transferSettled := false
	prototype, prototypeOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(982_006), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: output.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](982_106),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool {
				controlCells, controlOK := ReadValue(access, row, imported)
				selection, selectionOK := ReadValue(access, row, selected)
				count, countOK := SelectionCount(access, row, selection)
				if !controlOK || !selectionOK || !countOK {
					return false
				}
				if count == 0 {
					// Zero routes is a completed staged observation. This Rule owns
					// an ordinary exact output, so it settles NoCandidate; NoSelection
					// is reserved for a selector-routed output surface.
					transferSettled = waveCCell(controlCells) == 0 && NoCandidate(access, row)
					return transferSettled
				}
				tag, targetCells, targetOK := SelectionAt(access, row, selection, 0)
				return count == 1 && waveCCell(controlCells) == 2 && targetOK && tag == 7 && waveCCell(targetCells) == 5 && StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var importedOK, selectedOK, writeOK bool
		imported, importedOK = ReadFrom(rule, input, controlRead)
		selected, selectedOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, targetRead, []Dependency{ReadDependency(imported)}, func(context SelectorContext, _ ruleUnit) bool {
			locatorCalls++
			cells, readable := SelectorRead(context, imported)
			locatorControl, locatorReadable = waveCCell(cells), readable
			locatorRouted = locatorControl == 2 && SelectRoute(context, selectedRef, uint64(7))
			return readable && (locatorControl != 2 || locatorRouted)
		})
		resultWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && importedOK && selectedOK && writeOK
	})

	family, familyOK := DeclareActivationFamily(composition, coldKey(982_007))
	application, selectedTarget, endpoint := coldKey(982_008), coldKey(982_009), coldKey(982_010)
	activationRuns := 0
	trigger, triggerOK := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(982_011), Family: family, Inputs: 0, Admission: AdmitActivationByTrustedTheorem(coldKey(982_111)),
		Run: func(value Activation) bool {
			activationRuns++
			selectedApplication, ok := ActivationApplication(value)
			return ok && selectedApplication == application && Activate(value, selectedApplication, selectedTarget, endpoint)
		},
	})

	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(982_012),
		Project: func(observation Observation) uint64 {
			result := uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				if !readable {
					return false
				}
				result = waveCCell(cells)
				return true
			}) {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(982_112)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(query, outputRead)
		return ok
	})
	if !controlSeedOK || controlSeed == nil || !targetSeedOK || targetSeed == nil || !prototypeOK || prototype == nil ||
		!familyOK || !triggerOK || trigger == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("activation selector declarations")
	}

	controlRef, controlRefOK := control.Ref(0)
	var selectedRefOK bool
	selectedRef, selectedRefOK = target.Ref(0)
	outputRef, outputRefOK := output.Ref(0)
	if !controlRefOK || !selectedRefOK || !outputRefOK {
		t.Fatal("activation selector refs")
	}
	role, slot := coldKey(982_013), coldKey(982_014)
	prototypeInstance, prototypeInstanceOK := NewActivationPrototypeInstance(prototype, ruleUnitForSemantic(coldKey(982_015)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstancePortRead(binding, imported, role, slot) &&
			InstanceSelectorRead(binding, selected, targetRead) &&
			InstanceWrite(binding, resultWrite, outputRef)
	})
	controlSeedInstance, controlSeedInstanceOK := NewRuleInstance(controlSeed, ruleUnitForSemantic(coldKey(982_016)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlSeedWrite, controlRef)
	})
	targetSeedInstance, targetSeedInstanceOK := NewRuleInstance(targetSeed, ruleUnitForSemantic(coldKey(982_017)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, targetSeedWrite, selectedRef)
	})
	prototypeSource := coldKey(982_020)
	prototypeAdmission, prototypeAdmissionOK := ActivationPrototypeAdmissionFor(prototypeSource, prototypeInstance)
	if !prototypeInstanceOK || prototypeInstance == nil || !prototypeAdmissionOK || !controlSeedInstanceOK || controlSeedInstance == nil || !targetSeedInstanceOK || targetSeedInstance == nil {
		t.Fatal("activation selector instances")
	}

	build := NewSourceAssembly(composition)
	batch := build.state.batch
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	portSite, portSiteOK := build.Site(coldKey(982_018), scope, truth, true)
	triggerSite, triggerSiteOK := build.Site(coldKey(982_019), scope, truth, true)
	triggerOccurrence, triggerOccurrenceOK := build.At(triggerSite)
	triggerOperand, triggerOperandOK := build.Operand(triggerOccurrence, coldKey(982_021))
	controlSeedOccurrence, controlSeedOccurrenceOK := build.Relation(portSite, coldKey(982_022))
	targetSeedOccurrence, targetSeedOccurrenceOK := build.Relation(portSite, coldKey(982_023))
	controlSeedPrepared, controlSeedOperandOK := build.PrepareInstance(controlSeedOccurrence, controlSeedInstance)
	targetSeedPrepared, targetSeedOperandOK := build.PrepareInstance(targetSeedOccurrence, targetSeedInstance)
	prepared, staged := StageActivationPlan(build, composition, family, []ActivationPlanEntry{{
		Target: selectedTarget, Endpoint: endpoint, PortRole: role, Provenance: coldKey(982_024), Prototype: prototypeAdmission,
	}})
	if !scopeOK || !truthOK || !portSiteOK || !triggerSiteOK || !triggerOccurrenceOK || !triggerOperandOK ||
		!controlSeedOccurrenceOK || !targetSeedOccurrenceOK || !controlSeedOperandOK || !targetSeedOperandOK || !staged || !build.Seal() {
		t.Fatal("activation selector source build")
	}
	plan, planOK := FinalizeActivationPlan(build, prepared)
	if !planOK || plan == nil {
		t.Fatal("activation selector plan")
	}

	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		assembly.sourceAssembly = build
		portPoint := admitPoint(assembly, portSite.value)
		triggerPoint := admitPoint(assembly, triggerSite.value)
		base, baseOK := ActivationBaseAt(assembly, portPoint)
		port, portOK := NewActivationPort(role, base)
		portReadOK := portOK && AddActivationPortRead(port, slot, control, controlRef)
		triggerInstance, triggerInstanceOK := NewActivationTrigger(trigger, application, plan, []*ActivationPort{port}, func(*StructuralBinding) bool { return true })
		if portPoint == nil || triggerPoint == nil || !baseOK || !portReadOK || !triggerInstanceOK || triggerInstance == nil {
			return false
		}
		controlMember := admitInstance(assembly, portPoint, controlSeedPrepared.occurrence.value, controlSeedPrepared.operand.value, controlSeedInstance)
		targetMember := admitInstance(assembly, portPoint, targetSeedPrepared.occurrence.value, targetSeedPrepared.operand.value, targetSeedInstance)
		activationMember := admitStructural(assembly, triggerPoint, triggerOccurrence.value, triggerOperand.value, triggerInstance)
		if controlMember == nil || targetMember == nil || activationMember == nil || admitGroup(assembly, portPoint, controlMember, targetMember) == nil || admitGroup(assembly, triggerPoint, activationMember) == nil {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, outputRef)
		})
		return queryInstanceOK && admitQueryAt(assembly, portPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("activation selector assembly")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK || !receipt.Available() {
		t.Fatal("activation selector receipt before revision")
	}
	state, status := solver.Solve(context.Background())
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receipt.Available() || !readable || result != 9 || activationRuns == 0 || locatorCalls == 0 || transferCalls == 0 {
		t.Fatalf("activation selector solve = status:%v result:%d/%t activation:%d locator:%d control:%d/%t routed:%t transfer:%d settled:%t", status, result, readable, activationRuns, locatorCalls, locatorControl, locatorReadable, locatorRouted, transferCalls, transferSettled)
	}
}
