package expand

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/identity"
	arrangementexpand "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// Fixture carries the owner-issued declaration, committed target world, and
// expected logical identities for the bounded W3 Expand gate.
type Fixture struct {
	world                targetfixture.World
	contract             model.ExpandContract
	dependency           model.DependencyID
	mainScope            model.ScopeID
	otherScope           model.ScopeID
	typeID               model.TypeID
	readerMain           signature.Signature
	readerDelta          signature.Signature
	mixedDelta           signature.Signature
	readerDeltaWorker    *expandWorker
	mixedDeltaWorker     *expandWorker
	candidateDenominator model.DenominatorRef
	readerDenominator    model.DenominatorRef
	publisherDenominator model.DenominatorRef
	readerKeyID          model.KeyID

	candidate                model.RelationID
	publisher                model.RelationID
	reader                   model.RelationID
	foreignPublisherRelation model.RelationID

	candidateAddress     model.ColumnID
	candidatePayload     model.ColumnID
	readerKey            model.ColumnID
	readerPayload1Column model.ColumnID
	readerPayload2Column model.ColumnID

	candidateA model.RowID
	candidateB model.RowID
	publisherA model.RowID
	publisherB model.RowID
	readerA    model.RowID
	readerB    model.RowID
	readerC    model.RowID
	readerD    model.RowID

	key1 identity.ContentID
	key2 identity.ContentID
	key3 identity.ContentID
	key4 identity.ContentID

	candidateAddressA identity.ContentID
	candidateAddressB identity.ContentID
	candidatePayloadA identity.ContentID
	candidatePayloadB identity.ContentID
	readerPayload1A   identity.ContentID
	readerPayload1B   identity.ContentID
	readerPayload2A   identity.ContentID
	readerPayload2B   identity.ContentID
	readerPayload1D   identity.ContentID
	readerPayload2D   identity.ContentID
}

type expandIDs struct {
	schema                                                                                 model.SchemaID
	mainScope, otherScope                                                                  model.ScopeID
	typeID                                                                                 model.TypeID
	candidate, publisher, reader                                                           model.RelationID
	foreignPublisher                                                                       model.RelationID
	candidateAddress, candidatePayload                                                     model.ColumnID
	publisherAddress, publisherPayload                                                     model.ColumnID
	readerKey, readerPayload1, readerPayload2                                              model.ColumnID
	candidateKey, publisherKey, readerKeyID                                                model.KeyID
	candidateSeed, publisherSeed, readerMainSeed, readerOtherSeed, readerDelta, mixedDelta model.OperationID
	expression                                                                             model.ExpressionID
	dependency                                                                             model.DependencyID
}

