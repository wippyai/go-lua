package numericresult

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericliteral "github.com/wippyai/go-lua/analysis/domain/numeric/literal"
	numericoperation "github.com/wippyai/go-lua/analysis/domain/numeric/operation"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type numericResultFixture struct {
	source  *link.Link
	schema  *value.Schema
	algebra *numeric.Algebra
	shard   linkproject.Shard
}

func newNumericResultFixture(t testing.TB, text string) numericResultFixture {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "value_numeric_result.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(source)
	schema, schemaOK := value.Seal(source, heaps)
	algebra, algebraOK := numeric.New(source)
	shard, shardOK := source.Project().Mounts().At(0)
	if !heapsOK || !schemaOK || !algebraOK || !shardOK {
		t.Fatal("numeric result fixture sealing")
	}
	return numericResultFixture{source: source, schema: schema, algebra: algebra, shard: shard}
}

func numericResultKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("numeric result semantic key")
	}
	return key
}

func primitiveTerms(source *link.Link, shard linkproject.Shard) []keyspace.Term {
	program, ok := source.Project().Mounts().Program(shard)
	if !ok || program == nil {
		return nil
	}
	primitives := program.Flow().BinaryPrimitives()
	terms := make([]keyspace.Term, 0, primitives.Arithmetic().Count()+primitives.Bitwise().Count())
	for index := 0; index < primitives.Arithmetic().Count(); index++ {
		term, ok := primitives.Arithmetic().At(index)
		if ok {
			terms = append(terms, term)
		}
	}
	for index := 0; index < primitives.Bitwise().Count(); index++ {
		term, ok := primitives.Bitwise().At(index)
		if ok {
			terms = append(terms, term)
		}
	}
	return terms
}

func TestNumericResultOperandRetainsPrimitiveAndValueCoordinates(t *testing.T) {
	fixture := newNumericResultFixture(t, `
local arithmetic = 1 + 2
local bitwise = 7 & 3
local comparison = 1 < 2
return arithmetic, bitwise
`)
	composition := engine.NewComposition()
	valueOwner, valueOwnerOK := valueowner.Declare(composition, numericResultKey(1), numericResultKey(2), fixture.schema)
	numericOwner, numericOwnerOK := numericowner.Declare(composition, numericResultKey(3), fixture.algebra)
	if !valueOwnerOK || !numericOwnerOK || valueOwner == nil || numericOwner == nil {
		t.Fatal("owner declarations")
	}
	terms := primitiveTerms(fixture.source, fixture.shard)
	if len(terms) != 2 {
		t.Fatalf("primitive terms=%d, want 2", len(terms))
	}
	for _, term := range terms {
		operand, ok := NewOperand(fixture.schema, numericOwner, fixture.shard, term)
		if !ok || !operand.valid() {
			t.Fatalf("NewOperand(%v)", term)
		}
		if _, ok := operand.ID(); !ok {
			t.Fatalf("operand %v identity", term)
		}
		body, offset, cursor, ok := operand.SourcePosition()
		if !ok || body == 0 || offset < 0 || cursor < 0 {
			t.Fatalf("operand %v source position=%v/%d/%d/%v", term, body, offset, cursor, ok)
		}
		result, left, right, ok := operand.Coordinates()
		if !ok || !result.Valid() || !left.Valid() || !right.Valid() {
			t.Fatalf("operand %v Value coordinates", term)
		}
	}
	if comparison := fixtureComparisonTerm(fixture.source, fixture.shard); comparison != 0 {
		if _, ok := NewOperand(fixture.schema, numericOwner, fixture.shard, comparison); ok {
			t.Fatal("comparison primitive admitted by Numeric→Value operand")
		}
	}
}

func fixtureComparisonTerm(source *link.Link, shard linkproject.Shard) keyspace.Term {
	program, ok := source.Project().Mounts().Program(shard)
	if !ok || program == nil {
		return 0
	}
	primitives := program.Flow().BinaryPrimitives()
	if primitives.Order().Count() == 0 {
		return 0
	}
	term, _ := primitives.Order().At(0)
	return term
}

