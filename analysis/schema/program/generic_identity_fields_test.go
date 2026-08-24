package programschema_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

type genericIdentityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
	text  string
}

type genericIdentityOperations []genericIdentityOperation

func (operations *genericIdentityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, genericIdentityOperation{kind: 'i', id: id})
	return true
}

func (operations *genericIdentityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, genericIdentityOperation{kind: 'u', value: value})
	return true
}

func (operations *genericIdentityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, genericIdentityOperation{kind: 'b', value: encoded})
	return true
}

func (operations *genericIdentityOperations) WriteString(value string) bool {
	*operations = append(*operations, genericIdentityOperation{kind: 's', text: value})
	return true
}

func genericIdentityID(value byte) identity.ContentID { return identity.ContentID{value} }

func genericIdentityProgram(t *testing.T, publication programpublication.Publication) programschema.Program {
	t.Helper()
	catalog, catalogOK := programcatalog.CatalogID(genericIdentityID(250))
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := publication.Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal publication")
	}
	return programschema.Program{Frozen: frozen}
}

func TestOccurrenceIdentityFieldsPreserveLiteralAndChildOrder(t *testing.T) {
	occurrenceID, bodyID, pointID, inputID := genericIdentityID(1), genericIdentityID(2), genericIdentityID(3), genericIdentityID(4)
	occurrence, occurrenceOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, occurrenceID, bodyID, 9, 0, 1, 0, 1,
		keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "source"}, true,
	)
	point, pointOK := programschema.NewOccurrencePoint(pointID)
	input, inputOK := programschema.NewOccurrenceInput(inputID)
	if !occurrenceOK || !pointOK || !inputOK {
		t.Fatal("occurrence rows")
	}
	program := genericIdentityProgram(t, programpublication.Publication{
		Occurrences: []programschema.Occurrence{occurrence}, OccurrencePoints: []programschema.OccurrencePoint{point}, OccurrenceInputs: []programschema.OccurrenceInput{input},
	})
	var got genericIdentityOperations
	if !program.WriteOccurrenceIdentityFields(&got) {
		t.Fatal("write occurrence identity")
	}
	i := func(id identity.ContentID) genericIdentityOperation {
		return genericIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) genericIdentityOperation { return genericIdentityOperation{kind: 'u', value: value} }
	b := func(value bool) genericIdentityOperation {
		if value {
			return genericIdentityOperation{kind: 'b', value: 1}
		}
		return genericIdentityOperation{kind: 'b'}
	}
	s := func(value string) genericIdentityOperation { return genericIdentityOperation{kind: 's', text: value} }
	want := genericIdentityOperations{
		u(1), u(uint64(programschema.OccurrenceValueSource)), i(occurrenceID), i(bodyID), u(9), u(1), i(pointID), u(1), i(inputID),
		u(uint64(keyspace.FamilyString)), b(true), u(uint64(keyspace.LiteralString)), b(false), u(0), u(0), s("source"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("occurrence identity operations = %#v, want %#v", got, want)
	}
}