// New constructs the real relcompiled Expand world and freezes its owner
// supplied C→P/key vectors into the target mount.
func New(t targetfixture.Probe) Fixture {
	t.Helper()
	owner := targetfixture.NewIdentity(t, "relation-expand-w3")
	ids := expandIDs{
		schema: owner.Schema(t, "expand"), mainScope: owner.Scope(t, "main"), otherScope: owner.Scope(t, "other"),
		typeID: owner.Type(t, "opaque"), candidate: owner.Relation(t, "candidate"), publisher: owner.Relation(t, "publisher"), reader: owner.Relation(t, "reader"), foreignPublisher: owner.Relation(t, "foreign-publisher"),
		candidateSeed: owner.Operation(t, "seed-candidate"), publisherSeed: owner.Operation(t, "seed-publisher"), readerMainSeed: owner.Operation(t, "seed-reader-main"), readerOtherSeed: owner.Operation(t, "seed-reader-other"), readerDelta: owner.Operation(t, "reader-delta"), mixedDelta: owner.Operation(t, "mixed-delta"),
		expression: owner.Expression(t, "expand"), dependency: owner.Dependency(t, "expand"),
	}
	ids.candidateAddress = owner.Column(t, ids.candidate, "address")
	ids.candidatePayload = owner.Column(t, ids.candidate, "payload")
	ids.publisherAddress = owner.Column(t, ids.publisher, "address")
	ids.publisherPayload = owner.Column(t, ids.publisher, "payload")
	ids.readerKey = owner.Column(t, ids.reader, "key")
	ids.readerPayload1 = owner.Column(t, ids.reader, "payload-1")
	ids.readerPayload2 = owner.Column(t, ids.reader, "payload-2")
	ids.candidateKey = owner.Key(t, ids.candidate, "address")
	ids.publisherKey = owner.Key(t, ids.publisher, "address")
	ids.readerKeyID = owner.Key(t, ids.reader, "key")

	idsCandidateA := owner.Row(t, ids.candidate, "candidate-a")
	idsCandidateB := owner.Row(t, ids.candidate, "candidate-b")
	idsPublisherA := owner.Row(t, ids.publisher, "publisher-a")
	idsPublisherB := owner.Row(t, ids.publisher, "publisher-b")
	idsReaderA := owner.Row(t, ids.reader, "reader-a")
	idsReaderB := owner.Row(t, ids.reader, "reader-b")
	idsReaderC := owner.Row(t, ids.reader, "reader-c")
	idsReaderD := owner.Row(t, ids.reader, "reader-d")
	key1 := ownerContent(t, owner, "key-1")
	key2 := ownerContent(t, owner, "key-2")
	key3 := ownerContent(t, owner, "key-3")
	key4 := ownerContent(t, owner, "key-4")
	contract := model.DefineExpandContract(ids.candidate, ids.publisher, ids.reader, ids.readerKey, ids.publisher).WithScope(ids.mainScope)

	candidateDenominator := mustDenominator(t, ids.candidate, ids.candidateKey)
	publisherDenominator := mustDenominator(t, ids.publisher, ids.publisherKey)
	readerDenominator := mustDenominator(t, ids.reader, ids.readerKeyID)
	candidateSeed := seedSignature(t, owner.Owner(), ids.schema, ids.candidateSeed, []signature.Output{
		{Relation: ids.candidate, Column: ids.candidateAddress, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: candidateDenominator},
		{Relation: ids.candidate, Column: ids.candidatePayload, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: candidateDenominator},
	})
	publisherSeed := seedSignature(t, owner.Owner(), ids.schema, ids.publisherSeed, []signature.Output{
		{Relation: ids.publisher, Column: ids.publisherAddress, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: publisherDenominator},
		{Relation: ids.publisher, Column: ids.publisherPayload, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: publisherDenominator},
	})
	readerMainSeed := seedSignature(t, owner.Owner(), ids.schema, ids.readerMainSeed, []signature.Output{
		{Relation: ids.reader, Column: ids.readerKey, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload1, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload2, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
	})
	readerDelta := seedSignature(t, owner.Owner(), ids.schema, ids.readerDelta, []signature.Output{
		{Relation: ids.reader, Column: ids.readerKey, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload1, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload2, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
	})
	mixedDelta := seedSignature(t, owner.Owner(), ids.schema, ids.mixedDelta, []signature.Output{
		{Relation: ids.candidate, Column: ids.candidatePayload, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: candidateDenominator},
		{Relation: ids.reader, Column: ids.readerPayload1, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
	})
	readerDeltaWorker := &expandWorker{}
	mixedDeltaWorker := &expandWorker{}
	readerOtherSeed := seedSignature(t, owner.Owner(), ids.schema, ids.readerOtherSeed, []signature.Output{
		{Relation: ids.reader, Column: ids.readerKey, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload1, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
		{Relation: ids.reader, Column: ids.readerPayload2, Type: ids.typeID, Presence: signature.ProduceOpaque, Denominator: readerDenominator},
	})

	equatable, ok := model.NewEquatableCapability(ids.typeID)
	if !ok {
		t.Fatal("Expand equality capability")
	}
	mainRegion := ownerScopeRegion(t, owner, "scope-region-main")
	otherRegion := ownerScopeRegion(t, owner, "scope-region-other")
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.candidate, []model.ColumnID{ids.candidateAddress, ids.candidatePayload}, []model.KeyID{ids.candidateKey}, ids.mainScope),
			model.DefineRelationSchema(ids.publisher, []model.ColumnID{ids.publisherAddress, ids.publisherPayload}, []model.KeyID{ids.publisherKey}, ids.mainScope),
			model.DefineRelationSchema(ids.reader, []model.ColumnID{ids.readerKey, ids.readerPayload1, ids.readerPayload2}, []model.KeyID{ids.readerKeyID}, ids.mainScope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.candidateAddress, ids.typeID), model.DefineColumnSchema(ids.candidatePayload, ids.typeID),
			model.DefineColumnSchema(ids.publisherAddress, ids.typeID), model.DefineColumnSchema(ids.publisherPayload, ids.typeID),
			model.DefineColumnSchema(ids.readerKey, ids.typeID), model.DefineColumnSchema(ids.readerPayload1, ids.typeID), model.DefineColumnSchema(ids.readerPayload2, ids.typeID),
		},
		TypeCapabilities: []model.TypeCapability{equatable},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.candidateKey, []model.ColumnID{ids.candidateAddress}),
			model.DefineKeySchema(ids.publisherKey, []model.ColumnID{ids.publisherAddress}),
			model.DefineKeySchema(ids.readerKeyID, []model.ColumnID{ids.readerKey}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.mainScope, nil, mainRegion), model.DefineScopeSchema(ids.otherScope, nil, otherRegion)},
		Signatures: []signature.Signature{candidateSeed, publisherSeed, readerMainSeed, readerOtherSeed, readerDelta, mixedDelta},
		Rules:      []relcompile.Rule{{ID: ids.dependency, Expression: ids.expression, Candidate: ids.candidate, Joins: []relcompile.JoinSpec{{Relation: ids.reader, Scope: ids.mainScope, Expand: &contract}}}},
	}

	addressA, addressB := ownerContent(t, owner, "candidate-address-a"), ownerContent(t, owner, "candidate-address-b")
	candidatePayloadA, candidatePayloadB := ownerContent(t, owner, "candidate-payload-a"), ownerContent(t, owner, "candidate-payload-b")
	readerPayload1A, readerPayload1B := ownerContent(t, owner, "reader-payload-1-a"), ownerContent(t, owner, "reader-payload-1-b")
	readerPayload2A, readerPayload2B := ownerContent(t, owner, "reader-payload-2-a"), ownerContent(t, owner, "reader-payload-2-b")
	readerPayload1C, readerPayload2C := ownerContent(t, owner, "reader-payload-1-c"), ownerContent(t, owner, "reader-payload-2-c")
	readerPayload1D, readerPayload2D := ownerContent(t, owner, "reader-payload-1-d"), ownerContent(t, owner, "reader-payload-2-d")

	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    owner,
		Declaration: declaration,
		Bindings:    []binding.Factory{newExpandFactory(readerDelta, readerDeltaWorker), newExpandFactory(mixedDelta, mixedDeltaWorker)},
		Populations: []targetfixture.Population{
			{Denominator: candidateDenominator, Rows: []model.RowID{idsCandidateA, idsCandidateB}},
			{Denominator: publisherDenominator, Rows: []model.RowID{idsPublisherA, idsPublisherB}},
			{Denominator: readerDenominator, Rows: []model.RowID{idsReaderA, idsReaderB, idsReaderC, idsReaderD}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.mainScope, Region: "main"}, {ID: ids.otherScope, Region: "other"}},
		Initials: []targetfixture.Initial{
			{Operation: candidateSeed, Scope: ids.mainScope, Cells: seedCells(ids.typeID, candidateDenominator, []seedRow{
				{row: idsCandidateA, columns: []model.ColumnID{ids.candidateAddress, ids.candidatePayload}, values: []identity.ContentID{addressA, candidatePayloadA}},
				{row: idsCandidateB, columns: []model.ColumnID{ids.candidateAddress, ids.candidatePayload}, values: []identity.ContentID{addressB, candidatePayloadB}},
			})},
			{Operation: publisherSeed, Scope: ids.mainScope, Cells: seedCells(ids.typeID, publisherDenominator, []seedRow{
				{row: idsPublisherA, columns: []model.ColumnID{ids.publisherAddress, ids.publisherPayload}, values: []identity.ContentID{ownerContent(t, owner, "publisher-address-a"), ownerContent(t, owner, "publisher-payload-a")}},
				{row: idsPublisherB, columns: []model.ColumnID{ids.publisherAddress, ids.publisherPayload}, values: []identity.ContentID{ownerContent(t, owner, "publisher-address-b"), ownerContent(t, owner, "publisher-payload-b")}},
			})},
			{Operation: readerMainSeed, Scope: ids.mainScope, Cells: seedCells(ids.typeID, readerDenominator, []seedRow{
				{row: idsReaderA, columns: []model.ColumnID{ids.readerKey, ids.readerPayload1, ids.readerPayload2}, values: []identity.ContentID{key1, readerPayload1A, readerPayload2A}},
				{row: idsReaderB, columns: []model.ColumnID{ids.readerKey, ids.readerPayload1, ids.readerPayload2}, values: []identity.ContentID{key2, readerPayload1B, readerPayload2B}},
			})},
			{Operation: readerOtherSeed, Scope: ids.otherScope, Cells: seedCells(ids.typeID, readerDenominator, []seedRow{
				{row: idsReaderC, columns: []model.ColumnID{ids.readerKey, ids.readerPayload1, ids.readerPayload2}, values: []identity.ContentID{key3, readerPayload1C, readerPayload2C}},
				{row: idsReaderD, columns: []model.ColumnID{ids.readerKey, ids.readerPayload1, ids.readerPayload2}, values: []identity.ContentID{key4, readerPayload1D, readerPayload2D}},
			})},
		},
		Authorities: func(binding.Issuer) (targetfixture.Registry, bool) {
			return targetfixture.Registry{Equalities: []binding.ValueEquality{tokenEquality{typeID: ids.typeID}}}, true
		},
		ResolveExpand: func(value model.ExpandContract) ([]arrangementexpand.Vector, bool) {
			if value != contract {
				return nil, false
			}
			first, firstOK := arrangementexpand.NewVector(idsCandidateA, idsPublisherA, []identity.ContentID{key2, key3, key1})
			second, secondOK := arrangementexpand.NewVector(idsCandidateB, idsPublisherB, []identity.ContentID{key1})
			return []arrangementexpand.Vector{first, second}, firstOK && secondOK
		},
		MountByte: 0xE7,
	})

	return Fixture{
		world: world, contract: contract, dependency: ids.dependency, mainScope: ids.mainScope, otherScope: ids.otherScope, typeID: ids.typeID,
		readerMain: readerMainSeed, readerDelta: readerDelta, mixedDelta: mixedDelta, readerDeltaWorker: readerDeltaWorker, mixedDeltaWorker: mixedDeltaWorker, candidateDenominator: candidateDenominator, readerDenominator: readerDenominator, publisherDenominator: publisherDenominator, readerKeyID: ids.readerKeyID,
		candidate: ids.candidate, publisher: ids.publisher, reader: ids.reader, foreignPublisherRelation: ids.foreignPublisher,
		candidateAddress: ids.candidateAddress, candidatePayload: ids.candidatePayload, readerKey: ids.readerKey, readerPayload1Column: ids.readerPayload1, readerPayload2Column: ids.readerPayload2,
		candidateA: idsCandidateA, candidateB: idsCandidateB, publisherA: idsPublisherA, publisherB: idsPublisherB, readerA: idsReaderA, readerB: idsReaderB, readerC: idsReaderC, readerD: idsReaderD,
		key1: key1, key2: key2, key3: key3, key4: key4,
		candidateAddressA: addressA, candidateAddressB: addressB, candidatePayloadA: candidatePayloadA, candidatePayloadB: candidatePayloadB,
		readerPayload1A: readerPayload1A, readerPayload1B: readerPayload1B, readerPayload2A: readerPayload2A, readerPayload2B: readerPayload2B,
		readerPayload1D: readerPayload1D, readerPayload2D: readerPayload2D,
	}
}