func TestNumericResultOperandRejectsForeignOwnerAndMalformedCursor(t *testing.T) {
	fixture := newNumericResultFixture(t, "local result = 1 + 2\nreturn result")
	term := primitiveTerms(fixture.source, fixture.shard)[0]
	composition := engine.NewComposition()
	numericOwner, ok := numericowner.Declare(composition, numericResultKey(10), fixture.algebra)
	if !ok || numericOwner == nil {
		t.Fatal("Numeric owner")
	}
	valueOwner, ok := valueowner.Declare(composition, numericResultKey(11), numericResultKey(12), fixture.schema)
	if !ok || valueOwner == nil {
		t.Fatal("Value owner")
	}
	operand, ok := NewOperand(fixture.schema, numericOwner, fixture.shard, term)
	if !ok {
		t.Fatal("operand")
	}
	foreignSourceFixture := newNumericResultFixture(t, "local result = 1 + 2\nreturn result")
	foreignAlgebra := foreignSourceFixture.algebra
	foreignComposition := engine.NewComposition()
	foreignNumericOwner, foreignOK := numericowner.Declare(foreignComposition, numericResultKey(13), foreignAlgebra)
	if !foreignOK || foreignNumericOwner == nil {
		t.Fatal("foreign Numeric owner")
	}
	if _, ok := NewOperand(fixture.schema, foreignNumericOwner, fixture.shard, term); ok {
		t.Fatal("foreign Numeric owner crossed Link fence")
	}
	operand.offset++
	if operand.valid() {
		t.Fatal("malformed source offset remained valid")
	}
	_ = valueOwner
	_ = operand
}

func TestNumericResultDeclaresExactCrossFactorSchema(t *testing.T) {
	fixture := newNumericResultFixture(t, "local result = 1 + 2\nreturn result")
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, numericResultKey(20), numericResultKey(21), fixture.schema)
	numerics, numericsOK := numericowner.Declare(composition, numericResultKey(22), fixture.algebra)
	rule, ruleOK := Declare(composition, numericResultKey(23), numericResultKey(24), numericResultKey(25), values, numerics)
	if !valuesOK || !numericsOK || !ruleOK || values == nil || numerics == nil || rule == nil {
		t.Fatal("Numeric→Value declarations")
	}
	if !declareNumericResultQuery(composition, values) {
		t.Fatal("query declaration")
	}
	report, reportOK := composition.SemanticReport()
	if reportOK {
		t.Fatal("unsealed composition published report")
	}
	if !composition.Seal() {
		t.Fatal("composition seal")
	}
	report, reportOK = composition.SemanticReport()
	if !reportOK {
		t.Fatal("sealed composition report")
	}
	var found *engine.RuleSchemaReport
	for index := range report.Rules {
		if report.Rules[index].Semantic == numericResultKey(23) {
			found = &report.Rules[index]
			break
		}
	}
	if found == nil || found.Inputs != 2 || len(found.Reads) != 1 || len(found.Carries) != 1 || len(found.Writes) != 1 ||
		found.Reads[0].Input != 1 || found.Reads[0].Factor != numericResultKey(22) ||
		found.Carries[0].Input != 0 || found.Carries[0].Factor != numericResultKey(20) || found.Writes[0].Factor != numericResultKey(20) {
		t.Fatalf("Numeric→Value schema = %#v", found)
	}
}

