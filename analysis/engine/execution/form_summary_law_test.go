package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// summaryFixture is one bound Factor carrying a correlated and a distributive
// summary over the same two coordinates, with only the first coordinate
// written. The second coordinate is therefore a declared cell with no stored
// value in every partition row, which is the sparse case the S form must
// preserve rather than compact away.
type summaryFixture struct {
	binding      *factbinding.Binding[uint64, uint64]
	correlated   carrier.Unit
	distributive carrier.Unit
	target       carrier.Target
	state        carrier.State
	whole        support.Mask
	work         *carrier.Work
	run          *Run
	serial       uint64
}

func newSummaryFixture(t testing.TB) *summaryFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	whole := regions.True()
	onOne, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("region literal")
	}
	algebra, ok := factbinding.Admit[uint64, uint64](2, 0, lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return 0 },
		Equal:    func(left, right uint64) bool { return left == right },
		Same:     func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}, func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("algebra")
	}
	var correlated, distributive carrier.Unit
	var target carrier.Target
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		first, declared := binding.DeclareExact(0)
		if !declared {
			return false
		}
		if _, declared = binding.DeclareExact(1); !declared {
			return false
		}
		if correlated, declared = binding.DeclareSummary([]uint64{0, 1}); !declared {
			return false
		}
		if distributive, declared = binding.DeclareDistributiveSummary([]uint64{0, 1}); !declared {
			return false
		}
		target, declared = binding.DeclareStrong(first)
		return declared
	})
	if !ok {
		t.Fatal("binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(target, onOne, 9) {
		t.Fatal("patch")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	next, _, ok := work.Commit(state, []carrier.Patch{candidate})
	if !ok {
		t.Fatal("commit")
	}
	return &summaryFixture{
		binding: binding, correlated: correlated, distributive: distributive,
		target: target, state: next, whole: whole, work: work,
		run: NewRun(1, 0), serial: 500,
	}
}

func (fixture *summaryFixture) issue(t testing.TB) Ticket {
	t.Helper()
	fixture.serial++
	ticket, ok := issueExecutionRow(fixture.run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 0, 1, 17, fixture.serial)
	if !ok {
		t.Fatal("issue")
	}
	return ticket
}

func summaryContract(form ruleprogram.ReadForm, multiplicity ruleprogram.Multiplicity) SummaryContract {
	return SummaryContract{
		Form: form,
		Contract: ruleplan.ReadContract{
			Order:        ruleprogram.OrderCanonical,
			Sparse:       ruleprogram.SparseDense,
			OnOpaque:     ruleprogram.OnOpaqueRefuse,
			Multiplicity: multiplicity,
		},
		Denominator: ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
	}
}

// TestSummaryRowRequiresItsSealedReadContract states the S form's mandatory
// contract: a summary or complete row cannot be sealed without the order,
// absence policy, opaque policy, width, and closed denominator its vector is
// read under. Every incomplete contract is refused at seal, so no delivery
// ever happens under an invented default.
func TestSummaryRowRequiresItsSealedReadContract(t *testing.T) {
	fixture := newSummaryFixture(t)
	complete := summaryContract(ruleprogram.Summary, ruleprogram.MultiplicityMany)
	if _, ok := NewSummaryRow(fixture.binding, fixture.correlated, 0, fixture.target, 0, complete); !ok {
		t.Fatal("complete summary contract was refused")
	}
	missingDenominator := complete
	missingDenominator.Denominator = ruleplan.DenominatorAddr{}
	exactForm := complete
	exactForm.Form = ruleprogram.Exact
	noOrder := complete
	noOrder.Contract.Order = ruleprogram.OrderInvalid
	noSparse := complete
	noSparse.Contract.Sparse = ruleprogram.SparseInvalid
	noOpaque := complete
	noOpaque.Contract.OnOpaque = ruleprogram.OnOpaqueInvalid
	noWidth := complete
	noWidth.Contract.Multiplicity = ruleprogram.MultiplicityInvalid
	for name, contract := range map[string]SummaryContract{
		"missing-denominator": missingDenominator,
		"exact-form":          exactForm,
		"no-order":            noOrder,
		"no-sparse":           noSparse,
		"no-opaque":           noOpaque,
		"no-width":            noWidth,
		"zero":                {},
	} {
		if contract.Available() {
			t.Fatalf("%s contract reports available", name)
		}
		if _, ok := NewSummaryRow(fixture.binding, fixture.correlated, 0, fixture.target, 0, contract); ok {
			t.Fatalf("%s contract sealed a summary row", name)
		}
	}
	closed := summaryContract(ruleprogram.Complete, ruleprogram.MultiplicityMany)
	if !closed.Closed() || complete.Closed() {
		t.Fatalf("closed disposition = complete %t / summary %t", closed.Closed(), complete.Closed())
	}
}

// TestSummaryVectorPreservesCellOrderAndAbsence states the delivery law: every
// partition row carries the whole declared cell vector in sealed coordinate
// order, and a coordinate with no stored value stays a cell of that vector
// reporting absence. Compacting an absent cell would renumber every later
// cell and break the correlation between a position and the coordinate the
// owner declared at it.
func TestSummaryVectorPreservesCellOrderAndAbsence(t *testing.T) {
	fixture := newSummaryFixture(t)
	row, ok := NewSummaryRow(fixture.binding, fixture.correlated, 0, fixture.target, 0, summaryContract(ruleprogram.Summary, ruleprogram.MultiplicityMany))
	if !ok || !row.Valid() {
		t.Fatal("summary row")
	}
	ticket := fixture.issue(t)
	var scratch Scratch[uint64, uint64]
	rows, storedFirstCell := 0, 0
	for {
		vector, status := row.Deliver(ticket, &scratch)
		if status == ReadExhausted {
			break
		}
		if status != ReadAvailable || !vector.Valid() {
			t.Fatalf("delivery status = %d valid %t", status, vector.Valid())
		}
		if vector.Count() != 2 {
			t.Fatalf("vector width = %d, want the two declared coordinates", vector.Count())
		}
		value, present, available := vector.At(0)
		if !available {
			t.Fatal("declared cell 0 is not a cell")
		}
		if present {
			if value != 9 {
				t.Fatalf("cell 0 value = %d, want the written 9", value)
			}
			storedFirstCell++
		}
		if _, present, available = vector.At(1); !available {
			t.Fatal("declared cell 1 is not a cell")
		} else if present {
			t.Fatal("cell 1 was never written yet reports a stored value")
		}
		if _, _, available = vector.At(2); available {
			t.Fatal("an index past the declared width is a cell")
		}
		if _, _, available = vector.At(-1); available {
			t.Fatal("a negative index is a cell")
		}
		if region, regionOK := row.Region(&scratch); !regionOK || !region.Valid() {
			t.Fatal("delivered row carries no authenticated region")
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("correlated summary delivered no partition row")
	}
	if storedFirstCell == 0 {
		t.Fatal("no partition row observed the written coordinate")
	}
	if !row.Close(ticket, &scratch) || !ticket.Close() {
		t.Fatal("summary lifecycle")
	}
}

// TestSummaryDeliveryDistinguishesCorrelatedFromDistributive states that the
// two declared summary folds are distinct deliveries over the same
// coordinates: the correlated reader receives the exact joint partition, and
// the distributive reader receives the coordinate-wise one. The S form reads
// the fold the owner sealed and never re-partitions.
func TestSummaryDeliveryDistinguishesCorrelatedFromDistributive(t *testing.T) {
	fixture := newSummaryFixture(t)
	contract := summaryContract(ruleprogram.Summary, ruleprogram.MultiplicityMany)
	count := func(unit carrier.Unit) int {
		row, ok := NewSummaryRow(fixture.binding, unit, 0, fixture.target, 0, contract)
		if !ok {
			t.Fatal("summary row")
		}
		ticket := fixture.issue(t)
		var scratch Scratch[uint64, uint64]
		rows := 0
		for {
			vector, status := row.Deliver(ticket, &scratch)
			if status == ReadExhausted {
				break
			}
			if status != ReadAvailable || vector.Count() != 2 {
				t.Fatalf("delivery status = %d width %d", status, vector.Count())
			}
			rows++
		}
		if !row.Close(ticket, &scratch) || !ticket.Close() {
			t.Fatal("lifecycle")
		}
		return rows
	}
	correlated := count(fixture.correlated)
	distributive := count(fixture.distributive)
	if correlated <= distributive {
		t.Fatalf("correlated partition rows = %d, distributive = %d: the joint partition is not finer", correlated, distributive)
	}
}

// TestSummaryDeliveryAllocatesNothingWhenWarm states the S form's cost law: a
// warm delivery reuses the caller's Scratch and the Binding's observation
// storage, so reading a whole cell vector allocates nothing per row and
// nothing per cell.
func TestSummaryDeliveryAllocatesNothingWhenWarm(t *testing.T) {
	fixture := newSummaryFixture(t)
	row, ok := NewSummaryRow(fixture.binding, fixture.distributive, 0, fixture.target, 0, summaryContract(ruleprogram.Complete, ruleprogram.MultiplicityMany))
	if !ok {
		t.Fatal("summary row")
	}
	var scratch Scratch[uint64, uint64]
	deliver := func() bool {
		fixture.serial++
		ticket, issued := issueExecutionRow(fixture.run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 0, 1, 17, fixture.serial)
		if !issued {
			return false
		}
		for {
			vector, status := row.Deliver(ticket, &scratch)
			if status == ReadExhausted {
				break
			}
			if status != ReadAvailable {
				return false
			}
			for index := 0; index < vector.Count(); index++ {
				if _, _, available := vector.At(index); !available {
					return false
				}
			}
		}
		return row.Close(ticket, &scratch) && ticket.Close()
	}
	if !deliver() || !deliver() {
		t.Fatal("warmup")
	}
	if allocations := testing.AllocsPerRun(20, func() {
		if !deliver() {
			t.Fatal("warm delivery")
		}
	}); allocations != 0 {
		t.Fatalf("warm summary delivery allocated %v times", allocations)
	}
}

// TestSummaryFormIsDerivedFromItsSealedRead states the derivation law: a
// descriptor with a Summary or Complete read is the S form, because a
// whole-vector read is what that form is. The derivation reads the sealed read
// vocabulary, so a rule that declares a vector read can never be routed to a
// form that delivers one cell.
func TestSummaryFormIsDerivedFromItsSealedRead(t *testing.T) {
	for _, testCase := range []struct {
		name string
		rule generated.CompiledRule
	}{
		{name: "summary", rule: planCompiledSummaryRule(t, ruleprogram.Summary)},
		{name: "complete", rule: planCompiledSummaryRule(t, ruleprogram.Complete)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			row, ok := DeclaredForm(testCase.rule)
			if !ok || row.Form != FormSummary {
				t.Fatalf("derived as %q/%t, want summary", row.Form.Name(), ok)
			}
			if row.Input != 0 {
				t.Fatalf("derived read port = %d", row.Input)
			}
		})
	}
	// The two ways an S read can be malformed are refused one layer below the
	// derivation, at the descriptor seal: a complete vector selects nothing, so
	// it carries no predicate, and both S reads are vectors of a closed
	// denominator, so neither seals without one.
	if _, sealed := summaryPlanSpec(ruleprogram.Complete, true, ruleplan.DenominatorAddr{Ordinal: 0, Present: true}); sealed {
		t.Fatal("a complete read carrying a selection predicate sealed")
	}
	if _, sealed := summaryPlanSpec(ruleprogram.Summary, true, ruleplan.DenominatorAddr{}); sealed {
		t.Fatal("a summary read with no sealed denominator sealed")
	}
	if _, sealed := summaryPlanSpec(ruleprogram.Complete, false, ruleplan.DenominatorAddr{}); sealed {
		t.Fatal("a complete read with no sealed denominator sealed")
	}
}

// TestSummaryFormRefusesByNameWithoutAnImplementation states what an S row
// does today: the form is declared and named, and the typed implementation
// column does not cover it, so a build refuses and names "summary". A plan row
// is never silently dropped from the ladder or folded into the exact form.
func TestSummaryFormRefusesByNameWithoutAnImplementation(t *testing.T) {
	fixture := newExecutionFixture(t)
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, nil, nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	rows := []FormRow{{Member: 0, Form: FormSummary, Input: 0, Unit: fixture.unit, Target: fixture.target}}
	families, addresses, refused, built := BuildForms(plane, rows)
	if built || families != nil || addresses != nil {
		t.Fatalf("summary form built %d families / %d addresses", len(families), len(addresses))
	}
	if refused != FormSummary || refused.Name() != "summary" {
		t.Fatalf("refusal names form %d/%q, want summary", refused, refused.Name())
	}
}

func planCompiledSummaryRule(t *testing.T, form ruleprogram.ReadForm) generated.CompiledRule {
	t.Helper()
	rule, sealed := summaryPlanSpec(form, form == ruleprogram.Summary, ruleplan.DenominatorAddr{Ordinal: 0, Present: true})
	if !sealed {
		t.Fatalf("sealed %d plan", form)
	}
	return rule
}

func summaryPlanSpec(form ruleprogram.ReadForm, predicate bool, denominator ruleplan.DenominatorAddr) (generated.CompiledRule, bool) {
	var selection ruleplan.ProjectionAddr
	if predicate {
		selection = ruleplan.ProjectionAddr{Axis: 0, Member: 1}
	}
	return generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{{
			Input: 0, Factor: 1, Axis: 0,
			Relation:         ruleplan.RelationAddr{Axis: 0, Member: 0},
			Key:              ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Predicate:        selection,
			PredicatePresent: predicate,
			// Whether the rule's ordinal indexes this read is a property of
			// the form, so the fixture asks the same statement the seal does
			// rather than pinning one form's answer.
			Addressing:        ruleplan.RelationAddr{Axis: 0, Member: 0},
			AddressingPresent: ruleprogram.ReadFormCandidateAddressed(form),
			Form:              form,
			PointBound:        ruleprogram.PointBound,
			Contract:          ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseDense, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityMany},
			Denominator:       denominator,
			RowCapacity:       4, CellCapacity: 4,
		}},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
	})
}
