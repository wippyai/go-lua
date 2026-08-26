package arithmetic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Fixture is the complete arithmetic parity world.  Declaration, mount,
// geometry, and the initial database are retained as immutable capabilities;
// callers must advance the root only through the runtime/publication API.
// The worker is test data, not a second execution surface.
type Fixture struct {
	declaration Declaration
	mounted     witness.Mounted
	view        geometry.Geometry
	base        database.Version
	seedSource  *worker
	arithmetic  *worker
	outputWrite arrangement.Layout
}

// New constructs one fresh arithmetic world through declaration checking,
// mount specialization, physical cofiber construction, and database
// bootstrap.  The optional byte only separates independent mount identities in
// tests; it is not a semantic input.
func New(t testing.TB, mountBytes ...byte) Fixture {
	t.Helper()
	if len(mountBytes) > 1 {
		t.Fatalf("arithmetic fixture: at most one mount byte, got %d", len(mountBytes))
	}
	mountByte := byte(0xA1)
	if len(mountBytes) == 1 {
		mountByte = mountBytes[0]
	}
	declaration := buildDeclaration(t)

	seedCandidate := &worker{}
	seedSource := &worker{}
	arithmetic := &worker{}
	factory := factory{bindings: map[signature.Identity]binding.Binding{
		declaration.IDs.SeedCandidate: operationBinding{operation: declaration.Signatures[0], worker: seedCandidate},
		declaration.IDs.SeedSource:    operationBinding{operation: declaration.Signatures[1], worker: seedSource},
		declaration.IDs.Arithmetic:    operationBinding{operation: declaration.Arithmetic, worker: arithmetic},
	}}
	mounted, scope, manager, mask := newMounted(t, declaration, mountByte, factory)
	outputAccess, ok := arrangement.NewVectorAccess(declaration.IDs.Output, []model.ColumnID{declaration.IDs.OutputWrite})
	if !ok {
		t.Fatal("arithmetic output payload access")
	}
	outputWrite, ok := mounted.Arrangement().Resolve(outputAccess)
	if !ok || !outputWrite.Available() {
		t.Fatal("arithmetic sealed output payload layout")
	}
	scopeSchemas := declaration.Schema.Scopes()
	if len(scopeSchemas) != 1 || scopeSchemas[0].ID() != declaration.IDs.Scope || !scopeSchemas[0].Region().Available() {
		t.Fatal("arithmetic declared scope schema")
	}
	declared := map[identity.ContentID]support.Mask{scopeSchemas[0].Region().Identity(): mask}
	scopes, ok := cofiber.NewDeclared(mounted, manager, declared)
	if !ok || !scopes.Available() {
		t.Fatal("arithmetic cofiber")
	}
	view, ok := geometry.New(mounted, scopes)
	if !ok || !view.Available() {
		t.Fatal("arithmetic geometry")
	}
	base, ok := database.Bootstrap(mounted, view)
	if !ok || !base.Available() {
		t.Fatal("arithmetic database")
	}

	prepareSeed(t, mounted, scope, seedCandidate, declaration.Signatures[0], declaration.Candidates,
		[]model.RowID{declaration.IDs.CandidateA, declaration.IDs.CandidateB},
		[]model.ColumnID{declaration.IDs.CandidateAddress},
		[][]binding.ValueToken{{mustValue(t, mounted, declaration.IDs.Type, "address/a")}, {mustValue(t, mounted, declaration.IDs.Type, "address/b")}})
	prepareSeed(t, mounted, scope, seedSource, declaration.Signatures[1], declaration.Sources,
		[]model.RowID{declaration.IDs.SourceA, declaration.IDs.SourceB},
		[]model.ColumnID{declaration.IDs.SourceAddress, declaration.IDs.SourceLeft, declaration.IDs.SourceRight},
		[][]binding.ValueToken{
			{mustValue(t, mounted, declaration.IDs.Type, "address/a"), mustValue(t, mounted, declaration.IDs.Type, "address/a"), mustValue(t, mounted, declaration.IDs.Type, "address/a")},
			{mustValue(t, mounted, declaration.IDs.Type, "address/b"), mustValue(t, mounted, declaration.IDs.Type, "address/b"), mustValue(t, mounted, declaration.IDs.Type, "address/b")},
		})

	arithmetic.evaluate = arithmeticEvaluator(mounted, declaration)
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		t.Fatal("arithmetic scratch")
	}
	door, ok := publish.New(mounted, view)
	if !ok || !door.Available() {
		t.Fatal("arithmetic publish door")
	}
	base = publishSeed(t, mounted, door, base, scratch, scope, seedCandidate, declaration.Signatures[0])
	base = publishSeed(t, mounted, door, base, scratch, scope, seedSource, declaration.Signatures[1])
	return Fixture{declaration: declaration, mounted: mounted, view: view, base: base, seedSource: seedSource, arithmetic: arithmetic, outputWrite: outputWrite}
}

