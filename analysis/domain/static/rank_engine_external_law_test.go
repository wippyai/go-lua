package static_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestStaticRankDescendsThroughEngineRecurrence deliberately reaches the
// production Factor/factbinding path. It proves Bottom -> closed -> derived
// union -> Top is accepted as a strict descending widening sequence, rather
// than only checking the Static lattice in isolation.
func TestStaticRankDescendsThroughEngineRecurrence(t *testing.T) {
	authority := externalRankAuthority(t)
	left, right := externalRecordValues(t, authority)
	joined := authority.Join(left, right)
	if authority.Equal(joined, left) || authority.Equal(joined, right) || authority.Equal(joined, authority.Top()) {
		t.Fatal("Static recurrence lacks strict derived middle value")
	}
	var bottomTransition, firstTransition, secondTransition bool
	result := runExternalRankRecurrence(t, authority, func(value staticdomain.Value) staticdomain.Value {
		switch {
		case authority.Equal(value, authority.Bottom()):
			bottomTransition = true
			return left
		case authority.Equal(value, left):
			firstTransition = true
			return joined
		case authority.Equal(value, joined):
			secondTransition = true
			return authority.Top()
		default:
			return value
		}
	})
	if !authority.Equal(result, authority.Top()) || !bottomTransition || !firstTransition || !secondTransition {
		t.Fatalf("Static rank recurrence transitions:%t/%t/%t", bottomTransition, firstTransition, secondTransition)
	}
}

// This is the hostile one-cell case for the two non-equivalence traps in the
// Static order: an empty table shape widens to table-top, and authored Any is
// a noncanonical universal spelling whose contribution must select Unknown.
func TestStaticExtensionalDynamicRankDescendsThroughOneCoordinate(t *testing.T) {
	authority := externalRankAuthority(t)
	empty, tableTop, dynamic, unknown := externalDynamicValues(t, authority)
	if !authority.LessOrEq(empty, tableTop) || authority.Equal(empty, tableTop) {
		t.Fatal("empty-to-tableTop strict coverage edge unavailable")
	}
	if !authority.LessOrEq(dynamic, unknown) || authority.LessOrEq(unknown, dynamic) {
		t.Fatal("directed Any-to-Unknown exact edge unavailable")
	}
	if widened := authority.Join(tableTop, dynamic); !authority.Equal(widened, unknown) {
		t.Fatal("tableTop joined with authored Any did not select Unknown")
	}
	var sawEmpty, sawTableTop, sawAnyContribution, sawUnknown bool
	result := runExternalRankRecurrence(t, authority, func(value staticdomain.Value) staticdomain.Value {
		switch {
		case authority.Equal(value, authority.Bottom()):
			sawEmpty = true
			return empty
		case authority.Equal(value, empty):
			sawTableTop = true
			return tableTop
		case authority.Equal(value, tableTop):
			sawAnyContribution = true
			next := authority.Join(value, dynamic)
			if authority.Equal(next, unknown) {
				sawUnknown = true
			}
			return next
		default:
			return value
		}
	})
	if !authority.Equal(result, unknown) || !sawEmpty || !sawTableTop || !sawAnyContribution || !sawUnknown {
		t.Fatalf("dynamic recurrence transitions:%t/%t/%t/%t", sawEmpty, sawTableTop, sawAnyContribution, sawUnknown)
	}
}