type seedRow struct {
	row     model.RowID
	columns []model.ColumnID
	values  []identity.ContentID
}

func seedCells(typeID model.TypeID, denominator model.DenominatorRef, rows []seedRow) func(binding.Issuer) ([]targetfixture.Cell, bool) {
	return func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
		if !issuer.Available() || rows == nil {
			return nil, false
		}
		result := make([]targetfixture.Cell, 0)
		for _, row := range rows {
			if len(row.columns) != len(row.values) {
				return nil, false
			}
			for index, column := range row.columns {
				token, ok := issuer.IssueValue(typeID, row.values[index])
				if !ok {
					return nil, false
				}
				cell, ok := targetfixture.Opaque(denominator, row.row, column, token)
				if !ok {
					return nil, false
				}
				result = append(result, cell)
			}
		}
		return result, true
	}
}

func ownerContent(t targetfixture.Probe, owner targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := owner.Content(label)
	if !ok {
		t.Fatalf("content %s", label)
	}
	return value
}

func ownerScopeRegion(t targetfixture.Probe, owner targetfixture.Identity, label string) region.Region {
	t.Helper()
	atomID := ownerContent(t, owner, label)
	atom, ok := region.NewAtom(atomID)
	if !ok {
		t.Fatalf("scope atom %s", label)
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatalf("scope region %s", label)
	}
	return value
}

