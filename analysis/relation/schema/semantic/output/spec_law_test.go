package output_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type contributionFixture struct {
	owner     model.OwnerID
	operation model.OperationID
	relation  model.RelationID
	column    model.ColumnID
	key       model.KeyID
	typeID    model.TypeID
	algebra   model.TypeCapability
	signature signature.Signature
	port      output.OutputPort
}

func newContributionFixture(t *testing.T) contributionFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(token("owner"))
	if !ok {
		t.Fatal("owner")
	}
	operation, ok := model.IssueOperationID(owner, token("operation"))
	if !ok {
		t.Fatal("operation")
	}
	relation, ok := model.IssueRelationID(owner, token("relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, token("column"))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, token("key"))
	if !ok {
		t.Fatal("key")
	}
	typeID, ok := model.IssueTypeID(owner, token("type"))
	if !ok {
		t.Fatal("type")
	}
	algebra, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("algebra")
	}
	schemaID, ok := model.IssueSchemaID(owner, token("schema"))
	if !ok {
		t.Fatal("schema")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	identityValue := signature.Identity{Operation: operation, Version: 1}
	sealedSignature, ok := signature.Seal(signature.Spec{
		Identity: identityValue,
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Outputs: []signature.Output{{
			Relation: relation, Column: column, Type: typeID,
			Presence: signature.ProducePresent, Denominator: denominator,
		}},
		Cardinality: cardinality,
	})
	if !ok {
		t.Fatal("signature")
	}
	return contributionFixture{
		owner: owner, operation: operation, relation: relation, column: column,
		key: key, typeID: typeID, algebra: algebra, signature: sealedSignature,
		port: output.OutputPort{Operation: identityValue, Column: column},
	}
}

func (value contributionFixture) spec(reducer output.ReducerKind) output.Spec {
	return output.Spec{
		Signature: value.signature,
		Port:      value.port,
		ValueType: value.typeID,
		Algebra:   value.algebra,
		Reducer:   reducer,
	}
}

func token(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("analysis/relation/schema/semantic/output/law/v2", []byte(label))
	if !ok {
		panic("unable to derive test token")
	}
	return value
}

func TestContributionSpecSealsStructuralPortAndCapability(t *testing.T) {
	value := newContributionFixture(t)
	spec, ok := output.Seal(value.spec(output.Contributions))
	if !ok || !spec.Available() || !spec.Digest().Available() {
		t.Fatal("valid contribution did not seal")
	}
	if spec.Port() != value.port || spec.Column() != value.column ||
		spec.Presence() != signature.ProducePresent ||
		spec.ValueType() != value.typeID || !spec.Algebra().Equal(value.algebra) ||
		spec.Reducer() != output.Contributions {
		t.Fatal("sealed structural or presence fields changed")
	}
}

func TestContributionSpecUsesSignatureOutputForAndRejectsInventedPort(t *testing.T) {
	value := newContributionFixture(t)
	foreignColumn, ok := model.IssueColumnID(value.relation, token("undeclared-column"))
	if !ok {
		t.Fatal("foreign column")
	}
	wrongPort := value.spec(output.Contributions)
	wrongPort.Port.Column = foreignColumn
	if sealed, ok := output.Seal(wrongPort); ok || sealed.Available() {
		t.Fatal("undeclared output column was admitted")
	}

	wrongOperation := value.spec(output.Contributions)
	foreignOperation, ok := model.IssueOperationID(value.owner, token("other-operation"))
	if !ok {
		t.Fatal("other operation")
	}
	wrongOperation.Port.Operation = signature.Identity{Operation: foreignOperation, Version: 1}
	if sealed, ok := output.Seal(wrongOperation); ok || sealed.Available() {
		t.Fatal("foreign output identity was admitted")
	}
}

func TestContributionSpecRejectsMissingForeignAndFakeUniqueReducer(t *testing.T) {
	value := newContributionFixture(t)
	missingSignature := value.spec(output.Contributions)
	missingSignature.Signature = signature.Signature{}
	if sealed, ok := output.Seal(missingSignature); ok || sealed.Available() {
		t.Fatal("missing signature was admitted")
	}
	missingFence, ok := signature.Seal(signature.Spec{
		Identity: value.signature.Identity(),
		Outputs:  value.signature.Outputs(),
	})
	if !ok {
		t.Fatal("signature with missing fence")
	}
	missingFenceSpec := value.spec(output.Contributions)
	missingFenceSpec.Signature = missingFence
	if sealed, ok := output.Seal(missingFenceSpec); ok || sealed.Available() {
		t.Fatal("signature with missing fence was admitted")
	}

	foreignOwner, ok := model.IssueOwnerID(token("foreign-owner"))
	if !ok {
		t.Fatal("foreign owner")
	}
	foreignType, ok := model.IssueTypeID(foreignOwner, token("foreign-type"))
	if !ok {
		t.Fatal("foreign type")
	}
	foreignAlgebra, ok := model.NewAscendingCapability(foreignType)
	if !ok {
		t.Fatal("foreign algebra")
	}
	foreign := value.spec(output.Contributions)
	foreign.ValueType, foreign.Algebra = foreignType, foreignAlgebra
	if sealed, ok := output.Seal(foreign); ok || sealed.Available() {
		t.Fatal("foreign value authority was admitted")
	}

	fakeUnique := value.spec(output.ReducerKind(2))
	if sealed, ok := output.Seal(fakeUnique); ok || sealed.Available() {
		t.Fatal("fake Unique reducer was admitted")
	}
}

func TestContributionSpecDigestCoversCapabilityAndPort(t *testing.T) {
	value := newContributionFixture(t)
	first, ok := output.Seal(value.spec(output.Contributions))
	if !ok {
		t.Fatal("first contribution")
	}
	decodeOnly, ok := model.NewDecodeOnlyCapability(value.typeID)
	if !ok {
		t.Fatal("decode-only capability")
	}
	changedCapability := value.spec(output.Contributions)
	changedCapability.Algebra = decodeOnly
	second, ok := output.Seal(changedCapability)
	if !ok || first.Digest() == second.Digest() {
		t.Fatal("capability change did not change digest")
	}

	otherColumn, ok := model.IssueColumnID(value.relation, token("other-column"))
	if !ok {
		t.Fatal("other column")
	}
	otherSignature, ok := signature.Seal(signature.Spec{
		Identity: value.signature.Identity(),
		Fence:    value.signature.Fence(),
		Outputs: []signature.Output{{
			Relation: value.relation, Column: otherColumn, Type: value.typeID,
			Presence:    signature.ProducePresent,
			Denominator: value.signature.Outputs()[0].Denominator,
		}},
		Cardinality: value.signature.Cardinality(),
	})
	if !ok {
		t.Fatal("other signature")
	}
	changedPort := output.Spec{
		Signature: otherSignature,
		Port: output.OutputPort{
			Operation: value.signature.Identity(), Column: otherColumn,
		},
		ValueType: value.typeID, Algebra: value.algebra, Reducer: output.Contributions,
	}
	third, ok := output.Seal(changedPort)
	if !ok || first.Digest() == third.Digest() {
		t.Fatal("port change did not change digest")
	}
}

func TestContributionSpecRetainsDeclaredPresenceAndDigestsIt(t *testing.T) {
	value := newContributionFixture(t)
	first, ok := output.Seal(value.spec(output.Contributions))
	if !ok || first.Presence() != signature.ProducePresent {
		t.Fatal("present signature presence was not retained exactly")
	}
	outputs := value.signature.Outputs()
	outputs[0].Presence = signature.ProduceOpaque
	opaqueSignature, ok := signature.Seal(signature.Spec{
		Identity:    value.signature.Identity(),
		Fence:       value.signature.Fence(),
		Outputs:     outputs,
		Cardinality: value.signature.Cardinality(),
	})
	if !ok {
		t.Fatal("opaque signature")
	}
	opaque := value.spec(output.Contributions)
	opaque.Signature = opaqueSignature
	second, ok := output.Seal(opaque)
	if !ok || second.Presence() != signature.ProduceOpaque {
		t.Fatal("opaque signature presence was not retained exactly")
	}
	if first.Digest() == second.Digest() || first.Equal(second) {
		t.Fatal("presence contract did not participate in contribution identity")
	}
}

func TestContributionSpecPreservesNegativePresenceWithoutInferringAbsence(t *testing.T) {
	value := newContributionFixture(t)
	var prior output.ContributionSpec
	for _, presence := range []signature.PresenceContract{signature.ProduceOptional, signature.ProduceAbsent} {
		outputs := value.signature.Outputs()
		outputs[0].Presence = presence
		semantic, ok := signature.Seal(signature.Spec{
			Identity:    value.signature.Identity(),
			Fence:       value.signature.Fence(),
			Outputs:     outputs,
			Cardinality: value.signature.Cardinality(),
		})
		if !ok {
			t.Fatalf("signature for %v", presence)
		}
		spec := value.spec(output.Contributions)
		spec.Signature = semantic
		sealed, ok := output.Seal(spec)
		if !ok || sealed.Presence() != presence {
			t.Fatalf("presence %v was not retained exactly", presence)
		}
		if prior.Available() && prior.Digest() == sealed.Digest() {
			t.Fatalf("presence %v did not change digest", presence)
		}
		prior = sealed
	}
}