// Declaration returns the immutable checked input artifact.
func (fixture Fixture) Declaration() Declaration { return fixture.declaration }

// IDs returns the owner-issued logical vocabulary used by the fixture.
func (fixture Fixture) IDs() IDs { return fixture.declaration.IDs }

// Observation returns the schema-sealed cross-relation observation family.
// Its parent population is Source; its output cells are owned by Output.
func (fixture Fixture) Observation() identity.ContentID { return fixture.declaration.Observation }

// Mounted returns the immutable mount capability.
func (fixture Fixture) Mounted() witness.Mounted { return fixture.mounted }

// View returns the solve-local physical geometry authority.
func (fixture Fixture) View() geometry.Geometry { return fixture.view }

// Base returns the seeded immutable database root.  It contains candidate and
// source facts; output is an admitted but empty destination relation.
func (fixture Fixture) Base() database.Version { return fixture.base }

// ArithmeticEvaluations reports the number of semantic worker invocations
// made by the arithmetic operation.
func (fixture Fixture) ArithmeticEvaluations() int {
	if fixture.arithmetic == nil {
		return 0
	}
	return fixture.arithmetic.Evaluations()
}

// Output binds the canonical output key layout to a committed root.  A fresh
// read scratch is created for each reader, so callers cannot share mutable
// read state accidentally.
func (fixture Fixture) Output(root database.Version) (read.Reader, bool) {
	if !fixture.mounted.Available() || !root.Available() || fixture.view.Manager() == nil {
		return read.Reader{}, false
	}
	access, ok := arrangement.NewKeyAccess(fixture.declaration.IDs.OutputKey)
	if !ok {
		return read.Reader{}, false
	}
	layout, ok := fixture.mounted.Arrangement().Resolve(access)
	if !ok || !layout.Available() {
		return read.Reader{}, false
	}
	scratch := store.NewReadScratch(fixture.view.Manager())
	if scratch == nil || !scratch.Available() {
		return read.Reader{}, false
	}
	return read.Bind(root, layout, fixture.view, scratch)
}

// OutputPayload binds the sealed output vector for parity assertions. The
// key reader above intentionally exposes only row identity; this reader
// exposes the exact authored address/write columns and nothing else.
func (fixture Fixture) OutputPayload(root database.Version) (read.Reader, bool) {
	if !fixture.mounted.Available() || !root.Available() || fixture.view.Manager() == nil || !fixture.outputWrite.Available() {
		return read.Reader{}, false
	}
	scratch := store.NewReadScratch(fixture.view.Manager())
	if scratch == nil || !scratch.Available() {
		return read.Reader{}, false
	}
	return read.Bind(root, fixture.outputWrite, fixture.view, scratch)
}

// Expected contains the two output values that the declared worker must
// publish.  It is derived from the same opaque operands used to seed the
// source relation; no reader or physical coordinate participates.
func (fixture Fixture) Expected() map[model.RowID]identity.ContentID {
	if !fixture.declaration.Schema.Available() {
		return nil
	}
	left := derive("address/a")
	right := derive("address/a")
	first, _ := identity.DeriveContentID(declarationDomain+"/result", left[:], right[:])
	left = derive("address/b")
	right = derive("address/b")
	second, _ := identity.DeriveContentID(declarationDomain+"/result", left[:], right[:])
	return map[model.RowID]identity.ContentID{
		fixture.declaration.IDs.OutputA: first,
		fixture.declaration.IDs.OutputB: second,
	}
}

func mustValue(t testing.TB, mounted witness.Mounted, typeID model.TypeID, label string) binding.ValueToken {
	t.Helper()
	value, ok := mounted.IssueValue(typeID, derive(label))
	if !ok {
		t.Fatalf("arithmetic value %s", label)
	}
	return value
}

func prepareSeed(t testing.TB, mounted witness.Mounted, scope witness.Scope, worker *worker, operation signature.Signature, denominator model.DenominatorRef, rows []model.RowID, columns []model.ColumnID, values [][]binding.ValueToken) {
	t.Helper()
	if worker == nil || !operation.Available() || len(rows) == 0 || len(rows) != len(values) || len(columns) == 0 {
		t.Fatal("arithmetic seed shape")
	}
	token, ok := mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("arithmetic seed scope")
	}
	denominatorWitness, ok := mounted.Denominator(denominator)
	if !ok {
		t.Fatal("arithmetic seed denominator witness")
	}
	proposals := make([]binding.Proposal, 0, len(rows)*len(columns))
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("arithmetic seed presence")
	}
	for rowIndex, row := range rows {
		if len(values[rowIndex]) != len(columns) {
			t.Fatal("arithmetic seed columns")
		}
		for columnIndex, column := range columns {
			cell, cellOK := mounted.IssueCell(denominatorWitness, scope, column, row)
			if !cellOK {
				t.Fatal("arithmetic seed cell")
			}
			proposal, proposalOK := binding.NewProposal(cell, values[rowIndex][columnIndex], presence)
			if !proposalOK {
				t.Fatal("arithmetic seed proposal")
			}
			proposals = append(proposals, proposal)
		}
	}
	worker.operation = operation
	worker.proposals = map[binding.ScopeToken][]binding.Proposal{token: proposals}
}

