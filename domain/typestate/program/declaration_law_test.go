package program

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The declaration is composed of the sealed execution forms and nothing else:
// an exact keyed read of the receiver's Value fact, a dependent selected read
// of this axis's own cell whose coordinate is computed from that fact and
// whose tag is the obligation's protocol, one fold that routes its result back
// to the read cell, and an identity carry. No seventh form is introduced.
func TestObligationProgramIsOneExactReadAndOneSelectedCellRead(t *testing.T) {
	declaration := Obligation()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("obligation declaration rejected: %+v", problem)
	}
	if declaration.Candidate.AxisRelation.Axis.Key != valueAxisKey || declaration.Candidate.AxisRelation.Member != MountedCallArgumentCandidates {
		t.Fatalf("candidate = %+v, want the Value mounted-call argument candidates", declaration.Candidate)
	}
	if got := declaration.JoinCount(); got != 2 {
		t.Fatalf("join count = %d, want the receiver read and the cell read", got)
	}

	receiver, ok := declaration.JoinAt(0)
	if !ok || receiver.Read.Form != ruleprogram.Exact || receiver.Read.Axis.EntryReference().Key != valueAxisKey ||
		receiver.Relation.Member != MountedCallArguments || receiver.Key.Member != MountedCallArgumentKey ||
		len(receiver.Sources) != 1 || !receiver.Sources[0].Candidate || receiver.Predicate.Declared() {
		t.Fatalf("receiver join = %+v, want a candidate-only exact Value read", receiver)
	}

	cell, ok := declaration.JoinAt(1)
	if !ok || cell.Read.Form != ruleprogram.Selected || cell.Read.Axis.EntryReference().Key != AxisKey ||
		cell.Relation.Member != StateCells || cell.Key.Member != StateCellKey ||
		cell.Predicate.Member != StateCellProtocol ||
		len(cell.Sources) != 2 || !cell.Sources[0].Candidate || cell.Sources[1] != ruleprogram.PriorSource(0) {
		t.Fatalf("cell join = %+v, want the protocol-selected candidate plus prior receiver result", cell)
	}
	if cell.Read.Contract.DenominatorRef.EntryReference().Key != schema.Key("coordinates/typestate") {
		t.Fatalf("cell denominator = %+v, want the typestate coordinate world", cell.Read.Contract.DenominatorRef)
	}

	if declaration.Carry == nil || declaration.Carry.Mode != ruleprogram.CarryIdentity || declaration.Carry.Transform.Declared() {
		t.Fatalf("carry = %+v, want an untransformed identity carry", declaration.Carry)
	}
	if len(declaration.Fold.Inputs) != 2 || declaration.Fold.Inputs[0] != 0 || declaration.Fold.Inputs[1] != 1 {
		t.Fatalf("fold inputs = %v, want both reads", declaration.Fold.Inputs)
	}
	if len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("fold outputs = %d, want the successor state alone", len(declaration.Fold.Outputs))
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 1 ||
		output.Column.Key != StateOutputKey || output.Destination.Member != StateCellDestination {
		t.Fatalf("state output = %+v, want the successor routed back to the cell it was read from", output)
	}
}

// An operation moves a resource's state; it does not move the resource. The
// coordinate the successor is written at is therefore the coordinate the
// current state was read at, and the two projections address the one relation
// that owns the cell.
func TestSuccessorStateIsPublishedAtTheCellItWasReadFrom(t *testing.T) {
	catalog := AxisMemberCatalog()
	read, readOK := projectionByKey(catalog, StateCellKey)
	written, writtenOK := projectionByKey(catalog, StateCellDestination)
	if !readOK || !writtenOK {
		t.Fatal("the cell projections are not declared")
	}
	if read.Relation != written.Relation || read.Relation != StateCells {
		t.Fatalf("cell projections address %q and %q, want the one cell relation", read.Relation, written.Relation)
	}
	if read.Result != CellCarrier || written.Result != CellCarrier {
		t.Fatalf("cell projections carry %q and %q, want the cell carrier", read.Result, written.Result)
	}
	if read.Role != member.Key || written.Role != member.Destination {
		t.Fatalf("cell projection roles = %d and %d, want key and destination", read.Role, written.Role)
	}
}

// A receiver the analysis cannot follow is the case an unproven verdict
// answers. The read must therefore propagate authenticated opaque evidence:
// refusing it would drop the call out of the population and report nothing,
// which is the one answer a soundness judgment may not give.
func TestOpaqueReceiverIsJudgedRatherThanDropped(t *testing.T) {
	declaration := Obligation()
	for index := 0; index < declaration.JoinCount(); index++ {
		join, ok := declaration.JoinAt(index)
		if !ok {
			t.Fatalf("join %d is unavailable", index)
		}
		if join.Read.Contract.OnOpaque != ruleprogram.OnOpaquePropagateAuthenticated {
			t.Fatalf("join %d refuses opaque evidence, so an unfollowable receiver would be reported clean", index)
		}
	}
}