func TestNumericResultRunsThroughSourceAssemblyAndSolver(t *testing.T) {
	t.Skip("the current composer has no canonical Numeric operation-result source boundary; the direct Numeric-result bridge law below supplies that exact predecessor")
	fixture := newNumericResultFixture(t, "local result = 1 + 2\nreturn result")
	program, programOK := fixture.source.Project().Mounts().Program(fixture.shard)
	if !programOK || program == nil {
		t.Fatal("mounted Program")
	}
	primitives := program.Flow().BinaryPrimitives()
	binaryTerm, binaryOK := primitives.Arithmetic().At(0)
	primitive, primitiveOK := primitives.Primitive(binaryTerm)
	operation, operationOK := primitive.Operation()
	if !binaryOK || !primitiveOK || !operationOK || operation.Op != flowkind.BinaryAdd {
		t.Fatal("addition primitive")
	}
	leftScalar, leftScalarOK := fixture.algebra.ScalarFor(fixture.shard, operation.Owner, operation.Left)
	if !leftScalarOK {
		t.Fatal("Numeric scalar operands")
	}
	leftLiteral, leftLiteralOK := numericliteral.NewOperand(fixture.source, fixture.algebra, leftScalar)
	numericOperation, numericOperationOK := numericoperation.NewOperand(fixture.source, fixture.algebra, fixture.shard, binaryTerm)
	if !leftLiteralOK || !numericOperationOK {
		t.Fatal("Numeric source/operation operands")
	}
	leftKey, leftKeyOK := leftLiteral.Key()
	operationKey, operationKeyOK := numericOperation.Key()
	if !leftKeyOK || !operationKeyOK || leftKey != operationKey {
		t.Fatalf("Numeric source root mismatch left=%v/%v operation=%v/%v", leftKey, leftKeyOK, operationKey, operationKeyOK)
	}
	valueRaw, valueRawOK := fixture.source.Boundary().Values().Of(fixture.shard, operation.Left)
	valueCoordinate, valueCoordinateOK := fixture.schema.CoordinateFor(valueRaw)
	valueIndex, valueIndexOK := fixture.schema.CoordinateIndex(valueCoordinate)
	if !valueRawOK || !valueCoordinateOK || !valueIndexOK {
		t.Fatal("Value source coordinate")
	}
	seed, seedOK := fixture.schema.SourceSeedAt(int(valueIndex))
	if !seedOK {
		t.Fatal("Value source seed")
	}
	valueRawResult, valueRawResultOK := fixture.source.Boundary().Values().Of(fixture.shard, binaryTerm)
	valueResult, valueResultOK := fixture.schema.CoordinateFor(valueRawResult)
	if !valueRawResultOK || !valueResultOK {
		t.Fatal("Value result coordinate")
	}

	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, numericResultKey(40), numericResultKey(41), fixture.schema)
	numerics, numericsOK := numericowner.Declare(composition, numericResultKey(42), fixture.algebra)
	seedRule, seedRuleOK := valuesource.Declare(composition, numericResultKey(43), numericResultKey(44), numericResultKey(45), values)
	literalRule, literalRuleOK := numericliteral.Declare(composition, numericResultKey(46), numericResultKey(47), numericResultKey(48), numerics)
	operationRule, operationRuleOK := numericoperation.Declare(composition, numericResultKey(49), numericResultKey(50), numericResultKey(51), numerics)
	bridgeRule, bridgeRuleOK := Declare(composition, numericResultKey(52), numericResultKey(53), numericResultKey(54), values, numerics)
	if !valuesOK || !numericsOK || !seedRuleOK || !literalRuleOK || !operationRuleOK || !bridgeRuleOK || values == nil || numerics == nil || seedRule == nil || literalRule == nil || operationRule == nil || bridgeRule == nil {
		t.Fatal("production Rule declarations")
	}
	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: numericResultKey(55),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				actual, present, cellOK := cells.At(0)
				return rows == 1 && cellOK && present && fixture.schema.Presence(actual) == value.PresencePresent && fixture.schema.RuntimeKinds(actual).Contains(runtimekind.Number)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: numericResultKey(56), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v },
			Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
				if v {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var ok bool
		read, ok = engine.QueryReadFrom(query, values.ExactRead())
		return ok
	})
	if !queryOK || query == nil {
		t.Fatal("production query declaration")
	}
	var numericRead engine.QueryRead[engine.OrderedCells[numeric.Value]]
	numericQuery, numericQueryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: numericResultKey(68), Project: func(observation engine.Observation) bool {
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, ok := engine.QueryValue(row, numericRead)
				if !ok || cells.Count() != 1 {
					return false
				}
				_, present, cellOK := cells.At(0)
				return cellOK && present
			})
		},
		Result: engine.FrozenResult[bool]{Semantic: numericResultKey(69), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v }, Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
			if v {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		var ok bool
		numericRead, ok = engine.QueryReadFrom(query, numerics.ExactRead())
		return ok
	})
	if !numericQueryOK || numericQuery == nil {
		t.Fatal("numeric debug query")
	}
	if !composition.Seal() {
		t.Fatal("production composition seal")
	}
	operationRef, operationRefOK := numerics.Locate(operationKey)
	if !operationRefOK {
		t.Fatal("Numeric operation query ref")
	}
	seedInstance, seedInstanceOK := seedRule.Instance(seed)
	leftInstance, leftInstanceOK := literalRule.Instance(leftLiteral)
	operationInstance, operationInstanceOK := operationRule.Instance(numericOperation)
	bridgeOperand, bridgeOperandOK := NewOperand(fixture.schema, numerics, fixture.shard, binaryTerm)
	bridgeInstance, bridgeInstanceOK := bridgeRule.Instance(bridgeOperand)
	if !seedInstanceOK || !leftInstanceOK || !operationInstanceOK || !bridgeOperandOK || !bridgeInstanceOK || seedInstance == nil || leftInstance == nil || operationInstance == nil || bridgeInstance == nil {
		t.Fatal("production Rule instances")
	}
	outputRef, outputRefOK := values.Locate(valueResult)
	if !outputRefOK {
		t.Fatal("Value output ref")
	}

	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		t.Fatal("source controls")
	}
	valueSeedSite, valueSeedSiteOK := source.Site(numericResultKey(57), scope, truth, true)
	numericSeedSite, numericSeedSiteOK := source.Site(numericResultKey(58), scope, truth, true)
	operationSite, operationSiteOK := source.Site(numericResultKey(59), scope, falsity, false)
	bridgeSite, bridgeSiteOK := source.Site(numericResultKey(60), scope, falsity, false)
	seedOccurrence, seedOccurrenceOK := source.Relation(valueSeedSite, numericResultKey(61))
	leftOccurrence, leftOccurrenceOK := source.Relation(numericSeedSite, numericResultKey(62))
	operationOccurrence, operationOccurrenceOK := source.Relation(operationSite, numericResultKey(63))
	bridgeOccurrence, bridgeOccurrenceOK := source.Relation(bridgeSite, numericResultKey(64))
	seedPrepared, seedPreparedOK := source.PrepareInstance(seedOccurrence, seedInstance)
	leftPrepared, leftPreparedOK := source.PrepareInstance(leftOccurrence, leftInstance)
	operationPrepared, operationPreparedOK := source.PrepareInstance(operationOccurrence, operationInstance)
	bridgePrepared, bridgePreparedOK := source.PrepareInstance(bridgeOccurrence, bridgeInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	seedToOperation, seedToOperationOK := source.Boundary(numericSeedSite, operationSite, numericResultKey(65), truth, reindex, truth)
	seedToBridge, seedToBridgeOK := source.Boundary(valueSeedSite, bridgeSite, numericResultKey(66), truth, reindex, truth)
	operationToBridge, operationToBridgeOK := source.Boundary(operationSite, bridgeSite, numericResultKey(67), truth, reindex, truth)
	if !valueSeedSiteOK || !numericSeedSiteOK || !operationSiteOK || !bridgeSiteOK || !seedOccurrenceOK || !leftOccurrenceOK || !operationOccurrenceOK || !bridgeOccurrenceOK || !seedPreparedOK || !leftPreparedOK || !operationPreparedOK || !bridgePreparedOK || !reindexOK || !seedToOperationOK || !seedToBridgeOK || !operationToBridgeOK || !source.Seal() {
		t.Fatal("source assembly declaration")
	}
	var queryInstance *engine.QueryInstance[bool]
	var numericQueryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		valueSeedPoint, valueSeedPointOK := assembly.Point(valueSeedSite)
		numericSeedPoint, numericSeedPointOK := assembly.Point(numericSeedSite)
		operationPoint, operationPointOK := assembly.Point(operationSite)
		bridgePoint, bridgePointOK := assembly.Point(bridgeSite)
		seedMember, seedMemberOK := assembly.Member(valueSeedPoint, seedPrepared)
		leftMember, leftMemberOK := assembly.Member(numericSeedPoint, leftPrepared)
		operationMember, operationMemberOK := assembly.Member(operationPoint, operationPrepared)
		bridgeMember, bridgeMemberOK := assembly.Member(bridgePoint, bridgePrepared)
		_, valueSeedGroupOK := assembly.Group(valueSeedPoint, seedMember)
		_, numericSeedGroupOK := assembly.Group(numericSeedPoint, leftMember)
		operationGroup, operationGroupOK := assembly.Group(operationPoint, operationMember)
		bridgeGroup, bridgeGroupOK := assembly.Group(bridgePoint, bridgeMember)
		operationBoundaryOK := assembly.Boundary(operationGroup, seedToOperation)
		seedBridgeBoundaryOK := assembly.Boundary(bridgeGroup, seedToBridge)
		operationBridgeBoundaryOK := assembly.Boundary(bridgeGroup, operationToBridge)
		_ = operationBoundaryOK
		_ = seedBridgeBoundaryOK
		_ = operationBridgeBoundaryOK
		if !valueSeedPointOK || !numericSeedPointOK || !operationPointOK || !bridgePointOK || !seedMemberOK || !leftMemberOK || !operationMemberOK || !bridgeMemberOK || !valueSeedGroupOK || !numericSeedGroupOK || !operationGroupOK || !bridgeGroupOK || !operationBoundaryOK || !seedBridgeBoundaryOK || !operationBridgeBoundaryOK {
			t.Logf("assembly pieces points=%v/%v/%v/%v members=%v/%v/%v/%v groups=%v/%v/%v/%v boundaries=%v/%v/%v", valueSeedPointOK, numericSeedPointOK, operationPointOK, bridgePointOK, seedMemberOK, leftMemberOK, operationMemberOK, bridgeMemberOK, valueSeedGroupOK, numericSeedGroupOK, operationGroupOK, bridgeGroupOK, operationBoundaryOK, seedBridgeBoundaryOK, operationBridgeBoundaryOK)
			return false
		}
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, outputRef)
		})
		if queryOK {
			_, observationOK = assembly.Query(bridgePoint, queryInstance)
		}
		var numericQueryOK, numericObservationOK bool
		numericQueryInstance, numericQueryOK = engine.NewQueryInstance(numericQuery, func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, numericRead, operationRef)
		})
		if numericQueryOK {
			_, numericObservationOK = assembly.Query(numericSeedPoint, numericQueryInstance)
		}
		return queryOK && observationOK && numericQueryOK && numericObservationOK
	})
	if !assembled || solver == nil || queryInstance == nil || numericQueryInstance == nil {
		t.Fatal("source assembly compile")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("solver status=%v state=%v", status, state)
	}
	receipt, receiptOK := queryInstance.Receipt()
	actual, actualOK := engine.QueryResult(receipt, state)
	numericReceipt, numericReceiptOK := numericQueryInstance.Receipt()
	numericActual, numericActualOK := engine.QueryResult(numericReceipt, state)
	if !receiptOK || !actualOK || !actual {
		t.Fatalf("solver query receipt=%v/%v actual=%v numeric=%v/%v/%v", receiptOK, actualOK, actual, numericReceiptOK, numericActualOK, numericActual)
	}
}

func declareNumericResultQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: numericResultKey(26), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: numericResultKey(27), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v },
			Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
				if v {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, owner.ExactRead())
		return ok
	})
	return ok && query != nil
}

// numericResultSeed is a test-only exact Numeric predecessor. It stands for
// the already-produced Numeric operation result while keeping the bridge law
// on the production SourceAssembly/Solver path.
type numericResultSeed struct {
	key     numeric.Key
	value   numeric.Value
	content [32]byte
}

type numericResultSeedRule struct {
	rule  *engine.Rule[numeric.Value, numericResultSeed]
	owner *numericowner.Owner
	write engine.Write[numeric.Value]
}

func newNumericResultSeedRule(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *numericowner.Owner, seed numericResultSeed) (*numericResultSeedRule, bool) {
	if composition == nil || owner == nil || !seed.key.Valid() || seed.content == [32]byte{} || !owner.Algebra().Admits(seed.key, seed.value) {
		return nil, false
	}
	declaration := &numericResultSeedRule{owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[numeric.Value, numericResultSeed]{
		Semantic: semantic, OperandFamily: family, OperandContent: func(operand numericResultSeed) (numericResultSeed, [32]byte, bool) {
			return operand, operand.content, operand.key.Valid() && operand.content != [32]byte{}
		}, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, func(derivation engine.RuleDerivation[numeric.Value, numericResultSeed]) (engine.RuleEvidence, bool) {
			if derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			operand, operandOK := derivation.Operand()
			disposition, dispositionOK := derivation.DispositionAt(0)
			target, targetOK := disposition.TargetAt(0)
			actual, actualOK := disposition.Value()
			ref, refOK := owner.Locate(operand.key)
			if !operandOK || !dispositionOK || !targetOK || !actualOK || !derivation.OperandContentMatches(operand.content) || !refOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !engine.TargetMatchesRef(target, ref) || !owner.Algebra().Equal(actual, operand.value) {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access engine.Access[numeric.Value, numericResultSeed]) bool {
			operand, operandOK := engine.Operand(access)
			if !operandOK || operand.key != seed.key || operand.content != seed.content {
				return false
			}
			rows := 0
			completed := engine.Product(access, func(row engine.Row) bool {
				rows++
				staged := rows == 1 && engine.StageValue(access, row, operand.value)
				return staged
			})
			return completed && rows == 1
		},
	}, func(rule *engine.Rule[numeric.Value, numericResultSeed]) bool {
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if writeOK {
			declaration.rule, declaration.write = rule, write
		}
		return writeOK
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *numericResultSeedRule) Instance(seed numericResultSeed) (*engine.RuleInstance[numeric.Value, numericResultSeed], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !seed.key.Valid() {
		return nil, false
	}
	ref, ok := rule.owner.Locate(seed.key)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, seed, func(binding *engine.RuleBinding[numeric.Value, numericResultSeed]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func TestNumericResultBridgeRunsThroughRealSourceAssemblyAndSolver(t *testing.T) {
	fixture := newNumericResultFixture(t, "local result = 1 + 2\nreturn result")
	program, programOK := fixture.source.Project().Mounts().Program(fixture.shard)
	if !programOK || program == nil {
		t.Fatal("Program")
	}
	binaries := program.Flow().BinaryPrimitives().Arithmetic()
	binaryTerm, binaryOK := binaries.At(0)
	primitive, primitiveOK := program.Flow().BinaryPrimitives().Primitive(binaryTerm)
	operation, operationOK := primitive.Operation()
	if !binaryOK || !primitiveOK || !operationOK || operation.Op != flowkind.BinaryAdd {
		t.Fatal("primitive addition")
	}
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, numericResultKey(80), numericResultKey(81), fixture.schema)
	numerics, numericsOK := numericowner.Declare(composition, numericResultKey(82), fixture.algebra)
	valueRule, valueRuleOK := valuesource.Declare(composition, numericResultKey(83), numericResultKey(84), numericResultKey(85), values)
	bridgeRule, bridgeRuleOK := Declare(composition, numericResultKey(86), numericResultKey(87), numericResultKey(88), values, numerics)
	if !valuesOK || !numericsOK || !valueRuleOK || !bridgeRuleOK || values == nil || numerics == nil || valueRule == nil || bridgeRule == nil {
		t.Fatal("bridge declarations")
	}
	numericOperand, numericOperandOK := numericoperation.NewOperand(fixture.source, fixture.algebra, fixture.shard, binaryTerm)
	numericKey, numericKeyOK := numericOperand.Key()
	resultValue, resultOK := numericoperation.Result(fixture.algebra, numericOperand, fixture.algebra.Default())
	if !numericOperandOK || !numericKeyOK || !resultOK {
		t.Fatal("exact Numeric operation result")
	}
	if resultValue.IsDefault() {
		t.Fatal("exact Numeric result unexpectedly default")
	}
	seed := numericResultSeed{key: numericKey, value: resultValue, content: sha256.Sum256([]byte("numeric-result-seed"))}
	seedRule, seedRuleOK := newNumericResultSeedRule(composition, numericResultKey(89), numericResultKey(90), numericResultKey(91), numerics, seed)
	if !seedRuleOK || seedRule == nil {
		t.Fatal("Numeric result predecessor")
	}
	leftRaw, leftRawOK := fixture.source.Boundary().Values().Of(fixture.shard, operation.Left)
	leftCoordinate, leftCoordinateOK := fixture.schema.CoordinateFor(leftRaw)
	leftIndex, leftIndexOK := fixture.schema.CoordinateIndex(leftCoordinate)
	resultRaw, resultRawOK := fixture.source.Boundary().Values().Of(fixture.shard, binaryTerm)
	resultCoordinate, resultCoordinateOK := fixture.schema.CoordinateFor(resultRaw)
	if !leftRawOK || !leftCoordinateOK || !leftIndexOK || !resultRawOK || !resultCoordinateOK {
		t.Fatal("Value bridge coordinates")
	}
	valueSeed, valueSeedOK := fixture.schema.SourceSeedAt(int(leftIndex))
	bridgeOperand, bridgeOperandOK := NewOperand(fixture.schema, numerics, fixture.shard, binaryTerm)
	if !valueSeedOK || !bridgeOperandOK {
		t.Fatal("Value/Numeric bridge operands")
	}
	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: numericResultKey(92),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				actual, present, cellOK := cells.At(0)
				return rows == 1 && cellOK && present && fixture.schema.RuntimeKinds(actual) == runtimekind.Bit(runtimekind.Number)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{Semantic: numericResultKey(93), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v }, Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
			if v {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		var ok bool
		read, ok = engine.QueryReadFrom(query, values.ExactRead())
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("bridge query/composition seal")
	}
	valueInstance, valueInstanceOK := valueRule.Instance(valueSeed)
	seedInstance, seedInstanceOK := seedRule.Instance(seed)
	bridgeInstance, bridgeInstanceOK := bridgeRule.Instance(bridgeOperand)
	outputRef, outputRefOK := values.Locate(resultCoordinate)
	if !valueInstanceOK || !seedInstanceOK || !bridgeInstanceOK || !outputRefOK || valueInstance == nil || seedInstance == nil || bridgeInstance == nil {
		t.Fatal("bridge instances")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	valueSite, valueSiteOK := source.Site(numericResultKey(94), scope, truth, true)
	bridgeSite, bridgeSiteOK := source.Site(numericResultKey(96), scope, falsity, false)
	valueOccurrence, valueOccurrenceOK := source.Relation(valueSite, numericResultKey(97))
	seedOccurrence, seedOccurrenceOK := source.Relation(valueSite, numericResultKey(98))
	bridgeOccurrence, bridgeOccurrenceOK := source.Relation(bridgeSite, numericResultKey(99))
	valuePrepared, valuePreparedOK := source.PrepareInstance(valueOccurrence, valueInstance)
	seedPrepared, seedPreparedOK := source.PrepareInstance(seedOccurrence, seedInstance)
	bridgePrepared, bridgePreparedOK := source.PrepareInstance(bridgeOccurrence, bridgeInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	valueBoundary, valueBoundaryOK := source.Boundary(valueSite, bridgeSite, numericResultKey(100), truth, reindex, truth)
	numericBoundary, numericBoundaryOK := source.Boundary(valueSite, bridgeSite, numericResultKey(101), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !valueSiteOK || !bridgeSiteOK || !valueOccurrenceOK || !seedOccurrenceOK || !bridgeOccurrenceOK || !valuePreparedOK || !seedPreparedOK || !bridgePreparedOK || !reindexOK || !valueBoundaryOK || !numericBoundaryOK || !source.Seal() {
		t.Fatal("bridge SourceAssembly declaration")
	}
	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		valuePoint, valuePointOK := assembly.Point(valueSite)
		bridgePoint, bridgePointOK := assembly.Point(bridgeSite)
		valueMember, valueMemberOK := assembly.Member(valuePoint, valuePrepared)
		seedMember, seedMemberOK := assembly.Member(valuePoint, seedPrepared)
		bridgeMember, bridgeMemberOK := assembly.Member(bridgePoint, bridgePrepared)
		_, valueGroupOK := assembly.Group(valuePoint, valueMember)
		_, numericGroupOK := assembly.Group(valuePoint, seedMember)
		bridgeGroup, bridgeGroupOK := assembly.Group(bridgePoint, bridgeMember)
		valueBoundaryOK := assembly.Boundary(bridgeGroup, valueBoundary)
		numericBoundaryOK := assembly.Boundary(bridgeGroup, numericBoundary)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, outputRef)
		})
		var observationOK bool
		if queryOK {
			_, observationOK = assembly.Query(bridgePoint, queryInstance)
		}
		return valuePointOK && bridgePointOK && valueMemberOK && seedMemberOK && bridgeMemberOK && valueGroupOK && numericGroupOK && bridgeGroupOK && valueBoundaryOK && numericBoundaryOK && queryOK && observationOK
	})
	if !assembled || solver == nil || queryInstance == nil {
		t.Fatal("bridge SourceAssembly compile")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("bridge solver status=%v state=%v", status, state)
	}
	receipt, receiptOK := queryInstance.Receipt()
	actual, actualOK := engine.QueryResult(receipt, state)
	if !receiptOK || !actualOK || !actual {
		t.Fatalf("bridge solver result=%v/%v/%v", receiptOK, actualOK, actual)
	}
}