func TestSummaryIdentityFieldsPreserveFamilyOrder(t *testing.T) {
	occurrenceID, subjectID, bodyID, pointID := genericIdentityID(11), genericIdentityID(12), genericIdentityID(13), genericIdentityID(14)
	negativeInteger := int64(-7)
	exact, exactOK := programschema.NewExactScalarSummary(occurrenceID, subjectID, bodyID, programschema.ExactScalarSummaryLeft, programschema.SummaryLiteral{Kind: 2, Integer: negativeInteger, FloatBits: 8})
	arithmetic, arithmeticOK := programschema.NewArithmeticSummary(occurrenceID, bodyID, programschema.SummaryOperator(3), programschema.NumericRepresentationInteger, programschema.NumericRepresentationFloat, programschema.NumericRepresentationNumber, programschema.ArithmeticDivisorNonzero)
	unary, unaryOK := programschema.NewUnarySummary(occurrenceID, bodyID, pointID, programschema.SummaryOperator(4), programschema.NumericRepresentationFloat, programschema.NumericRepresentationNumber)
	if !exactOK || !arithmeticOK || !unaryOK {
		t.Fatal("summary rows")
	}
	program := genericIdentityProgram(t, programpublication.Publication{ExactScalarSummaries: []programschema.ExactScalarSummary{exact}, ArithmeticSummaries: []programschema.ArithmeticSummary{arithmetic}, UnarySummaries: []programschema.UnarySummary{unary}})
	var got genericIdentityOperations
	if !program.WriteSummaryIdentityFields(&got) {
		t.Fatal("write summary identity")
	}
	i := func(id identity.ContentID) genericIdentityOperation {
		return genericIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) genericIdentityOperation { return genericIdentityOperation{kind: 'u', value: value} }
	want := genericIdentityOperations{
		u(1), i(exact.ID()), i(occurrenceID), i(subjectID), i(bodyID), u(uint64(programschema.ExactScalarSummaryLeft)), u(2), u(uint64(negativeInteger)), u(8),
		u(1), i(arithmetic.ID()), i(occurrenceID), i(bodyID), u(3), u(uint64(programschema.NumericRepresentationInteger)), u(uint64(programschema.NumericRepresentationFloat)), u(uint64(programschema.NumericRepresentationNumber)), u(uint64(programschema.ArithmeticDivisorNonzero)),
		u(1), i(unary.ID()), i(occurrenceID), i(bodyID), i(pointID), u(4), u(uint64(programschema.NumericRepresentationFloat)), u(uint64(programschema.NumericRepresentationNumber)),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary identity operations = %#v, want %#v", got, want)
	}
}

func TestEnvironmentAndLocalTransferIdentityFieldsPreserveWitnessAndKeyOrder(t *testing.T) {
	edgeID, fromID, toID, routeID := genericIdentityID(21), genericIdentityID(22), genericIdentityID(23), genericIdentityID(24)
	transferID := genericIdentityID(25)
	edge, edgeOK := programschema.NewEnvironmentEdge(edgeID, fromID, toID, routeID, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, 0, 0, programschema.EnvironmentArmLocal, false, false, false, false)
	transfer, transferOK := programschema.NewLocalTransfer(transferID, fromID, toID, false, 0, 1)
	write, writeOK := programschema.NewLocalTransferWrite(schema.Key("value-source"))
	if !edgeOK || !transferOK || !writeOK {
		t.Fatal("environment/local-transfer rows")
	}
	program := genericIdentityProgram(t, programpublication.Publication{EnvironmentEdges: []programschema.EnvironmentEdge{edge}, LocalTransfers: []programschema.LocalTransfer{transfer}, LocalTransferWrites: []programschema.LocalTransferWrite{write}})
	var got genericIdentityOperations
	if !program.WriteEnvironmentLocalTransferIdentityFields(&got) {
		t.Fatal("write environment/local-transfer identity")
	}
	i := func(id identity.ContentID) genericIdentityOperation {
		return genericIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) genericIdentityOperation { return genericIdentityOperation{kind: 'u', value: value} }
	b := func(value bool) genericIdentityOperation {
		if value {
			return genericIdentityOperation{kind: 'b', value: 1}
		}
		return genericIdentityOperation{kind: 'b'}
	}
	s := func(value string) genericIdentityOperation { return genericIdentityOperation{kind: 's', text: value} }
	want := genericIdentityOperations{
		u(1), i(edgeID), i(fromID), i(toID), i(routeID), u(uint64(programschema.EnvironmentArmLocal)), i(identity.ContentID{}), i(identity.ContentID{}), i(identity.ContentID{}), b(false), b(false), i(identity.ContentID{}), i(identity.ContentID{}), b(false), i(identity.ContentID{}), b(false), u(0),
		u(1), i(transferID), i(fromID), i(toID), b(false), u(1), s("value-source"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment/local-transfer identity operations = %#v, want %#v", got, want)
	}
}

func TestRuleOccurrenceIdentityFieldsPreserveKeyAndOptionalPayloadOrder(t *testing.T) {
	pointID := genericIdentityID(31)
	rule, ruleOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("rule-key"), schema.Key("writes-key"), 0, pointID, nil, programissuance.StageBase, programissuance.InputNone, programschema.RuleOccurrenceRoute{}, false, programschema.RuleOccurrenceSource{})
	if !ruleOK {
		t.Fatal("rule occurrence")
	}
	program := genericIdentityProgram(t, programpublication.Publication{RuleOccurrences: []programschema.RuleOccurrence{rule}})
	var got genericIdentityOperations
	if !program.WriteRuleOccurrenceIdentityFields(&got) {
		t.Fatal("write rule identity")
	}
	i := func(id identity.ContentID) genericIdentityOperation {
		return genericIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) genericIdentityOperation { return genericIdentityOperation{kind: 'u', value: value} }
	s := func(value string) genericIdentityOperation { return genericIdentityOperation{kind: 's', text: value} }
	b := func(value bool) genericIdentityOperation {
		if value {
			return genericIdentityOperation{kind: 'b', value: 1}
		}
		return genericIdentityOperation{kind: 'b'}
	}
	want := genericIdentityOperations{u(1), s("rule-key"), s("writes-key"), u(0), i(pointID), i(identity.ContentID{}), s(string(programissuance.StageBase)), s(string(programissuance.InputNone)), i(identity.ContentID{}), b(false)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rule identity operations = %#v, want %#v", got, want)
	}
}