// The declaration seals through the real plan compiler against the axes,
// members, roles and denominators it names. This is what proves the family is
// expressible as a Program rather than only as prose about one.
func TestObligationProgramSealsThroughThePlanCompiler(t *testing.T) {
	compiled, failure := compileObligationProgram(t, Obligation())
	if failure.Available() || !compiled.Available() {
		t.Fatalf("sealed obligation plan unavailable: catalog=%+v failure=%+v", compiled, failure)
	}
}

// The seal is owner-qualified end to end. A candidate taken from the wrong
// axis, a cell read whose coordinate does not consume the receiver fact, and a
// destination that is not a destination projection are each refused rather
// than repaired.
func TestObligationProgramRefusesUnownedAndUnkeyedRows(t *testing.T) {
	foreignCandidate := Obligation()
	foreignCandidate.Candidate.AxisRelation.Axis = schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: AxisKey}
	if _, failure := compileObligationProgram(t, foreignCandidate); !failure.Available() {
		t.Fatal("a candidate from an axis that declares no such relation was admitted")
	}

	unkeyedCell := Obligation()
	unkeyedCell.Joins[1].Sources = []ruleprogram.SourceRef{ruleprogram.CandidateSource()}
	if _, failure := compileObligationProgram(t, unkeyedCell); !failure.Available() {
		if problem, valid := unkeyedCell.Check(); valid {
			t.Fatalf("a cell read that ignores the receiver fact sealed: %+v", problem)
		}
	}

	keyAsDestination := Obligation()
	keyAsDestination.Fold.Outputs[0].Destination.Member = StateCellKey
	if _, failure := compileObligationProgram(t, keyAsDestination); !failure.Available() {
		t.Fatal("the state output accepted a key projection as its destination")
	}

	missingReducer := Obligation()
	missingReducer.Fold.Reducer = member.ReducerRef{}
	if problem, valid := missingReducer.Check(); valid || problem.Kind != ruleprogram.ProblemFold {
		t.Fatalf("a fold with no reducer: valid=%v problem=%+v", valid, problem)
	}
}

// The axis declaration is the coordinate space statecell seals: dense, because
// both directories it is the product of are dense, so a cell with no row is a
// published absence rather than ignorance, and the read that materializes that
// absence names the axis's own coordinate world.
func TestAxisDeclaresTheDenseCellSpaceItsReadDependsOn(t *testing.T) {
	spec := AxisEntry[testAxisInputs]()
	if spec.Cardinality != axis.CardinalityDense {
		t.Fatalf("cardinality = %d, want dense", spec.Cardinality)
	}
	if axis.CoverageFor(spec.Cardinality) != axis.CoverageTotal {
		t.Fatal("a dense cell space must publish total coverage")
	}
	if spec.Signature.Key != CellCarrier || spec.Signature.Fact != StateCarrier {
		t.Fatalf("signature = %+v, want the cell key and the abstract state fact", spec.Signature)
	}
	if len(spec.Dependencies) != 2 || spec.Dependencies[0] != "heap" || spec.Dependencies[1] != valueAxisKey {
		t.Fatalf("dependencies = %v, want the two directories the space is derived from", spec.Dependencies)
	}
	if len(spec.Frame.Outputs) != 1 || spec.Frame.Outputs[0].Key != StateOutputKey || spec.Frame.Outputs[0].Writer != AxisKey {
		t.Fatalf("frame = %+v, want one self-written state column", spec.Frame)
	}
	cell, ok := Obligation().JoinAt(1)
	if !ok || cell.Read.Contract.Sparse != ruleprogram.SparseDefault {
		t.Fatalf("cell read sparsity = %+v, want the declared default at an unwritten cell", cell.Read.Contract)
	}
}

type testAxisInputs interface {
	HeapAllocationKeyCount() int
	TargetProtocolCount() int
}

func projectionByKey(catalog member.Catalog, key schema.Key) (member.Projection, bool) {
	for index := 0; index < catalog.ProjectionCount(); index++ {
		projection, ok := catalog.ProjectionAt(index)
		if ok && projection.Key == key {
			return projection, true
		}
	}
	return member.Projection{}, false
}

// obligationNoopSurface fills an unrelated surface in the focused seal
// fixture. It has no declaration authority.
type obligationNoopSurface struct{ kind schema.SurfaceKind }