func publishSeed(t testing.TB, mounted witness.Mounted, door publish.Door, base database.Version, scratch *store.ReadScratch, scope witness.Scope, worker *worker, operation signature.Signature) database.Version {
	t.Helper()
	provenance, ok := arithmeticProvenance(mounted, operation)
	if !ok {
		t.Fatal("arithmetic seed lineage")
	}
	application, ok := apply.Apply(mounted, operation.Identity(), scope, provenance, binding.NewOwnerNamedDestination(operation.Outputs()[0].Relation))
	if !ok || !application.Available() || application.Outcome().Code != outcome.Produced {
		t.Fatal("arithmetic seed apply")
	}
	settlement := door.Publish(base, scratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() {
		t.Fatal("arithmetic seed publish")
	}
	return settlement.Next()
}

func arithmeticProvenance(mounted witness.Mounted, operation signature.Signature) (model.LineageRef, bool) {
	if !mounted.Available() || !operation.Available() || operation.OutputLen() == 0 {
		return model.LineageRef{}, false
	}
	authority, ok := mounted.Lineage()
	if !ok || authority == nil {
		return model.LineageRef{}, false
	}
	var result model.LineageRef
	for _, output := range operation.Outputs() {
		if !output.Denominator.Available() {
			return model.LineageRef{}, false
		}
		ref, refOK := mounted.DenominatorLineage(output.Denominator)
		if !refOK {
			return model.LineageRef{}, false
		}
		if !result.Available() {
			result = ref
			continue
		}
		result, ok = authority.Join(result, ref)
		if !ok {
			return model.LineageRef{}, false
		}
	}
	return result, result.Available()
}

func arithmeticEvaluator(mounted witness.Mounted, declaration Declaration) func(binding.Frame, *binding.ProposalBuffer) outcome.Result {
	return func(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		if !mounted.Available() || frame.Len() != 3 || buffer == nil {
			return outcome.Result{Code: outcome.NoSelection}
		}
		// Apply may normalize a conjunction into a new mounted scope. Redeem
		// that exact invocation scope from the frame; never reuse a declaration
		// scope captured at fixture construction for output addresses.
		scope, scopeOK := mounted.ScopeForToken(frame.Scope())
		if !scopeOK || !scope.ValidFor(mounted.RuntimeFence()) {
			return outcome.Result{Code: outcome.Refused}
		}
		var cells [3]binding.Cell
		for index := range cells {
			slot, ok := frame.At(index)
			if !ok || !slot.IsScalar() {
				return outcome.Result{Code: outcome.NoSelection}
			}
			cells[index], ok = slot.At(0)
			if !ok || !cells[index].Value().Available() {
				return outcome.Result{Code: outcome.NoSelection}
			}
		}
		candidate := cells[0].Value().Opaque()
		left := cells[1].Value().Opaque()
		right := cells[2].Value().Opaque()
		result, ok := identity.DeriveContentID(declarationDomain+"/result", left[:], right[:])
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		row := declaration.IDs.OutputA
		if candidate == derive("address/b") {
			row = declaration.IDs.OutputB
		}
		outputWitness, witnessOK := mounted.Denominator(declaration.Outputs)
		if !witnessOK {
			return outcome.Result{Code: outcome.Refused}
		}
		address, ok := mounted.IssueCell(outputWitness, scope, declaration.IDs.OutputAddress, row)
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		write, ok := mounted.IssueCell(outputWitness, scope, declaration.IDs.OutputWrite, row)
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		addressValue, ok := mounted.IssueValue(declaration.IDs.Type, candidate)
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		writeValue, ok := mounted.IssueValue(declaration.IDs.Type, result)
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		presence, ok := model.NewPresence(model.Present)
		if !ok {
			return outcome.Result{Code: outcome.Refused}
		}
		first, ok := binding.NewProposal(address, addressValue, presence)
		if !ok || !buffer.Append(first) {
			return outcome.Result{Code: outcome.Refused}
		}
		second, ok := binding.NewProposal(write, writeValue, presence)
		if !ok || !buffer.Append(second) {
			return outcome.Result{Code: outcome.Refused}
		}
		return outcome.Result{Code: outcome.Produced}
	}
}