func TestRuleOccurrenceNativeBitParticipatesInIdentity(t *testing.T) {
	pointID, inputID := genericIdentityID(32), genericIdentityID(33)
	ordinary, ordinaryOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("rule-key"), schema.Key("writes-key"), 0, pointID, []identity.ContentID{inputID}, programissuance.StageCallDispatch, programissuance.InputPreviousStage, programschema.RuleOccurrenceRoute{}, false, programschema.RuleOccurrenceSource{})
	native, nativeOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("rule-key"), schema.Key("writes-key"), 0, pointID, []identity.ContentID{inputID}, programissuance.StageCallDispatch, programissuance.InputPreviousStage, programschema.RuleOccurrenceRoute{}, true, programschema.RuleOccurrenceSource{})
	if !ordinaryOK || !nativeOK {
		t.Fatal("rule occurrence variants")
	}
	var ordinaryFields, nativeFields genericIdentityOperations
	if !genericIdentityProgram(t, programpublication.Publication{RuleOccurrences: []programschema.RuleOccurrence{ordinary}}).WriteRuleOccurrenceIdentityFields(&ordinaryFields) ||
		!genericIdentityProgram(t, programpublication.Publication{RuleOccurrences: []programschema.RuleOccurrence{native}}).WriteRuleOccurrenceIdentityFields(&nativeFields) {
		t.Fatal("write rule identity variants")
	}
	if reflect.DeepEqual(ordinaryFields, nativeFields) || len(ordinaryFields) != len(nativeFields) ||
		ordinaryFields[len(ordinaryFields)-1] != (genericIdentityOperation{kind: 'b'}) ||
		nativeFields[len(nativeFields)-1] != (genericIdentityOperation{kind: 'b', value: 1}) {
		t.Fatalf("native bit did not exclusively change the canonical rule identity: ordinary=%#v native=%#v", ordinaryFields, nativeFields)
	}
}

func TestRuleOccurrenceRoutePointParticipatesInIdentityIndependently(t *testing.T) {
	pointID, inputID := genericIdentityID(36), genericIdentityID(37)
	routeID, firstLanding, secondLanding := genericIdentityID(38), genericIdentityID(39), genericIdentityID(40)
	first, firstOK := programschema.NewRuleOccurrenceWithInputs(
		schema.Key("rule-key"), schema.Key("writes-key"), 0, pointID,
		[]identity.ContentID{inputID}, programissuance.StagePredecessor, programissuance.InputPreviousStage,
		programschema.RuleOccurrenceRoute{Point: firstLanding, ID: routeID}, false, programschema.RuleOccurrenceSource{},
	)
	second, secondOK := programschema.NewRuleOccurrenceWithInputs(
		schema.Key("rule-key"), schema.Key("writes-key"), 0, pointID,
		[]identity.ContentID{inputID}, programissuance.StagePredecessor, programissuance.InputPreviousStage,
		programschema.RuleOccurrenceRoute{Point: secondLanding, ID: routeID}, false, programschema.RuleOccurrenceSource{},
	)
	if !firstOK || !secondOK {
		t.Fatal("routed rule occurrence variants")
	}
	var firstFields, secondFields genericIdentityOperations
	if !genericIdentityProgram(t, programpublication.Publication{RuleOccurrences: []programschema.RuleOccurrence{first}}).WriteRuleOccurrenceIdentityFields(&firstFields) ||
		!genericIdentityProgram(t, programpublication.Publication{RuleOccurrences: []programschema.RuleOccurrence{second}}).WriteRuleOccurrenceIdentityFields(&secondFields) {
		t.Fatal("write routed rule identity variants")
	}
	if reflect.DeepEqual(firstFields, secondFields) || len(firstFields) != len(secondFields) || len(firstFields) < 2 {
		t.Fatalf("route landing did not independently remint rule identity: first=%#v second=%#v", firstFields, secondFields)
	}
	for index := range firstFields {
		if index == len(firstFields)-2 {
			if firstFields[index] != (genericIdentityOperation{kind: 'i', id: firstLanding}) || secondFields[index] != (genericIdentityOperation{kind: 'i', id: secondLanding}) {
				t.Fatalf("route landing identity slot was not canonical: first=%#v second=%#v", firstFields[index], secondFields[index])
			}
			continue
		}
		if firstFields[index] != secondFields[index] {
			t.Fatalf("route landing changed unrelated identity slot %d: first=%#v second=%#v", index, firstFields[index], secondFields[index])
		}
	}
}

