package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// TestExternalSourceAssemblyRecurrentExternalIngressCompletes exercises the
// public source cut with an InitAbsent recurrence head.  A zero-input semantic
// seed feeds a one-input ingress/dispatch group at the head, while a separate
// one-input group carries the head's own factor from the recurrent
// predecessor.  The ingress must be present in the first exact RHS and the
// self edge must recur through the same head without a second source or solver
// transaction.
func TestExternalSourceAssemblyRecurrentExternalIngressCompletes(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(31), KeyEnd: 1, Lattice: facadeLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	carry, carryOK := engine.Carry(factor)
	var ingressWrite engine.Write[uint64]
	ingress, ingressOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(32), OperandFamily: facadeKey(33), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(34)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 1) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		ingressWrite, ok = engine.WriteTo(rule, write)
		return ok
	})
	dispatch, dispatchOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(35), OperandFamily: facadeKey(36), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: 1, Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(37)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(engine.Row) bool { return true })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && engine.CarryFrom(rule, input, carry)
	})
	self, selfOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(38), OperandFamily: facadeKey(39), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: 1, Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(40)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(engine.Row) bool { return true })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && engine.CarryFrom(rule, input, carry)
	})
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(41),
		Project: func(observation engine.Observation) uint64 {
			var value uint64
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, ok := engine.QueryValue(row, queryRead)
				if !ok || cells.Count() != 1 {
					return false
				}
				entry, present, valid := cells.At(0)
				if !valid || !present {
					return false
				}
				value = entry
				return true
			}) {
				return 0
			}
			return value
		},
		Result: engine.FrozenResult[uint64]{
			Semantic: facadeKey(42), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, read)
		return ok
	})
	if factor == nil || !factorOK || !readOK || !writeOK || !carryOK || ingress == nil || !ingressOK || dispatch == nil || !dispatchOK || self == nil || !selfOK || query == nil || !queryOK || !composition.Seal() {
		t.Fatal("recurrent external-ingress cold declaration")
	}
	ingressRef, ingressRefOK := factor.Ref(0)
	ingressInstance, ingressInstanceOK := engine.NewRuleInstance(ingress, facadeUnitFor(facadeKey(40)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, ingressWrite, ingressRef)
	})
	dispatchInstance, dispatchInstanceOK := engine.NewRuleInstance(dispatch, facadeUnitFor(facadeKey(43)), func(*engine.RuleBinding[uint64, facadeUnit]) bool { return true })
	selfInstance, selfInstanceOK := engine.NewRuleInstance(self, facadeUnitFor(facadeKey(44)), func(*engine.RuleBinding[uint64, facadeUnit]) bool { return true })
	if !ingressRefOK || !ingressInstanceOK || ingressInstance == nil || !dispatchInstanceOK || dispatchInstance == nil || !selfInstanceOK || selfInstance == nil {
		t.Fatal("recurrent external-ingress instances")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	ingressSite, ingressSiteOK := source.Site(facadeKey(42), scope, truth, true)
	headSite, headSiteOK := source.Site(facadeKey(43), scope, falsity, false)
	ingressOccurrence, ingressOccurrenceOK := source.At(ingressSite)
	headOccurrence, headOccurrenceOK := source.At(headSite)
	ingressPrepared, ingressPreparedOK := source.PrepareInstance(ingressOccurrence, ingressInstance)
	dispatchPrepared, dispatchPreparedOK := source.PrepareInstance(headOccurrence, dispatchInstance)
	selfPrepared, selfPreparedOK := source.PrepareInstance(headOccurrence, selfInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	ingressBoundary, ingressBoundaryOK := source.Boundary(ingressSite, headSite, facadeKey(44), truth, reindex, truth)
	selfBoundary, selfBoundaryOK := source.Boundary(headSite, headSite, facadeKey(45), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !ingressSiteOK || !headSiteOK || !ingressOccurrenceOK || !headOccurrenceOK || !ingressPreparedOK || !dispatchPreparedOK || !selfPreparedOK || !reindexOK || !ingressBoundaryOK || !selfBoundaryOK {
		t.Fatal("recurrent external-ingress source admission")
	}
	if count, ok := source.InputCount(ingressPrepared); !ok || count != 0 {
		t.Fatalf("ingress input arity count=%d ok=%t", count, ok)
	}
	if count, ok := source.InputCount(dispatchPrepared); !ok || count != 1 {
		t.Fatalf("dispatch input arity count=%d ok=%t", count, ok)
	}
	if count, ok := source.InputCount(selfPrepared); !ok || count != 1 {
		t.Fatalf("self input arity count=%d ok=%t", count, ok)
	}
	if !source.Seal() {
		t.Fatal("recurrent external-ingress source seal")
	}
	var queryInstance *engine.QueryInstance[uint64]
	solver, assembled := source.Assemble(func(value *engine.Assembly) bool {
		ingressPoint, ingressPointOK := value.Point(ingressSite)
		headPoint, headPointOK := value.Point(headSite)
		ingressMember, ingressMemberOK := value.Member(ingressPoint, ingressPrepared)
		dispatchMember, dispatchMemberOK := value.Member(headPoint, dispatchPrepared)
		selfMember, selfMemberOK := value.Member(headPoint, selfPrepared)
		_, ingressGroupOK := value.Group(ingressPoint, ingressMember)
		dispatchGroup, dispatchGroupOK := value.Group(headPoint, dispatchMember)
		selfGroup, selfGroupOK := value.Group(headPoint, selfMember)
		if !ingressPointOK || !headPointOK || !ingressMemberOK || !dispatchMemberOK || !selfMemberOK || !ingressGroupOK || !dispatchGroupOK || !selfGroupOK {
			return false
		}
		if !value.Boundary(dispatchGroup, ingressBoundary) || !value.Boundary(selfGroup, selfBoundary) {
			return false
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, queryRead, ingressRef)
		})
		_, queryAttached := value.Query(headPoint, queryInstance)
		return queryInstanceOK && queryAttached
	})
	if !assembled || solver == nil {
		t.Fatalf("recurrent external-ingress assembly assembled=%t solver=%p", assembled, solver)
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("recurrent external-ingress solve state=%v status=%v", state, status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	value, readable := engine.QueryResult(receipt, state)
	if !receiptOK || !readable || value != 1 {
		t.Fatalf("recurrent external-ingress result=%d readable=%t receipt=%t", value, readable, receiptOK)
	}
}
