package cofiber_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber/lower"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/relationadmission"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	checktyping "github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

const admissionLawDomain = "analysis/program/relationadmission/law/v1"

// TestArithmeticAdmissionBuildsOneReadySolveWorld proves the complete new
// composition seam.  The source fact is admitted only as a zero-input owner
// publication; Solve then redeems the ordinary declared arithmetic rule and
// commits its result through the existing runtime.
func TestArithmeticAdmissionBuildsOneReadySolveWorld(t *testing.T) {
	specimen := newArithmeticSpecimen(t)
	ready, refusal := relationadmission.Admit(specimen.input(t))
	if refusal != nil || !ready.Available() {
		if refusal == nil {
			t.Fatal("admission returned an unavailable ready artifact")
		}
		t.Fatalf("admission refused: %v issues=%+v", refusal, refusal.CertificateIssues())
	}
	if !ready.Mounted().Available() || !ready.Geometry().ValidFor(ready.Mounted()) || !ready.Base().Available() {
		t.Fatal("ready handoff did not retain the native mounted/geometry/base authorities")
	}

	result, solved := runtime.Solve(ready.Mounted(), ready.Base(), ready.Geometry())
	if !solved || !result.Available() || result.Evaluations() == 0 {
		t.Fatalf("admitted arithmetic solve: solved=%v result=%#v", solved, result)
	}
	assertResultValue(t, specimen, ready, result.Root(), result.Evaluations(), result.Publications())
}

// TestArithmeticAdmissionRefusesAnInputBearingInitialOperation is the nearest
// negative to the admitted world: every declaration and injected authority is
// unchanged, but the sealed initial row names the arithmetic signature. Its
// declared input must never be fabricated from a callback or base state.
func TestArithmeticAdmissionRefusesAnInputBearingInitialOperation(t *testing.T) {
	specimen := newArithmeticSpecimen(t)
	input := specimen.input(t)
	input.Declaration.Initials = []plan.Initial{plan.DefineInitial(specimen.arithmetic.Identity(), specimen.scope)}
	ready, refusal := relationadmission.Admit(input)
	if ready.Available() || refusal == nil {
		t.Fatalf("input-bearing initial operation admitted: ready=%v refusal=%v", ready.Available(), refusal)
	}
	if refusal.Code() != relationadmission.RefusalCertificate {
		t.Fatalf("initial refusal code=%v want=%v", refusal.Code(), relationadmission.RefusalCertificate)
	}
	issues := refusal.CertificateIssues()
	if len(issues) == 0 {
		t.Fatal("certificate refusal did not retain the typed initial-shape issue")
	}
	found := false
	for _, issue := range issues {
		if issue.Pass == certificate.Typing && issue.Code == uint16(checktyping.CodeShapeMismatch) && strings.HasPrefix(issue.Path, "initial[") {
			found = true
		}
	}
	if !found {
		t.Fatalf("certificate refusal lost typed initial issue: %+v", issues)
	}
}

type arithmeticSpecimen struct {
	owner        model.OwnerID
	lineageOwner model.OwnerID
	schema       model.SchemaID
	scope        model.ScopeID
	typeID       model.TypeID

	source        model.RelationID
	result        model.RelationID
	sourceColumn  model.ColumnID
	resultAddress model.ColumnID
	resultColumn  model.ColumnID
	sourceKey     model.KeyID
	resultKey     model.KeyID
	sourceRow     model.RowID
	resultRow     model.RowID

	sourceDenominator model.DenominatorRef
	resultDenominator model.DenominatorRef
	seedSource        signature.Signature
	seedResult        signature.Signature
	arithmetic        signature.Signature
	declaration       relcompile.Declaration
	region            identity.ContentID
	one               identity.ContentID
	three             identity.ContentID
}