func mustDenominator(t targetfixture.Probe, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return value
}

func seedSignature(t targetfixture.Probe, owner model.OwnerID, schema model.SchemaID, operation model.OperationID, outputs []signature.Output) signature.Signature {
	t.Helper()
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	cardinality, ok := model.NewCardinality(model.BoundedMany, 3)
	if !ok {
		t.Fatal("cardinality")
	}
	value, ok := signature.Seal(signature.Spec{Identity: signature.Identity{Operation: operation, Version: 1}, Fence: signature.Fence{Owner: owner, Schema: schema}, Outputs: outputs, Cardinality: cardinality, Outcomes: outcomes})
	if !ok {
		t.Fatal("seed signature")
	}
	return value
}

// expandWorker is a fixture-owned semantic worker for the two post-bootstrap
// mutation operations. The proposal buffer remains the authority for output
// destination/value validation; this worker only replays the owner-issued
// proposal vector selected by the mutation method.
type expandWorker struct {
	proposals []binding.Proposal
}

func (worker *expandWorker) Evaluate(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if worker == nil || buffer == nil {
		return outcome.Result{}
	}
	for _, proposal := range worker.proposals {
		if !buffer.Append(proposal) {
			return outcome.Result{}
		}
	}
	return outcome.Result{Code: outcome.Produced}
}