func (surface obligationNoopSurface) Kind() schema.SurfaceKind { return surface.kind }
func (obligationNoopSurface) Entries() []schema.Entry          { return nil }
func (obligationNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// focusedValueCatalog is the Value member vocabulary this judgment requires:
// the candidate relation over mounted call arguments and the exact read of one
// argument's solved Value fact. Value does not publish it today - the columns
// are named in this package's declaration for exactly that reason - so the
// focused seal authors them here, the same way the Store law authors its own
// focused Value and Placement axes.
func focusedValueCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := valueCandidateProvider()
	catalog, ok := member.NewCatalog(
		[]member.Relation{
			{Key: MountedCallArgumentCandidates, Subject: MountedCallArgumentCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: MountedCallArguments, Subject: ValueFactCarrier, Inputs: []member.Carrier{MountedCallArgumentCarrier}, CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Projection{
			{Key: MountedCallArgumentKey, Relation: MountedCallArguments, Role: member.Key, Result: ValueCoordinateCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{
			{Key: schema.Key("value/reducer/identity"), Inputs: []member.ReducerInput{
				{Axis: axisReference(valueAxisKey), Carrier: ValueFactCarrier, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
			}, Outputs: []member.ReducerOutput{
				{Axis: axisReference(valueAxisKey), Carrier: ValueFactCarrier},
			}},
		},
		[]member.CarryTransform{},
	)
	if !ok {
		t.Fatal("focused Value member catalog rejected")
	}
	return catalog
}

func compileObligationProgram(t *testing.T, declaration ruleprogram.Program) (ruleplan.Catalog, schema.SealFailure) {
	t.Helper()
	valueAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:         valueAxisKey,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: "value/facts", Writer: valueAxisKey}}},
		Catalog:     focusedValueCatalog(t),
		Signature:   axis.Signature{Key: ValueCoordinateCarrier, Fact: ValueFactCarrier},
		Semantic:    vocabulary.RoleKey("factor/value"),
	})
	if !ok {
		t.Fatal("focused Value axis rejected")
	}
	typestateAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:          AxisKey,
		Storage:      axis.StorageEngine,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeProcess,
		Mutability:   axis.MutabilityFrozen,
		Concurrency:  axis.ConcurrencyShared,
		Dependencies: []schema.Key{valueAxisKey},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: StateOutputKey, Writer: AxisKey}}},
		Catalog:      AxisMemberCatalog(),
		Signature:    axis.Signature{Key: CellCarrier, Fact: StateCarrier},
		Semantic:     vocabulary.RoleKey(FactorRole),
	})
	if !ok {
		t.Fatal("focused Typestate axis rejected")
	}

	programRule, ok := rule.New(rule.Spec{
		Key:      RuleKey,
		Lane:     rule.LaneLink,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  declaration,
	})
	if !ok {
		// A declaration the rule surface refuses is a refused declaration.
		// The fixture reports it as one instead of failing the law that asked
		// whether it would be refused at all.
		return ruleplan.Catalog{}, seal.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}

	roles := []string{"factor/value", FactorRole, RuleRole, OperandRole}
	entries := make([]*structure.Entry, 0, len(roles))
	for index, role := range roles {
		entry, entryOK := structure.New(structure.Spec{
			Key:      vocabulary.RoleKey(role),
			Category: structure.CategorySemanticRole,
			Ordinal:  uint16(index + 1),
			Spelling: role,
			Accepted: true,
		})
		if !entryOK {
			t.Fatalf("semantic role %q rejected", role)
		}
		entries = append(entries, entry)
	}
	// The structure seal is a closed-world surface: every declared category
	// needs at least one entry, though this focused law only exercises
	// semantic roles. These neutral entries complete the fixture and carry no
	// authority over the declaration under test.
	for category := structure.CategoryInvalid + 1; category.Available(); category++ {
		if category == structure.CategorySemanticRole {
			continue
		}
		spelling := "typestate-law/category/" + strconv.Itoa(int(category))
		entry, entryOK := structure.New(structure.Spec{
			Key:      schema.Key(spelling),
			Category: category,
			Ordinal:  1,
			Spelling: spelling,
			Accepted: true,
		})
		if !entryOK {
			t.Fatalf("focused structure filler %q rejected", spelling)
		}
		entries = append(entries, entry)
	}

	valueUniverse, ok := identity.DeriveContentID("go-lua/typestate-program/value", []byte("value"))
	if !ok {
		t.Fatal("Value denominator universe identity unavailable")
	}
	typestateUniverse, ok := identity.DeriveContentID("go-lua/typestate-program/typestate", []byte("typestate"))
	if !ok {
		t.Fatal("Typestate denominator universe identity unavailable")
	}
	valueDenominator, ok := denominator.Coordinate(valueAxisKey, valueUniverse)
	if !ok {
		t.Fatal("Value denominator rejected")
	}
	typestateDenominator, ok := denominator.Coordinate(AxisKey, typestateUniverse)
	if !ok {
		t.Fatal("Typestate denominator rejected")
	}

	builder := seal.NewBuilder()
	for _, surface := range []seal.Surface{
		structure.NewSurface(entries),
		axis.NewSurface([]*axis.Template[struct{}]{valueAxis, typestateAxis}),
		obligationNoopSurface{kind: schema.SurfaceKindIssuance},
		rule.NewSurface([]*rule.Template{programRule}),
		obligationNoopSurface{kind: schema.SurfaceKindDiagnostic},
		obligationNoopSurface{kind: schema.SurfaceKindComposite},
		denominator.NewSurface([]*denominator.Entry{valueDenominator, typestateDenominator}),
		obligationNoopSurface{kind: schema.SurfaceKindQuery},
		obligationNoopSurface{kind: schema.SurfaceKindObservation},
	} {
		if !builder.Register(surface) {
			t.Fatal("focused surface registration failed")
		}
	}
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil {
		return ruleplan.Catalog{}, sealFailure
	}
	return ruleplan.Compile(table)
}