func newArithmeticSpecimen(t testing.TB) arithmeticSpecimen {
	t.Helper()
	owner := admissionIssue(t, "owner", model.IssueOwnerID)
	lineageOwner := admissionIssue(t, "lineage-owner", model.IssueOwnerID)
	schema := admissionIssue(t, "schema", func(value identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, value) })
	scope := admissionIssue(t, "scope", func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) })
	typeID := admissionIssue(t, "type/integer", func(value identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, value) })
	typeCapability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK {
		t.Fatal("type capability")
	}
	source := admissionIssue(t, "relation/source", func(value identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, value) })
	result := admissionIssue(t, "relation/result", func(value identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, value) })
	sourceColumn := admissionIssue(t, "column/source/value", func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(source, value) })
	resultAddress := admissionIssue(t, "column/result/address", func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(result, value) })
	resultColumn := admissionIssue(t, "column/result/value", func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(result, value) })
	sourceKey := admissionIssue(t, "key/source", func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(source, value) })
	resultKey := admissionIssue(t, "key/result", func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(result, value) })
	sourceRow := admissionIssue(t, "row/source", func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(source, value) })
	resultRow := admissionIssue(t, "row/result", func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(result, value) })
	seedSourceOperation := admissionIssue(t, "operation/seed-source", func(value identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, value) })
	seedResultOperation := admissionIssue(t, "operation/seed-result", func(value identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, value) })
	arithmeticOperation := admissionIssue(t, "operation/add-two", func(value identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, value) })
	expression := admissionIssue(t, "expression/add-two", func(value identity.ContentID) (model.ExpressionID, bool) {
		return model.IssueExpressionID(owner, value)
	})
	dependency := admissionIssue(t, "dependency/add-two", func(value identity.ContentID) (model.DependencyID, bool) {
		return model.IssueDependencyID(owner, value)
	})
	sourceDenominator := admissionDenominator(t, source, sourceKey)
	resultDenominator := admissionDenominator(t, result, resultKey)

	cardinality := admissionCardinality(t, model.ExactlyOne, 0)
	produced := admissionOutcomes(t, outcome.Produced)
	delivery := admissionScalar(t)
	seedSource := admissionSignature(t, signature.Spec{
		Identity: signature.Identity{Operation: seedSourceOperation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Outputs: []signature.Output{{
			Relation: source, Column: sourceColumn, Type: typeID, Presence: signature.ProducePresent, Denominator: sourceDenominator,
		}},
		Cardinality: cardinality,
		Outcomes:    produced,
	})
	seedResult := admissionSignature(t, signature.Spec{
		Identity: signature.Identity{Operation: seedResultOperation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Outputs: []signature.Output{{
			Relation: result, Column: resultAddress, Type: typeID, Presence: signature.ProducePresent, Denominator: resultDenominator,
		}},
		Cardinality: cardinality,
		Outcomes:    produced,
	})
	arithmetic := admissionSignature(t, signature.Spec{
		Identity: signature.Identity{Operation: arithmeticOperation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Inputs: []signature.Input{{
			Relation: source, Column: sourceColumn, Type: typeID, Presence: signature.RequirePresent,
			Delivery: delivery, Denominator: sourceDenominator,
		}},
		Outputs: []signature.Output{{
			Relation: result, Column: resultColumn, Type: typeID, Presence: signature.ProducePresent, Denominator: resultDenominator,
		}},
		Cardinality: cardinality,
		Outcomes:    produced,
	})

	declaration := relcompile.Declaration{
		SchemaID: schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(source, []model.ColumnID{sourceColumn}, []model.KeyID{sourceKey}, scope),
			model.DefineRelationSchema(result, []model.ColumnID{resultAddress, resultColumn}, []model.KeyID{resultKey}, scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(sourceColumn, typeID),
			model.DefineColumnSchema(resultAddress, typeID),
			model.DefineColumnSchema(resultColumn, typeID),
		},
		TypeCapabilities: []model.TypeCapability{typeCapability},
		Keys: []model.KeySchema{
			model.DefineKeySchema(sourceKey, []model.ColumnID{sourceColumn}),
			model.DefineKeySchema(resultKey, []model.ColumnID{resultAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(scope, nil, region.True())},
		Signatures: []signature.Signature{seedSource, seedResult, arithmetic},
		Rules: []relcompile.Rule{{
			ID:         dependency,
			Expression: expression,
			Candidate:  source,
			Joins: []relcompile.JoinSpec{{
				Relation: result, LeftColumns: []model.ColumnID{sourceColumn}, RightColumns: []model.ColumnID{resultAddress},
			}},
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence()},
			Scope:      scope,
			Apply:      arithmetic.Identity(),
			Output:     algebra.OwnerNamed(),
			Publish:    &relcompile.Publication{Relation: result, Key: resultKey, Columns: []model.ColumnID{resultColumn}},
		}},
		Initials: []plan.Initial{
			plan.DefineInitial(seedSource.Identity(), scope),
			plan.DefineInitial(seedResult.Identity(), scope),
		},
	}
	return arithmeticSpecimen{
		owner: owner, lineageOwner: lineageOwner, schema: schema, scope: scope, typeID: typeID,
		source: source, result: result, sourceColumn: sourceColumn, resultAddress: resultAddress, resultColumn: resultColumn,
		sourceKey: sourceKey, resultKey: resultKey, sourceRow: sourceRow, resultRow: resultRow,
		sourceDenominator: sourceDenominator, resultDenominator: resultDenominator,
		seedSource: seedSource, seedResult: seedResult, arithmetic: arithmetic, declaration: declaration,
		region: region.True().Identity(), one: admissionContent(t, "integer/1"), three: admissionContent(t, "integer/3"),
	}
}

func (specimen arithmeticSpecimen) input(t testing.TB) relationadmission.Input {
	t.Helper()
	lineageFactory, ok := lineage.NewFactory(specimen.lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	return relationadmission.Input{
		Declaration: specimen.declaration,
		Inventory:   specimenInventoryFactory{specimen: specimen},
		Bindings: specimenBindingFactory{bindings: map[signature.Identity]binding.Binding{
			specimen.seedSource.Identity(): fixtureBinding{signature: specimen.seedSource, worker: seedWorker{
				column: specimen.sourceColumn, row: specimen.sourceRow, typeID: specimen.typeID, value: specimen.one,
			}},
			specimen.seedResult.Identity(): fixtureBinding{signature: specimen.seedResult, worker: seedWorker{
				column: specimen.resultAddress, row: specimen.resultRow, typeID: specimen.typeID, value: specimen.one,
			}},
			specimen.arithmetic.Identity(): fixtureBinding{signature: specimen.arithmetic, worker: arithmeticWorker{
				column: specimen.resultColumn, row: specimen.resultRow, typeID: specimen.typeID, input: specimen.one, output: specimen.three,
			}},
		}},
		Algebras: specimenAlgebras{algebra: specimenAlgebra{typeID: specimen.typeID, lower: specimen.one, upper: specimen.three}},
		Lineage:  lineageFactory,
		Geometry: specimenGeometryFactory{region: specimen.region},
	}
}

func assertResultValue(t testing.TB, specimen arithmeticSpecimen, ready relationadmission.Ready, root database.Version, evaluations, publications uint64) {
	t.Helper()
	access, accessOK := arrangement.NewVectorAccess(specimen.result, []model.ColumnID{specimen.resultColumn})
	if !accessOK {
		t.Fatal("result vector access")
	}
	layout, layoutOK := ready.Mounted().Arrangement().Resolve(access)
	if !layoutOK || !layout.Available() {
		t.Fatal("result layout")
	}
	scratch := store.NewReadScratch(ready.Geometry().Manager())
	if scratch == nil || !scratch.Available() {
		t.Fatal("result scratch")
	}
	reader, readerOK := read.Bind(root, layout, ready.Geometry(), scratch)
	if !readerOK || !reader.Available() {
		t.Fatal("result reader")
	}
	seen := false
	rows := 0
	observed := make([]identity.ContentID, 0, 1)
	completed, valid := reader.Scan(func(row read.Row) bool {
		rows++
		if row == nil || !row.Available() {
			return true
		}
		cells := row.Cells()
		if len(cells) == 1 && cells[0].Value().Available() {
			observed = append(observed, cells[0].Value().Opaque())
		}
		if row.ID() != specimen.resultRow || len(cells) != 1 || cells[0].Column() != specimen.resultColumn || !cells[0].Value().Available() || cells[0].Value().Opaque() != specimen.three {
			return true
		}
		seen = true
		return true
	})
	if !completed || !valid || !seen {
		t.Fatalf("result arithmetic value: evaluations=%d publications=%d completed=%v valid=%v seen=%v rows=%d observed=%v one=%v three=%v", evaluations, publications, completed, valid, seen, rows, observed, specimen.one, specimen.three)
	}
}

type specimenInventoryFactory struct{ specimen arithmeticSpecimen }

func (factory specimenInventoryFactory) Bind(cert certificate.Certificate) (witness.Inventory, bool) {
	if !cert.Available() || cert.SchemaID() != factory.specimen.schema {
		return nil, false
	}
	storeID, storeOK := identity.IssueStore()
	if !storeOK {
		return nil, false
	}
	fence, fenceOK := address.NewFence(cert.SchemaID(), cert.Digest(), storeID, identity.MountID{0xA4}, identity.Generation(1))
	if !fenceOK {
		return nil, false
	}
	return &specimenInventory{certificate: cert, fence: fence, specimen: factory.specimen}, true
}

type specimenInventory struct {
	certificate certificate.Certificate
	fence       address.Fence
	specimen    arithmeticSpecimen
	accesses    []arrangement.Access
}

func (inventory *specimenInventory) Fence() address.Fence { return inventory.fence }

func (inventory *specimenInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	for index, relation := range inventory.certificate.Relations() {
		if relation.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	for index, column := range inventory.certificate.Columns() {
		if column.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	for index, key := range inventory.certificate.Keys() {
		if key.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	for index, scope := range inventory.certificate.Scopes() {
		if scope.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	for index, expression := range inventory.certificate.Expressions() {
		if expression.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	for index, dependency := range inventory.certificate.Dependencies() {
		if dependency.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range inventory.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}

func (inventory *specimenInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	var rows []model.RowID
	var evidence identity.ContentID
	switch ref {
	case inventory.specimen.sourceDenominator:
		rows = []model.RowID{inventory.specimen.sourceRow}
		evidence = admissionContentNoTest("evidence/source")
	case inventory.specimen.resultDenominator:
		rows = []model.RowID{inventory.specimen.resultRow}
		evidence = admissionContentNoTest("evidence/result")
	default:
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(rows, evidence)
}

func (inventory *specimenInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

type specimenGeometryFactory struct{ region identity.ContentID }

func (factory specimenGeometryFactory) Bind(mounted witness.Mounted) (geometry.Geometry, bool) {
	if !mounted.Available() || !factory.region.Available() {
		return geometry.Geometry{}, false
	}
	found := false
	for _, scopeID := range mounted.Scopes() {
		scope, ok := mounted.Scope(scopeID)
		if !ok {
			return geometry.Geometry{}, false
		}
		value, ok := mounted.RegionForScope(scope)
		if !ok || !value.Available() {
			return geometry.Geometry{}, false
		}
		if value.Identity() == factory.region {
			found = true
		}
	}
	if !found {
		return geometry.Geometry{}, false
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		return geometry.Geometry{}, false
	}
	lowering, ok := lower.New(manager, nil)
	if !ok {
		return geometry.Geometry{}, false
	}
	capability, ok := lower.NewFactory(mounted.RuntimeFence().Mount(), lowering)
	if !ok {
		return geometry.Geometry{}, false
	}
	return capability.Bind(mounted)
}

type fixtureBinding struct {
	signature signature.Signature
	worker    binding.Worker
}

func (value fixtureBinding) Signature() signature.Signature { return value.signature }
func (value fixtureBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return value.worker, value.worker != nil
}

type specimenBindingFactory struct {
	bindings map[signature.Identity]binding.Binding
}

func (factory specimenBindingFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	value, ok := factory.bindings[operation.Identity()]
	if !ok || value == nil || value.Signature().Digest() != operation.Digest() {
		return nil, false
	}
	return value, true
}

type seedWorker struct {
	column model.ColumnID
	row    model.RowID
	typeID model.TypeID
	value  identity.ContentID
}

func (worker seedWorker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if frame.Len() != 0 {
		return outcome.Result{}
	}
	return appendPresent(buffer, worker.column, worker.row, worker.typeID, worker.value)
}

type arithmeticWorker struct {
	column model.ColumnID
	row    model.RowID
	typeID model.TypeID
	input  identity.ContentID
	output identity.ContentID
}

func (worker arithmeticWorker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if frame.Len() != 1 {
		return outcome.Result{Code: outcome.NoSelection}
	}
	slot, slotOK := frame.At(0)
	cell, cellOK := slot.At(0)
	if !slotOK || !cellOK || !cell.Value().Available() || cell.Value().Opaque() != worker.input {
		return outcome.Result{Code: outcome.NoSelection}
	}
	return appendPresent(buffer, worker.column, worker.row, worker.typeID, worker.output)
}

func appendPresent(buffer *binding.ProposalBuffer, column model.ColumnID, row model.RowID, typeID model.TypeID, opaque identity.ContentID) outcome.Result {
	if buffer == nil {
		return outcome.Result{}
	}
	issuer, issuerOK := binding.NewIssuer(buffer.Fence())
	if !issuerOK {
		return outcome.Result{}
	}
	output, outputOK := buffer.Signature().OutputFor(column.Relation(), column)
	witness, witnessOK := buffer.DestinationWitness(output.Denominator)
	destination, destinationOK := issuer.IssueCell(witness, buffer.Scope(), column, row)
	value, valueOK := issuer.IssueValue(typeID, opaque)
	presence, presenceOK := model.NewPresence(model.Present)
	proposal, proposalOK := binding.NewProposal(destination, value, presence)
	if !outputOK || !witnessOK || !destinationOK || !valueOK || !presenceOK || !proposalOK || !buffer.Append(proposal) {
		return outcome.Result{}
	}
	return outcome.Result{Code: outcome.Produced}
}

type specimenAlgebra struct {
	typeID model.TypeID
	lower  identity.ContentID
	upper  identity.ContentID
}

func (algebra specimenAlgebra) Type() model.TypeID { return algebra.typeID }

func (algebra specimenAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	if left.Same(right) {
		return left, true
	}
	if (left.Opaque() == algebra.lower && right.Opaque() == algebra.upper) || (left.Opaque() == algebra.upper && right.Opaque() == algebra.lower) {
		if left.Opaque() == algebra.upper {
			return left, true
		}
		return right, true
	}
	return binding.ValueToken{}, false
}

func (algebra specimenAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}

func (algebra specimenAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return false
	}
	return left.Same(right) || left.Opaque() == algebra.lower && right.Opaque() == algebra.upper
}

type specimenAlgebras struct{ algebra specimenAlgebra }

func (registry specimenAlgebras) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

func admissionContent(t testing.TB, label string) identity.ContentID {
	t.Helper()
	return admissionContentNoTest(label)
}

func admissionContentNoTest(label string) identity.ContentID {
	value, ok := identity.DeriveContentID(admissionLawDomain, []byte(label))
	if !ok {
		panic("admission law content")
	}
	return value
}

func admissionIssue[T any](t testing.TB, label string, issue func(identity.ContentID) (T, bool)) T {
	t.Helper()
	value, ok := issue(admissionContent(t, label))
	if !ok {
		t.Fatalf("issue %s", label)
	}
	return value
}

func admissionCardinality(t testing.TB, kind model.CardinalityKind, bound uint32) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, bound)
	if !ok {
		t.Fatal("cardinality")
	}
	return value
}

func admissionOutcomes(t testing.TB, codes ...outcome.Code) outcome.Set {
	t.Helper()
	value, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("outcomes")
	}
	return value
}

func admissionScalar(t testing.TB) signature.Delivery {
	t.Helper()
	value, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	return value
}

func admissionSignature(t testing.TB, spec signature.Spec) signature.Signature {
	t.Helper()
	value, ok := signature.Seal(spec)
	if !ok {
		t.Fatal("signature")
	}
	return value
}

func admissionDenominator(t testing.TB, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return value
}