type expandFactory struct {
	operation signature.Signature
	worker    *expandWorker
}

func newExpandFactory(operation signature.Signature, worker *expandWorker) binding.Factory {
	return expandFactory{operation: operation, worker: worker}
}

func (factory expandFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || !factory.operation.Available() || operation.Digest() != factory.operation.Digest() || factory.worker == nil {
		return nil, false
	}
	return expandBinding{operation: factory.operation, worker: factory.worker}, true
}

type expandBinding struct {
	operation signature.Signature
	worker    *expandWorker
}

func (bindingValue expandBinding) Signature() signature.Signature { return bindingValue.operation }

func (bindingValue expandBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return bindingValue.worker, bindingValue.worker != nil
}

type tokenEquality struct{ typeID model.TypeID }

func (value tokenEquality) Type() model.TypeID { return value.typeID }

func (value tokenEquality) Equal(left, right binding.ValueToken) bool {
	return left.Available() && right.Available() && left.Type() == value.typeID && right.Type() == value.typeID && left.Opaque() == right.Opaque()
}

func (fixture Fixture) World() targetfixture.World     { return fixture.world }
func (fixture Fixture) Contract() model.ExpandContract { return fixture.contract }
func (fixture Fixture) Dependency() model.DependencyID { return fixture.dependency }
func (fixture Fixture) MainScope() model.ScopeID       { return fixture.mainScope }
func (fixture Fixture) OtherScope() model.ScopeID      { return fixture.otherScope }
func (fixture Fixture) TypeID() model.TypeID           { return fixture.typeID }
func (fixture Fixture) ReaderKeyID() model.KeyID       { return fixture.readerKeyID }
func (fixture Fixture) ReaderDenominator() model.DenominatorRef {
	return fixture.readerDenominator
}
func (fixture Fixture) ReaderRelation() model.RelationID     { return fixture.reader }
func (fixture Fixture) CandidateAddress() model.ColumnID     { return fixture.candidateAddress }
func (fixture Fixture) CandidatePayload() model.ColumnID     { return fixture.candidatePayload }
func (fixture Fixture) ReaderKey() model.ColumnID            { return fixture.readerKey }
func (fixture Fixture) ReaderPayload1Column() model.ColumnID { return fixture.readerPayload1Column }
func (fixture Fixture) ReaderPayload2Column() model.ColumnID { return fixture.readerPayload2Column }
func (fixture Fixture) CandidateA() model.RowID              { return fixture.candidateA }
func (fixture Fixture) CandidateB() model.RowID              { return fixture.candidateB }
func (fixture Fixture) PublisherA() model.RowID              { return fixture.publisherA }
func (fixture Fixture) ForeignPublisherRelation() model.RelationID {
	return fixture.foreignPublisherRelation
}
func (fixture Fixture) Key1() identity.ContentID { return fixture.key1 }
func (fixture Fixture) Key2() identity.ContentID { return fixture.key2 }
func (fixture Fixture) Key3() identity.ContentID { return fixture.key3 }
func (fixture Fixture) Key4() identity.ContentID { return fixture.key4 }