func runExternalRankRecurrence(t *testing.T, authority *staticdomain.Authority, advance func(staticdomain.Value) staticdomain.Value) staticdomain.Value {
	t.Helper()
	composition := engine.NewComposition()
	keyEnd := uint64(authority.CoordinateCount())
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint32, staticdomain.Value]{
		Semantic: rankSemantic(1), KeyEnd: keyEnd, Lattice: authority.Lattice(), Default: authority.Bottom(),
		AdmitAt:     func(key uint32, value staticdomain.Value) bool { return uint64(key) < keyEnd && authority.Owns(value) },
		Fingerprint: authority.Fingerprint,
		WidenRank: engine.Measure[uint32, staticdomain.Value]{Width: 1, At: func(_ uint32, value staticdomain.Value, component int) uint64 {
			if component != 0 {
				return 0
			}
			return authority.WidenRank(value)
		}},
	}, func(*engine.Factor[uint32, staticdomain.Value]) bool { return true })
	if !factorOK || factor == nil {
		t.Fatal("Static rank Factor")
	}
	readForm, readOK := engine.ExactReadForm(factor)
	writeForm, writeOK := engine.ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("Static rank Factor forms")
	}
	unit := rankUnitFor(rankSemantic(2))
	var recurrenceRead engine.Read[engine.OrderedCells[staticdomain.Value]]
	var recurrenceWrite engine.Write[staticdomain.Value]
	recurrenceRule, recurrenceOK := engine.DeclareRule(composition, engine.RuleSpec[staticdomain.Value, rankUnit]{
		Semantic: rankSemantic(3), OperandFamily: rankSemantic(4), OperandContent: rankUnitContent,
		Output: factor.Output(), Inputs: 1, Admission: engine.AdmitRuleByTrustedTheorem[staticdomain.Value, rankUnit](rankSemantic(5)),
		Transfer: func(access engine.Access[staticdomain.Value, rankUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool {
				cells, readable := engine.ReadValue(access, row, recurrenceRead)
				value, present, valid := cells.At(0)
				if !readable || !valid || !present && !authority.Equal(value, authority.Bottom()) {
					return false
				}
				return engine.StageValue(access, row, advance(value))
			})
		},
	}, func(rule *engine.Rule[staticdomain.Value, rankUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		recurrenceRead, readOK = engine.ReadFrom(rule, input, readForm)
		recurrenceWrite, writeOK = engine.WriteTo(rule, writeForm)
		return inputOK && readOK && writeOK
	})
	if !recurrenceOK || recurrenceRule == nil {
		t.Fatal("Static rank rules")
	}
	var queryRead engine.QueryRead[engine.OrderedCells[staticdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[staticdomain.Value]{
		Semantic: rankSemantic(15),
		Project: func(observation engine.Observation) staticdomain.Value {
			result := authority.Bottom()
			engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, readable := engine.QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if readable && valid && present {
					result = value
				}
				return readable && valid && present
			})
			return result
		},
		Result: engine.FrozenResult[staticdomain.Value]{
			Semantic: rankSemantic(16), Freeze: func(value staticdomain.Value) staticdomain.Value { return value }, Clone: func(value staticdomain.Value) staticdomain.Value { return value },
			Equal: authority.Equal, Fingerprint: authority.Fingerprint,
		},
	}, func(query *engine.Query[staticdomain.Value]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(query, readForm)
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("Static rank query/composition")
	}
	ref, refOK := factor.Ref(0)
	if !refOK {
		t.Fatal("Static rank Factor Ref")
	}
	recurrenceInstance, recurrenceInstanceOK := engine.NewRuleInstance(recurrenceRule, unit, func(binding *engine.RuleBinding[staticdomain.Value, rankUnit]) bool {
		return engine.InstanceRead(binding, recurrenceRead, ref) && engine.InstanceWrite(binding, recurrenceWrite, ref)
	})
	if !recurrenceInstanceOK {
		t.Fatal("Static rank instances")
	}
	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("Static rank SourceAssembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	recurrenceSite, recurrenceSiteOK := source.Site(rankSemantic(17), scope, truth, true)
	recurrenceOccurrence, recurrenceOccurrenceOK := source.Relation(recurrenceSite, rankSemantic(18))
	recurrencePrepared, recurrencePreparedOK := source.PrepareInstance(recurrenceOccurrence, recurrenceInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	backedge, backedgeOK := source.Boundary(recurrenceSite, recurrenceSite, rankSemantic(19), truth, reindex, truth)
	if !scopeOK || !truthOK || !recurrenceSiteOK || !recurrenceOccurrenceOK || !recurrencePreparedOK || !reindexOK || !backedgeOK || !source.Seal() {
		t.Fatal("Static rank recurrence source")
	}
	var queryInstance *engine.QueryInstance[staticdomain.Value]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		recurrencePoint, recurrencePointOK := assembly.Point(recurrenceSite)
		recurrenceMember, recurrenceMemberOK := assembly.Member(recurrencePoint, recurrencePrepared)
		recurrenceGroup, recurrenceGroupOK := assembly.Group(recurrencePoint, recurrenceMember)
		var queryOK, queryAttached bool
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[staticdomain.Value]) bool {
			return engine.InstanceQueryRead(binding, queryRead, ref)
		})
		if queryOK {
			_, queryAttached = assembly.Query(recurrencePoint, queryInstance)
		}
		return recurrencePointOK && recurrenceMemberOK && recurrenceGroupOK && queryOK && queryAttached && assembly.Boundary(recurrenceGroup, backedge)
	})
	if !assembled || solver == nil || queryInstance == nil {
		t.Fatal("Static rank recurrence assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, resultOK := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !resultOK {
		t.Fatalf("Static rank recurrence = status:%v value:%v/%t", status, result, resultOK)
	}
	return result
}