func TestRuleOccurrenceInputRolesPreserveOrdinalAliasing(t *testing.T) {
	pointID, inputID := genericIdentityID(34), genericIdentityID(35)
	rule, ruleOK := programschema.NewRuleOccurrenceWithInputs(
		schema.Key("aliased-input-rule"), schema.Key("aliased-input-axis"), 0,
		pointID, []identity.ContentID{inputID, inputID},
		programissuance.StageComputation, programissuance.InputPreviousStage,
		programschema.RuleOccurrenceRoute{}, false, programschema.RuleOccurrenceSource{},
	)
	if !ruleOK || rule.InputPointCount() != 2 {
		t.Fatalf("aliased ordinal inputs were not sealed: rule=%+v", rule)
	}
	first, firstOK := rule.InputPointAt(0)
	second, secondOK := rule.InputPointAt(1)
	if !firstOK || !secondOK || first != inputID || second != inputID {
		t.Fatalf("ordinal input roles lost their alias: first=%v/%t second=%v/%t", first, firstOK, second, secondOK)
	}
}

func TestRegionWTOIdentityFieldsPreserveMemberAndBracketOrder(t *testing.T) {
	regionID, memberID, pointID := genericIdentityID(41), genericIdentityID(42), genericIdentityID(43)
	region, regionOK := programschema.NewRegion(regionID, identity.ContentID{}, 0, 1, true)
	member, memberOK := programschema.NewRegionMember(memberID)
	event, eventOK := programschema.NewWTOEvent(programschema.WTOEventPoint, identity.ContentID{}, pointID)
	if !regionOK || !memberOK || !eventOK {
		t.Fatal("region/WTO rows")
	}
	program := genericIdentityProgram(t, programpublication.Publication{Regions: []programschema.Region{region}, RegionMembers: []programschema.RegionMember{member}, WTOEvents: []programschema.WTOEvent{event}})
	var got genericIdentityOperations
	if !program.WriteRegionWTOIdentityFields(&got) {
		t.Fatal("write region/WTO identity")
	}
	i := func(id identity.ContentID) genericIdentityOperation {
		return genericIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) genericIdentityOperation { return genericIdentityOperation{kind: 'u', value: value} }
	b := func(value bool) genericIdentityOperation {
		if value {
			return genericIdentityOperation{kind: 'b', value: 1}
		}
		return genericIdentityOperation{kind: 'b'}
	}
	want := genericIdentityOperations{u(1), i(regionID), i(identity.ContentID{}), b(true), u(1), i(memberID), u(1), u(uint64(programschema.WTOEventPoint)), i(identity.ContentID{}), i(pointID)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("region/WTO identity operations = %#v, want %#v", got, want)
	}
}

func TestGenericIdentityWritersRejectAnUnpublishedProgram(t *testing.T) {
	var operations genericIdentityOperations
	program := programschema.Program{}
	if program.WriteOccurrenceIdentityFields(&operations) || program.WriteSummaryIdentityFields(&operations) || program.WriteEnvironmentLocalTransferIdentityFields(&operations) || program.WriteRuleOccurrenceIdentityFields(&operations) || program.WriteRegionWTOIdentityFields(&operations) {
		t.Fatal("an unpublished Program wrote an identity preimage")
	}
}