func (fixture Fixture) ReaderRowFor(key identity.ContentID) model.RowID {
	switch key {
	case fixture.key1:
		return fixture.readerA
	case fixture.key2:
		return fixture.readerB
	default:
		return fixture.readerC
	}
}

func (fixture Fixture) CandidateAddressValue(row model.RowID) identity.ContentID {
	if row == fixture.candidateA {
		return fixture.candidateAddressA
	}
	return fixture.candidateAddressB
}

func (fixture Fixture) CandidatePayloadValue(row model.RowID) identity.ContentID {
	if row == fixture.candidateA {
		return fixture.candidatePayloadA
	}
	return fixture.candidatePayloadB
}

func (fixture Fixture) ReaderPayload1For(key identity.ContentID) identity.ContentID {
	if key == fixture.key1 {
		return fixture.readerPayload1A
	}
	return fixture.readerPayload1B
}

func (fixture Fixture) ReaderPayload2For(key identity.ContentID) identity.ContentID {
	if key == fixture.key1 {
		return fixture.readerPayload2A
	}
	return fixture.readerPayload2B
}

// ReaderD is a mounted reader row whose key is intentionally absent from all
// owner C→R vectors. It is useful for proving authenticated unrelated-key
// emptiness without introducing a synthetic row identity in a law.
func (fixture Fixture) ReaderD() model.RowID { return fixture.readerD }

// ReaderPayloadLineageDelta republishes the existing reader-a/payload-1
// token with a strictly different, mounted lineage witness.  The R key and
// opaque payload remain byte-for-byte compatible: this is the only real
// successor the decode-only fixture can publish without inventing a lattice
// authority.  The transition is staged through the production proposal,
// transaction, and aggregate commit doors; no store/index root is fabricated.
func (fixture Fixture) ReaderPayloadLineageDelta() (database.Delta, bool) {
	return fixture.readerPayloadLineageDelta(fixture.mainScope, fixture.readerA, fixture.readerPayload1A)
}