type rankUnit struct{ digest [32]byte }

func rankSemantic(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[31] = value
	semantic, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("rank semantic")
	}
	return semantic
}

func rankUnitFor(semantic engine.SemanticKey) rankUnit { return rankUnit{digest: semantic.Digest()} }
func rankUnitContent(value rankUnit) (rankUnit, [32]byte, bool) {
	return value, value.digest, value.digest != [32]byte{}
}

func externalRecordValues(t *testing.T, authority *staticdomain.Authority) (staticdomain.Value, staticdomain.Value) {
	t.Helper()
	var left, right staticdomain.Value
	for index := 0; index < authority.CoordinateCount(); index++ {
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			continue
		}
		value, ok := authority.Result(coordinate)
		if !ok {
			continue
		}
		decoded, ok := authority.ClosedType(value)
		record, recordOK := decoded.(*typ.Record)
		if !ok || !recordOK || record.Kind() != kind.Record {
			continue
		}
		if record.GetField("right") != nil {
			right = value
		} else if record.GetField("left") != nil {
			left = value
		}
	}
	if !authority.Owns(left) || !authority.Owns(right) || authority.Equal(left, right) || authority.LessOrEq(left, right) || authority.LessOrEq(right, left) {
		t.Fatal("incomparable record Static values")
	}
	return left, right
}

func externalDynamicValues(t *testing.T, authority *staticdomain.Authority) (staticdomain.Value, staticdomain.Value, staticdomain.Value, staticdomain.Value) {
	t.Helper()
	var empty, dynamic, unknown staticdomain.Value
	for index := 0; index < authority.CoordinateCount(); index++ {
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			continue
		}
		value, ok := authority.Result(coordinate)
		if !ok {
			continue
		}
		decoded, ok := authority.ClosedType(value)
		if !ok {
			continue
		}
		switch typed := decoded.(type) {
		case *typ.Record:
			if len(typed.Fields) == 0 && len(typed.StaticMembers) == 0 && !typed.HasMapComponent() && !typed.Open {
				empty = value
			}
		default:
			if typ.IsAny(decoded) {
				dynamic = value
			}
			if typ.IsUnknown(decoded) {
				unknown = value
			}
		}
	}
	tableTop, tableOK := authority.RuntimeTypeOf(runtimekind.Bit(runtimekind.Table))
	if !authority.Owns(empty) || !tableOK || !authority.Owns(tableTop) || !authority.Owns(dynamic) || !authority.Owns(unknown) {
		t.Fatal("dynamic recurrence values unavailable")
	}
	return empty, tableTop, dynamic, unknown
}

func externalRankAuthority(t *testing.T) *staticdomain.Authority {
	t.Helper()
	program, err := programlower.Lower(programlower.Source{Name: "static_rank_external.lua", Text: []byte(`
		type Left = {left: string}
		type Right = {right: number}
		type Empty = {}
		type Dynamic = any
		type Unknown = unknown
	`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots:    []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries:  []target.InitialEntrySpec{{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable}, {Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__static_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable}},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "static_rank_external", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	authority, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