// UnrelatedReaderPayloadLineageDelta republishes a real R row whose key is
// absent from the frozen owner vectors. It changes only that row's payload
// lineage, so Expand must return an authenticated empty without scanning R.
func (fixture Fixture) UnrelatedReaderPayloadLineageDelta() (database.Delta, bool) {
	return fixture.readerPayloadLineageDelta(fixture.otherScope, fixture.readerD, fixture.readerPayload1D)
}

// MixedCandidateReaderDelta publishes one real positive C+R transition in a
// single database successor. Both rows keep their opaque values while their
// authenticated lineage advances. This is the fixture transition for the
// canonical Expand pivot: the union of candidates affected through C or R is
// recomputed once from the sealed successor views.
func (fixture Fixture) MixedCandidateReaderDelta() (database.Delta, bool) {
	if !fixture.world.Mounted().Available() || !fixture.world.View().ValidFor(fixture.world.Mounted()) || !fixture.world.Base().Available() || !fixture.readerMain.Available() || !fixture.candidateDenominator.Available() || !fixture.readerDenominator.Available() || !fixture.publisherDenominator.Available() {
		return database.Delta{}, false
	}
	mounted := fixture.world.Mounted()
	view := fixture.world.View()
	base := fixture.world.Base()
	issuer, ok := binding.NewIssuer(mounted.RuntimeFence())
	if !ok {
		return database.Delta{}, false
	}
	scope, ok := mounted.Scope(fixture.mainScope)
	if !ok || !scope.Available() {
		return database.Delta{}, false
	}
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok || !scopeToken.Available() {
		return database.Delta{}, false
	}
	candidateWitness, ok := mounted.Denominator(fixture.candidateDenominator)
	if !ok || !candidateWitness.Available() {
		return database.Delta{}, false
	}
	readerWitness, ok := mounted.Denominator(fixture.readerDenominator)
	if !ok || !readerWitness.Available() {
		return database.Delta{}, false
	}
	candidateDestination, ok := issuer.IssueCell(candidateWitness, scopeToken, fixture.candidatePayload, fixture.candidateA)
	if !ok {
		return database.Delta{}, false
	}
	candidateValue, ok := issuer.IssueValue(fixture.typeID, fixture.candidatePayloadA)
	if !ok {
		return database.Delta{}, false
	}
	readerDestination, ok := issuer.IssueCell(readerWitness, scopeToken, fixture.readerPayload1Column, fixture.readerA)
	if !ok {
		return database.Delta{}, false
	}
	readerValue, ok := issuer.IssueValue(fixture.typeID, fixture.readerPayload1A)
	if !ok {
		return database.Delta{}, false
	}
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		return database.Delta{}, false
	}
	candidateProposal, ok := binding.NewProposal(candidateDestination, candidateValue, presence)
	if !ok {
		return database.Delta{}, false
	}
	readerProposal, ok := binding.NewProposal(readerDestination, readerValue, presence)
	if !ok {
		return database.Delta{}, false
	}
	if !fixture.mixedDelta.Available() || fixture.mixedDeltaWorker == nil {
		return database.Delta{}, false
	}
	fixture.mixedDeltaWorker.proposals = []binding.Proposal{candidateProposal, readerProposal}
	readerLineage, ok := mounted.DenominatorLineage(fixture.readerDenominator)
	if !ok {
		return database.Delta{}, false
	}
	candidateLineage, ok := mounted.DenominatorLineage(fixture.candidateDenominator)
	if !ok {
		return database.Delta{}, false
	}
	publisherLineage, ok := mounted.DenominatorLineage(fixture.publisherDenominator)
	if !ok {
		return database.Delta{}, false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return database.Delta{}, false
	}
	lineage, ok := lineageAuthority.Join(candidateLineage, readerLineage)
	if !ok || !lineage.Available() {
		return database.Delta{}, false
	}
	lineage, ok = lineageAuthority.Join(lineage, publisherLineage)
	if !ok || !lineage.Available() {
		return database.Delta{}, false
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		return database.Delta{}, false
	}
	application, ok := apply.Apply(mounted, fixture.mixedDelta.Identity(), scope, lineage, binding.NewOwnerNamedDestination(candidateWitness.Relation()))
	if !ok || !application.Available() || application.Len() != 2 {
		return database.Delta{}, false
	}
	batch, ok := transaction.NewSubmissionBatch(application, witness.WideningPermit{}, nil)
	if !ok {
		return database.Delta{}, false
	}
	prepared, ok := transaction.Prepare(base, view, scratch, batch)
	if !ok || !prepared.Available() {
		return database.Delta{}, false
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !next.Available() || !delta.Available() || !delta.Base().Same(base) || !delta.Next().Same(next) {
		return database.Delta{}, false
	}
	return delta, true
}

func (fixture Fixture) readerPayloadLineageDelta(scopeID model.ScopeID, row model.RowID, payload identity.ContentID) (database.Delta, bool) {
	if !fixture.world.Mounted().Available() || !fixture.world.View().ValidFor(fixture.world.Mounted()) || !fixture.world.Base().Available() || !fixture.readerMain.Available() || !fixture.readerDenominator.Available() || !fixture.publisherDenominator.Available() {
		return database.Delta{}, false
	}
	mounted := fixture.world.Mounted()
	view := fixture.world.View()
	base := fixture.world.Base()
	issuer, ok := binding.NewIssuer(mounted.RuntimeFence())
	if !ok {
		return database.Delta{}, false
	}
	scope, ok := mounted.Scope(scopeID)
	if !ok || !scope.Available() {
		return database.Delta{}, false
	}
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok || !scopeToken.Available() {
		return database.Delta{}, false
	}
	readerWitness, ok := mounted.Denominator(fixture.readerDenominator)
	if !ok || !readerWitness.Available() {
		return database.Delta{}, false
	}
	destination, ok := issuer.IssueCell(readerWitness, scopeToken, fixture.readerPayload1Column, row)
	if !ok {
		return database.Delta{}, false
	}
	value, ok := issuer.IssueValue(fixture.typeID, payload)
	if !ok {
		return database.Delta{}, false
	}
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		return database.Delta{}, false
	}
	proposal, ok := binding.NewProposal(destination, value, presence)
	if !ok {
		return database.Delta{}, false
	}
	if !fixture.readerDelta.Available() || fixture.readerDeltaWorker == nil {
		return database.Delta{}, false
	}
	fixture.readerDeltaWorker.proposals = []binding.Proposal{proposal}
	readerLineage, ok := mounted.DenominatorLineage(fixture.readerDenominator)
	if !ok {
		return database.Delta{}, false
	}
	publisherLineage, ok := mounted.DenominatorLineage(fixture.publisherDenominator)
	if !ok {
		return database.Delta{}, false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return database.Delta{}, false
	}
	lineage, ok := lineageAuthority.Join(readerLineage, publisherLineage)
	if !ok || !lineage.Available() || lineage == readerLineage {
		return database.Delta{}, false
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		return database.Delta{}, false
	}
	application, ok := apply.Apply(mounted, fixture.readerDelta.Identity(), scope, lineage, binding.NewOwnerNamedDestination(readerWitness.Relation()))
	if !ok || !application.Available() || application.Len() != 1 {
		return database.Delta{}, false
	}
	batch, ok := transaction.NewSubmissionBatch(application, witness.WideningPermit{}, nil)
	if !ok {
		return database.Delta{}, false
	}
	prepared, ok := transaction.Prepare(base, view, scratch, batch)
	if !ok || !prepared.Available() {
		return database.Delta{}, false
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !next.Available() || !delta.Available() || !delta.Base().Same(base) || !delta.Next().Same(next) {
		return database.Delta{}, false
	}
	return delta, true
}
