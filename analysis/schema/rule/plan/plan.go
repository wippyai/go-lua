// Package plan compiles the callback-free Rule program declarations in a
// sealed schema into dense, owner-qualified addresses.  The result is still
// schema data: it contains no engine handles, callbacks, or domain values.
package plan

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	rule "github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const maxUint32 = uint64(^uint32(0))

// RelationAddr is a dense address for one owner-issued relation member.
// Axis is the ordinal of the owning axis in the sealed axis view and Member
// is the relation's ordinal in that axis's member catalog.
type RelationAddr struct {
	Axis   uint32
	Member uint32
}

// ProjectionAddr is a dense address for one owner-issued projection member.
type ProjectionAddr struct {
	Axis   uint32
	Member uint32
}

// ReducerAddr is a dense address for one owner-issued reducer member.
type ReducerAddr struct {
	Axis   uint32
	Member uint32
}

// OutputAddr is a dense address for one published output column. Frame is the
// declaration position in the owning axis's output frame.
type OutputAddr struct {
	Axis  uint32
	Frame uint32
}

// DenominatorAddr is an optional dense address into the complete denominator
// view. Present is explicit so the zero ordinal remains a valid address.
type DenominatorAddr struct {
	Ordinal uint32
	Present bool
}

// CarryTransformAddr is a dense address for one owner-issued carry transform
// member.
type CarryTransformAddr struct {
	Axis   uint32
	Member uint32
}

// Span identifies one contiguous range in a plan's private flattened source
// storage.
type Span struct {
	Start uint32
	Count uint32
}

// Source is one dense candidate-or-prior-join reference. Position is zero for
// the candidate and otherwise the prior join ordinal.
type Source struct {
	Position  uint32
	Candidate bool
}

// ReadContract is the fully resolved scalar half of a read contract. The
// authored denominator reference is deliberately absent; Join.Denominator is
// its sole compiled spelling.
type ReadContract struct {
	Order        program.Order
	Sparse       program.Sparse
	OnOpaque     program.OnOpaque
	Multiplicity program.Multiplicity
}

// Join is one flattened, schema-only join declaration. Sources addresses the
// private source range; the remaining fields are owner-issued member and
// sealed-axis ordinals.
type Join struct {
	Sources          Span
	Input            uint32
	Relation         RelationAddr
	Key              ProjectionAddr
	Predicate        ProjectionAddr
	PredicatePresent bool
	ReadAxis         uint32
	ReadForm         program.ReadForm
	ReadContract     ReadContract
	// PointBound is the authored disposition copied from the sealed
	// ReadDecl. It states whether this Input slot's own predecessor
	// topology point is transported into the rule, or whether the read
	// resolves through its Factor's directory/route surface and shares the
	// candidate's own point instead.
	PointBound program.PointBoundDecl
	// Cardinality is the route-width proof copied from the sealed read
	// contract. It is retained alongside ReadContract.Multiplicity so an
	// emitter can consume the bounded route fact without reopening the
	// declaration surface.
	Cardinality program.Multiplicity
	Denominator DenominatorAddr
	// Parent is the compiled restatement of JoinDecl.Parent: the relation
	// whose candidate rows this join's relation nests under as a bounded,
	// ordinal-addressed member set. ParentPresent is explicit because the
	// zero relation address is a valid one.
	//
	// It is compiled rather than dropped because it is the addressing fact a
	// Summary read over a self-provided member set is admissible by. A
	// consumer that lost it would have to rediscover nestedness from the
	// join's own shape, which is the inference the declaration exists to
	// replace.
	Parent        RelationAddr
	ParentPresent bool
	// KeyVector is the compiled restatement of JoinDecl.KeyVector: the
	// directory whose rows publish the ordered dense key vector this read is
	// taken over. It is the third addressing a whole-vector read can have, and
	// it is compiled for the same reason Parent is - a consumer that lost it
	// would have to rediscover the span from the join's shape.
	KeyVector        RelationAddr
	KeyVectorPresent bool
	// Addressing is the directory whose candidate ordinal indexes this join:
	// the relation that ISSUES the rows the read is resolved against. For an
	// exact read that is the read relation's own candidate provider; for a
	// whole-vector read over a nested member set it is the parent relation's,
	// because the owner resolves the parent row and enumerates its members
	// from it.
	//
	// AddressingPresent is explicit, and false is a statement of its own: a
	// selected read is addressed by the selection its own family resolves, and
	// an issued candidate is a Program row with no directory at all. Neither
	// names one.
	//
	// It is compiled rather than dropped because two directories addressed by
	// one occurrence enumerate their rows INDEPENDENTLY. A consumer handed
	// only the rule's dense ordinal can index its own directory correctly and
	// every other one by accident; carrying the directory is what lets the
	// ordinal be resolved again where the rows actually live.
	Addressing        RelationAddr
	AddressingPresent bool
}

// Carry is the compiled optional whole-output carry. TransformPresent is
// explicit because the zero transform address is a valid dense address.
type Carry struct {
	Input            uint32
	Mode             program.CarryMode
	Transform        CarryTransformAddr
	TransformPresent bool
	// TransformAxis and TransformKey retain the authored owner identity that
	// produced Transform.  The dense address is an execution-facing ordinal,
	// while these two fields are the sealed metadata fence: a later consumer
	// must not re-derive a member key from the address or accept a different
	// member that happens to occupy the same slot.
	TransformAxis schema.Key
	TransformKey  schema.Key
}

// Activation is one compiled branch vocabulary: the dense projection addresses
// the construct plane mounts a candidate branch by. Every address is a
// projection in the member.Identity role, because each one names a subject the
// analyzer did not mint and no dense coordinate carries.
type Activation struct {
	// Branch is the declaration ordinal of the join whose members are the
	// branches - the parent-declaring vector read.
	Branch uint32
	// Application is projected from the rule's own candidate row; the other
	// four are projected from one branch row.
	Application ProjectionAddr
	Target      ProjectionAddr
	Endpoint    ProjectionAddr
	Mount       ProjectionAddr
	Body        ProjectionAddr
}

// Transport is one compiled activation transport row: the dense ordinal of the
// axis carried across the candidate's transition, and whether the mounted body
// carries that axis back out to its trigger. One row is one axis, so the
// import/export symmetry is the shape rather than a checked relation between
// two lists.
type Transport struct {
	Axis     uint32
	Exported bool
}

// Output is one reducer publication declaration after all owner-issued
// members have been projected to dense addresses.
type Output struct {
	Address          OutputAddr
	Destination      ProjectionAddr
	Mode             program.OutputMode
	Slot             uint32
	RouteJoin        uint32
	RouteJoinPresent bool
}

// ScratchShape gives bounded counts for the plan's flattened storage. The
// counts are uint32 because they are also the bounds of dense execution
// addresses.
type ScratchShape struct {
	SourceCount    uint32
	JoinCount      uint32
	FoldInputCount uint32
	OutputCount    uint32
}

// AxisDirectoryEntry is one immutable row in the axis directory carried by a
// Catalog. Key is the declaration identity from the sealed axis surface;
// Semantic is the identity resolved from the sealed structural semantic-role
// vocabulary under the axis's declared role. The pair is the proof that a
// dense Plan axis ordinal names the same principal the engine declares a
// Factor under.
//
// The type contains no ordinal field because its position in AxisAt's bounded
// view is the ordinal. Callers receive value copies only.
type AxisDirectoryEntry struct {
	Key      schema.Key
	Semantic identity.SemanticKey
}

// Axis is a concise alias for AxisDirectoryEntry for callers that refer to a
// directory row as an axis.
type Axis = AxisDirectoryEntry

// Plan is the immutable compiled form of one rule ordinal. A zero-value Plan
// is an explicit absent program; it is not a request to consult a legacy
// fallback.
//
// The slices are private on purpose. Every exported accessor returns a value
// copy, and the compiler never retains a mutable schema or a lookup map.
type Plan struct {
	present    bool
	rule       uint32
	semantic   identity.SemanticKey
	operand    identity.SemanticKey
	inputCount uint32
	candidate  RelationAddr
	// candidateIssued marks the issued-row arm of the candidate choice, and
	// candidateIssuedRow is the issuance relation it names. On that arm
	// candidate stays zero: an issued Program row has no Factor relation, so a
	// reader that takes the address without asking the tag reads nothing.
	candidateIssued    bool
	candidateIssuedRow schema.Key
	reducer            ReducerAddr
	sources            []Source
	foldInputs         []uint32
	joins              []Join
	outputs            []Output
	carry              Carry
	carryPresent       bool
	transports         []Transport
	// activation is the semantic identity of the activation family this plan's
	// candidate branches are grouped under, resolved through the same sealed
	// role vocabulary as semantic and operand. It is absent for a plan that
	// declares no transport vector.
	activation identity.SemanticKey
	// branch is the compiled vocabulary one candidate branch is mounted by. It
	// is present under the same biconditional the vector and the family are.
	branch        Activation
	branchPresent bool
}

// TransportCount is the declared width of this plan's activation transport
// vector. It is zero for a rule that publishes no activation.
func (compiled Plan) TransportCount() int { return len(compiled.transports) }

// TransportAt returns one compiled transport row by its declaration ordinal.
func (compiled Plan) TransportAt(index int) (Transport, bool) {
	if index < 0 || index >= len(compiled.transports) {
		return Transport{}, false
	}
	return compiled.transports[index], true
}

// ActivationFamily is the activation family identity this plan's structural
// publication is admitted under. It is available exactly when the plan carries
// a transport vector, which is the biconditional the Program declares.
func (compiled Plan) ActivationFamily() identity.SemanticKey { return compiled.activation }

// ActivationBranch is the compiled vocabulary one candidate branch is mounted
// by, present under that same biconditional.
func (compiled Plan) ActivationBranch() (Activation, bool) {
	return compiled.branch, compiled.branchPresent
}

// Present reports whether this rule ordinal carried an authored Program.
func (compiled Plan) Present() bool { return compiled.present }

// Available is the non-optional spelling of Present for callers that use the
// common schema availability vocabulary.
func (compiled Plan) Available() bool { return compiled.present }

// Rule is the dense Rule surface ordinal this plan is aligned to.
func (compiled Plan) Rule() uint32 { return compiled.rule }

// Semantic is the rule identity resolved once through the sealed structural
// vocabulary. Runtime construction consumes this value; it never accepts a
// caller-derived identity.
func (compiled Plan) Semantic() identity.SemanticKey { return compiled.semantic }

// OperandFamily is the owner-issued operand identity declared by Program and
// resolved once through the same sealed vocabulary as Semantic.
func (compiled Plan) OperandFamily() identity.SemanticKey { return compiled.operand }

// InputCount is the dense input-port prefix used by this plan.
func (compiled Plan) InputCount() int { return int(compiled.inputCount) }

// Candidate returns the dense candidate relation address. It is zero for an
// absent plan and for a plan on the issued-row arm, which has no relation.
func (compiled Plan) Candidate() RelationAddr { return compiled.candidate }

// IssuedCandidate returns the issuance relation this plan draws its candidate
// rows through, and false when the plan is on the axis-relation arm.
func (compiled Plan) IssuedCandidate() (schema.Key, bool) {
	if !compiled.candidateIssued {
		return "", false
	}
	return compiled.candidateIssuedRow, compiled.candidateIssuedRow.Available()
}

// Reducer returns the owner-issued reducer selected once for this plan.
func (compiled Plan) Reducer() ReducerAddr { return compiled.reducer }

// SourceCount is the number of flattened sources.
func (compiled Plan) SourceCount() int { return len(compiled.sources) }

// SourceAt returns one flattened source by declaration position.
func (compiled Plan) SourceAt(index int) (Source, bool) {
	if index < 0 || index >= len(compiled.sources) {
		return Source{}, false
	}
	return compiled.sources[index], true
}

// JoinCount is the number of joins in declaration order.
func (compiled Plan) JoinCount() int { return len(compiled.joins) }

// JoinAt returns one compiled join by declaration position.
func (compiled Plan) JoinAt(index int) (Join, bool) {
	if index < 0 || index >= len(compiled.joins) {
		return Join{}, false
	}
	return compiled.joins[index], true
}

// FoldInputCount is the number of reducer input join ordinals.
func (compiled Plan) FoldInputCount() int { return len(compiled.foldInputs) }

// FoldInputAt returns one reducer input join ordinal.
func (compiled Plan) FoldInputAt(index int) (uint32, bool) {
	if index < 0 || index >= len(compiled.foldInputs) {
		return 0, false
	}
	return compiled.foldInputs[index], true
}

// OutputCount is the number of reducer outputs.
func (compiled Plan) OutputCount() int { return len(compiled.outputs) }

// OutputAt returns one compiled reducer output by declaration position.
func (compiled Plan) OutputAt(index int) (Output, bool) {
	if index < 0 || index >= len(compiled.outputs) {
		return Output{}, false
	}
	return compiled.outputs[index], true
}

// Carry returns the optional compiled carry. The bool distinguishes an absent
// carry from an identity carry at the zero input/transform address.
func (compiled Plan) Carry() (Carry, bool) {
	if !compiled.carryPresent {
		return Carry{}, false
	}
	return compiled.carry, true
}

// Scratch returns bounded counts for the plan's private flattened storage.
func (compiled Plan) Scratch() ScratchShape {
	return ScratchShape{
		SourceCount:    uint32(len(compiled.sources)),
		JoinCount:      uint32(len(compiled.joins)),
		FoldInputCount: uint32(len(compiled.foldInputs)),
		OutputCount:    uint32(len(compiled.outputs)),
	}
}

// Catalog is the immutable rule-ordinal plan catalog. Plans is aligned to the
// sealed Rule view's dense ordinals, including explicit absent plans. Digest
// is the exact digest issued by the sealed schema; this package derives no
// second identity.
type Catalog struct {
	digest        identity.ContentID
	axisDirectory []AxisDirectoryEntry
	plans         []Plan
}

// Available reports whether the catalog carries a schema fence, a complete
// axis directory, and a complete rule-ordinal plan slice. A zero-length axis
// surface is represented by a non-nil empty directory; failed compilation
// always returns the zero Catalog, whose directory is nil.
func (catalog Catalog) Available() bool {
	return catalog.digest.Available() && completeAxisDirectory(catalog.axisDirectory) && catalog.plans != nil
}

// Digest returns the sealed schema digest that fences this catalog.
func (catalog Catalog) Digest() identity.ContentID { return catalog.digest }

// AxisCount is the number of dense axis ordinals in the sealed axis surface.
func (catalog Catalog) AxisCount() int { return len(catalog.axisDirectory) }

// AxisAt returns one sealed axis declaration key and its resolved semantic
// identity by dense axis ordinal. The row is a value copy, so callers cannot
// mutate the catalog's private directory.
func (catalog Catalog) AxisAt(index int) (AxisDirectoryEntry, bool) {
	if index < 0 || index >= len(catalog.axisDirectory) {
		return AxisDirectoryEntry{}, false
	}
	return catalog.axisDirectory[index], true
}

// Count is the number of rule ordinals represented by this catalog.
func (catalog Catalog) Count() int { return len(catalog.plans) }

// At returns the plan aligned to one rule ordinal. The bool reports bounds;
// an in-range absent Program returns a Plan with Present()==false and true.
func (catalog Catalog) At(index int) (Plan, bool) {
	if index < 0 || index >= len(catalog.plans) {
		return Plan{}, false
	}
	return catalog.plans[index], true
}

// axisEntry is deliberately local and generic-free. Every axis template
// instantiation satisfies this small read-only surface, while the compiler
// remains independent of the composition's authority type parameter.
type axisEntry interface {
	schema.Entry
	Catalog() member.Catalog
	Signature() axis.Signature
	Semantic() schema.Key
	OutputCount() int
	OutputAt(index int) (axis.Output, bool)
}

// Compile compiles every Rule template in schema's sealed Rule view into a
// dense Catalog. The resulting Catalog is fenced by schema.Digest().
func Compile(table *seal.Schema) (Catalog, schema.SealFailure) {
	if table == nil || !table.Available() || !table.Resolver().Complete() {
		failure := compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
		if table != nil {
			failure.Schema = table.Digest()
		}
		return Catalog{}, failure
	}
	reject := func(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) (Catalog, schema.SealFailure) {
		failure := compileFailure(entry, law, disposition)
		failure.Schema = table.Digest()
		return Catalog{}, failure
	}
	axisView, axisOK := table.Surface(schema.SurfaceKindAxis)
	ruleView, ruleOK := table.Surface(schema.SurfaceKindRule)
	denominatorView, denominatorOK := table.Surface(schema.SurfaceKindDenominator)
	if !axisOK || !ruleOK || !denominatorOK {
		return reject(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}
	if !fitsUint32(axisView.Count()) || !fitsUint32(ruleView.Count()) || !fitsUint32(denominatorView.Count()) {
		return reject(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	roles, rolesOK := compileRoles(table)
	if !rolesOK {
		return reject(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}
	axisDirectory, axisDirectoryOK := compileAxisDirectory(axisView, roles)
	if !axisDirectoryOK {
		return reject(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}

	plans := make([]Plan, ruleView.Count())
	for position := 0; position < ruleView.Count(); position++ {
		entry, entryOK := ruleView.At(position)
		template, templateOK := entry.(*rule.Template)
		if !entryOK || !templateOK || template == nil || !template.EntryAvailable() {
			return reject(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
		}
		ordinal, ordinalOK := ruleView.Ordinal(template.ID())
		if !ordinalOK || uint64(ordinal) != uint64(position) {
			return reject(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		declaration := template.Program()
		if !declaration.Available() {
			// The ordinal is intentionally retained as an explicit absent Plan.
			plans[position] = Plan{rule: uint32(position)}
			continue
		}
		compiled, failure := compileProgram(uint32(position), template, declaration, axisView, denominatorView, axisDirectory, roles)
		if failure.Available() {
			failure.Schema = table.Digest()
			return Catalog{}, failure
		}
		plans[position] = compiled
	}
	return Catalog{digest: table.Digest(), axisDirectory: axisDirectory, plans: plans}, schema.SealFailure{}
}

// compileAxisDirectory derives the only axis-to-engine semantic binding proof
// this package is allowed to carry. It reads each axis in sealed view order,
// then resolves the axis's declared semantic role through the structural
// vocabulary and vocabulary package. No role spelling is hashed or rebuilt in
// this compiler, and no caller-provided identity map can enter the result.
func compileAxisDirectory(axisView seal.View, roles vocabulary.Roles) ([]AxisDirectoryEntry, bool) {
	if !axisView.Available() || !roles.Available() || !fitsUint32(axisView.Count()) {
		return nil, false
	}
	directory := make([]AxisDirectoryEntry, axisView.Count())
	if axisView.Count() == 0 {
		return directory, true
	}

	for position := 0; position < axisView.Count(); position++ {
		row, rowOK := axisView.At(position)
		axisRow, axisOK := row.(axisEntry)
		if !rowOK || !axisOK || axisRow == nil || !axisRow.EntryAvailable() {
			return nil, false
		}
		key := axisRow.Key()
		if !key.Available() {
			return nil, false
		}
		axisID := schema.NewEntryID(schema.SurfaceKindAxis, key)
		if axisID == (schema.EntryID{}) {
			return nil, false
		}
		ordinal, ordinalOK := axisView.Ordinal(axisID)
		if !ordinalOK || uint64(ordinal) != uint64(position) {
			return nil, false
		}
		semantic, semanticOK := roles.Key(axisRow.Semantic())
		if !semanticOK || !semantic.Available() {
			return nil, false
		}
		directory[position] = AxisDirectoryEntry{Key: key, Semantic: semantic}
	}
	if !completeAxisDirectory(directory) {
		return nil, false
	}
	return directory, true
}

func compileRoles(table *seal.Schema) (vocabulary.Roles, bool) {
	if table == nil {
		return vocabulary.Roles{}, false
	}
	structureView, structureOK := table.Surface(schema.SurfaceKindStructure)
	if !structureOK {
		return vocabulary.Roles{}, false
	}
	structuralEntries := structureView.Entries()
	entries := make([]*structure.Entry, 0, len(structuralEntries))
	for _, row := range structuralEntries {
		entry, entryOK := row.(*structure.Entry)
		if !entryOK || entry == nil || !entry.EntryAvailable() {
			return vocabulary.Roles{}, false
		}
		entries = append(entries, entry)
	}
	return vocabulary.NewRoles(entries)
}

func completeAxisDirectory(directory []AxisDirectoryEntry) bool {
	if directory == nil {
		return false
	}
	for _, row := range directory {
		if !row.Key.Available() || !row.Semantic.Available() {
			return false
		}
	}
	return true
}

func compileFailure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, law, disposition)
}

func compileProgram(ruleOrdinal uint32, template *rule.Template, declaration program.Program, axisView, denominatorView seal.View, axisDirectory []AxisDirectoryEntry, roles vocabulary.Roles) (Plan, schema.SealFailure) {
	if problem, valid := declaration.Check(); !valid {
		law := rule.LawProgramShape
		if problem.Kind == program.ProblemOutput {
			law = rule.LawProgramOutput
		}
		return Plan{}, compileFailure(template.ID(), law, schema.DispositionMalformed)
	}
	if !declaration.Candidate.Available() {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	// The reducer is resolved before the candidate because the two arms of the
	// candidate choice reach the candidate carrier by different routes. An
	// axis relation states its own subject; an issued Program row has no axis
	// owner to state one, and its carrier is the one the owner reducer already
	// declared it consumes. Resolving the reducer once, here, keeps that a
	// single lookup rather than a second resolution beside the fold.
	if !declaration.Fold.Reducer.Available() {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	reducerAxis, reducerCatalog, reducerAxisOrdinal, failure := resolveAxisMember(axisView, declaration.Fold.Reducer.Axis, declaration.Fold.Reducer.Member, memberReducer)
	if failure.Available() {
		failure.Entry = template.ID()
		return Plan{}, failure
	}
	reducer, reducerOK := reducerCatalog.Reducer(declaration.Fold.Reducer.Member)
	if !reducerOK {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
	}
	if reducerAxis.Key() != template.Writes() {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}

	// candidateCarrier is the typed row a join's first input and a carry
	// transform are held to. candidateRelation is the axis arm's own row, and
	// stays absent on the issued arm: an issued Program row has no Factor
	// relation, so nothing downstream may read one off it.
	var candidateCarrier member.Carrier
	var candidateRelation member.Relation
	var candidateAxis axisEntry
	var candidateCatalog member.Catalog
	var candidateOrdinal uint32
	if !declaration.Candidate.Issued() {
		var candidateFailure schema.SealFailure
		candidateAxis, candidateCatalog, candidateOrdinal, candidateFailure = resolveAxisMember(
			axisView, declaration.Candidate.AxisRelation.Axis, declaration.Candidate.AxisRelation.Member, memberRelation,
		)
		if candidateFailure.Available() {
			candidateFailure.Entry = template.ID()
			return Plan{}, candidateFailure
		}
		var candidateRelationOK bool
		candidateRelation, candidateRelationOK = candidateCatalog.Relation(declaration.Candidate.AxisRelation.Member)
		if !candidateRelationOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
		}
		if len(candidateRelation.Inputs) != 0 {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		candidateCarrier = candidateRelation.Subject
	}
	// On the issued arm candidateCarrier is still absent here. It is witnessed
	// by the first join input the declaration sources from the candidate, and
	// every later candidate-sourced input is held to that same witness. A
	// Program row has no axis owner to state its carrier, so the declaration's
	// own agreement is the statement; disagreement refuses.
	ruleSemantic, ruleSemanticOK := roles.Key(template.Semantic())
	operandSemantic, operandSemanticOK := roles.Key(declaration.OperandRole)
	if !ruleSemanticOK || !operandSemanticOK || !ruleSemantic.Available() || !operandSemantic.Available() {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
	}
	compiled := Plan{
		present: true, rule: ruleOrdinal, semantic: ruleSemantic, operand: operandSemantic,
	}
	if declaration.Candidate.Issued() {
		compiled.candidateIssued = true
		compiled.candidateIssuedRow = declaration.Candidate.IssuedRow
	} else {
		compiled.candidate = RelationAddr{Axis: candidateOrdinal, Member: mustRelationOrdinal(candidateCatalog, declaration.Candidate.AxisRelation.Member)}
	}
	if !fitsUint32(declaration.InputCount()) {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	compiled.inputCount = uint32(declaration.InputCount())
	joinFacts := make([]member.Carrier, 0, declaration.JoinCount())
	joinTags := make([]member.Carrier, 0, declaration.JoinCount())

	if !fitsUint32(declaration.JoinCount()) || !fitsUint32(len(declaration.Fold.Inputs)) || !fitsUint32(len(declaration.Fold.Outputs)) {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	for joinIndex := 0; joinIndex < declaration.JoinCount(); joinIndex++ {
		join, joinOK := declaration.JoinAt(joinIndex)
		if !joinOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		if !fitsUint32(len(join.Sources)) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		sourceStart := len(compiled.sources)
		if !fitsUint32(sourceStart) || !fitsUint32(sourceStart+len(join.Sources)) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		relationAxis, relationCatalog, relationAxisOrdinal, failure := resolveAxisMember(axisView, join.Relation.Axis, join.Relation.Member, memberRelation)
		if failure.Available() {
			failure.Entry = template.ID()
			return Plan{}, failure
		}
		relation, relationOK := relationCatalog.Relation(join.Relation.Member)
		if !relationOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
		}
		if len(relation.Inputs) != len(join.Sources) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		addressingRef, addressingPresent, addressed := joinAddressingDirectory(axisView, declaration, relationCatalog, relation, join.Read.Form)
		if !addressed {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		for sourceIndex, source := range join.Sources {
			if source.Position > maxUint32 {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			carrier := candidateCarrier
			if !source.Candidate {
				if source.Position >= uint64(len(joinFacts)) {
					return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
				}
				carrier = joinFacts[source.Position]
			} else if !carrier.Available() {
				carrier = relation.Inputs[sourceIndex]
				if !carrier.Available() {
					return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
				}
				candidateCarrier = carrier
			}
			if relation.Inputs[sourceIndex] != carrier {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			compiled.sources = append(compiled.sources, Source{Position: uint32(source.Position), Candidate: source.Candidate})
		}

		keyAxis, keyCatalog, keyAxisOrdinal, failure := resolveAxisMember(axisView, join.Key.Axis, join.Key.Member, memberProjection)
		if failure.Available() {
			failure.Entry = template.ID()
			return Plan{}, failure
		}
		key, keyOK := keyCatalog.Projection(join.Key.Member)
		if !keyOK || key.Role != member.Key || key.Relation != relation.Key || keyAxisOrdinal != relationAxisOrdinal || keyAxis.Key() != relationAxis.Key() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}

		compiledJoin := Join{
			Sources:    Span{Start: uint32(sourceStart), Count: uint32(len(join.Sources))},
			Input:      uint32(join.Read.Input),
			Relation:   RelationAddr{Axis: relationAxisOrdinal, Member: mustRelationOrdinal(relationCatalog, join.Relation.Member)},
			Key:        ProjectionAddr{Axis: keyAxisOrdinal, Member: mustProjectionOrdinal(keyCatalog, join.Key.Member)},
			ReadForm:   join.Read.Form,
			PointBound: join.Read.PointBound,
			ReadContract: ReadContract{
				Order: join.Read.Contract.Order, Sparse: join.Read.Contract.Sparse,
				OnOpaque: join.Read.Contract.OnOpaque, Multiplicity: join.Read.Contract.Multiplicity,
			},
			Cardinality: join.Read.Contract.Multiplicity,
		}

		// The addressing directory is compiled in the same dense coordinates
		// every other member address is, so a consumer can compare it with the
		// rule's own candidate relation without reopening a catalog.
		if addressingPresent {
			_, addressingCatalog, addressingAxisOrdinal, addressingFailure := resolveAxisMember(axisView, addressingRef.Axis, addressingRef.Member, memberRelation)
			if addressingFailure.Available() {
				addressingFailure.Entry = template.ID()
				return Plan{}, addressingFailure
			}
			if _, addressingOK := addressingCatalog.Relation(addressingRef.Member); !addressingOK {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
			}
			compiledJoin.AddressingPresent = true
			compiledJoin.Addressing = RelationAddr{Axis: addressingAxisOrdinal, Member: mustRelationOrdinal(addressingCatalog, addressingRef.Member)}
		}

		// The declared Parent is authenticated against the relation's own
		// sealed Parent, exactly as Key and Predicate are authenticated
		// against the relation they name. A restatement that names a
		// different relation, or one made over a relation that is not a
		// nested member set at all, is a declaration disagreeing with the
		// catalog it addresses.
		if join.Parent.Declared() {
			if !relation.Nested() || relation.Parent != join.Parent {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			parentAxis, parentCatalog, parentAxisOrdinal, parentFailure := resolveAxisMember(axisView, join.Parent.Axis, join.Parent.Member, memberRelation)
			if parentFailure.Available() {
				parentFailure.Entry = template.ID()
				return Plan{}, parentFailure
			}
			if _, parentOK := parentCatalog.Relation(join.Parent.Member); !parentOK || parentAxisOrdinal != relationAxisOrdinal || parentAxis.Key() != relationAxis.Key() {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			compiledJoin.ParentPresent = true
			compiledJoin.Parent = RelationAddr{Axis: parentAxisOrdinal, Member: mustRelationOrdinal(parentCatalog, join.Parent.Member)}
		}

		// The declared KeyVector is authenticated the same way: the directory
		// it names must be the relation's own candidate provider, and that
		// directory must actually publish a key vector. A read cannot borrow a
		// span from a row that carries none, and it cannot take one from a
		// directory it is not joined from.
		if join.KeyVector.Declared() {
			if relation.Nested() || relation.CandidateProvider.AxisRelation != join.KeyVector {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			keyVectorAxis, keyVectorCatalog, keyVectorAxisOrdinal, keyVectorFailure := resolveAxisMember(axisView, join.KeyVector.Axis, join.KeyVector.Member, memberRelation)
			if keyVectorFailure.Available() {
				keyVectorFailure.Entry = template.ID()
				return Plan{}, keyVectorFailure
			}
			publisher, publisherOK := keyVectorCatalog.Relation(join.KeyVector.Member)
			if !publisherOK || !publisher.PublishesKeyVector || keyVectorAxis.Key() != join.KeyVector.Axis.Key {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			compiledJoin.KeyVectorPresent = true
			compiledJoin.KeyVector = RelationAddr{Axis: keyVectorAxisOrdinal, Member: mustRelationOrdinal(keyVectorCatalog, join.KeyVector.Member)}
		}

		if join.Predicate.Declared() {
			predicateAxis, predicateCatalog, predicateAxisOrdinal, predicateFailure := resolveAxisMember(axisView, join.Predicate.Axis, join.Predicate.Member, memberProjection)
			if predicateFailure.Available() {
				predicateFailure.Entry = template.ID()
				return Plan{}, predicateFailure
			}
			predicate, predicateOK := predicateCatalog.Projection(join.Predicate.Member)
			if !predicateOK || predicate.Role != member.Predicate || predicate.Relation != relation.Key || predicateAxisOrdinal != relationAxisOrdinal || predicateAxis.Key() != relationAxis.Key() {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			compiledJoin.PredicatePresent = true
			compiledJoin.Predicate = ProjectionAddr{Axis: predicateAxisOrdinal, Member: mustProjectionOrdinal(predicateCatalog, join.Predicate.Member)}
		}

		readAxis, readAxisOrdinal, readFailure := resolveAxis(axisView, join.Read.Axis.EntryReference())
		if readFailure.Available() {
			readFailure.Entry = template.ID()
			return Plan{}, readFailure
		}
		readSignature := readAxis.Signature()
		if !readSignature.Available() || key.Result != readSignature.Key {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		// The tag names WHICH member of a many-valued delivery each cell is.
		// A selection is tagged by its Predicate projection. A nested member
		// set has no predicate and needs none: it is addressed by (parent,
		// ordinal), and the relation's own declared Ordinal carrier IS that
		// address - "what a CHILD Program consumes", in the catalog's words.
		// Deriving the tag from only the predicate left a member set foldable
		// by nothing, because its reducer had no carrier to agree with.
		var tag member.Carrier
		switch {
		case compiledJoin.PredicatePresent:
			predicate, _ := relationCatalog.Projection(join.Predicate.Member)
			tag = predicate.Result
		case compiledJoin.ParentPresent:
			tag = relation.Ordinal
		}
		joinFacts = append(joinFacts, readSignature.Fact)
		joinTags = append(joinTags, tag)
		compiledJoin.ReadAxis = readAxisOrdinal
		if !join.Read.Form.Available() || !join.Read.Contract.Available() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		denominatorAddress, denominatorFailure := compileDenominator(denominatorView, join.Read.Axis.EntryReference(), join.Read.Contract.DenominatorRef)
		if denominatorFailure.Available() {
			denominatorFailure.Entry = template.ID()
			return Plan{}, denominatorFailure
		}
		compiledJoin.Denominator = denominatorAddress
		compiled.joins = append(compiled.joins, compiledJoin)
	}

	// The arity and the per-position axis, form and multiplicity of a fold's
	// call are the owner reducer's statement, and a declaration package can
	// close that gate against its own catalog before any schema exists. It is
	// applied here through the same implementation, so the two cannot drift.
	if _, agrees := declaration.CheckAgainst(reducer); !agrees {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	compiled.foldInputs = make([]uint32, len(declaration.Fold.Inputs))
	for inputIndex, input := range declaration.Fold.Inputs {
		if uint64(input) >= uint64(len(compiled.joins)) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		compiled.foldInputs[inputIndex] = uint32(input)
		// Which carrier a join yields, and which tag names its members, are the
		// joined axis's statement rather than the declaration's, so they are
		// resolved here and nowhere earlier.
		signature := reducer.Inputs[inputIndex]
		if signature.Carrier != joinFacts[input] || signature.Tag != joinTags[input] {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
	}
	compiled.reducer = ReducerAddr{Axis: reducerAxisOrdinal, Member: mustReducerOrdinal(reducerCatalog, declaration.Fold.Reducer.Member)}

	var outputFact member.Carrier
	for _, output := range declaration.Fold.Outputs {
		if !output.Available() || uint64(output.ValueSlot) >= uint64(len(declaration.Fold.Outputs)) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		outputAxis, outputCatalog, outputAxisOrdinal, outputFailure := resolveAxisMember(axisView, output.Column.Axis, output.Column.Key, memberOutput)
		if outputFailure.Available() {
			outputFailure.Entry = template.ID()
			outputFailure.Law = rule.LawProgramOutput
			return Plan{}, outputFailure
		}
		if output.Column.Axis.Key != template.Writes() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		_ = outputCatalog
		outputSignature := outputAxis.Signature()
		if !outputSignature.Available() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		outputFact = outputSignature.Fact
		// A structural publication declares no output CARRIER - its fold
		// concludes a disposition, not a fact - so there is no reducer row for
		// the column to agree with. The column itself is still resolved above:
		// a structural row is indexed by it even though it writes nothing into
		// it. FoldDecl.checkAgainst already holds the two arities to their
		// biconditional, so an empty list here is the structural case and
		// never an ordinary reducer that lost a row.
		if output.Mode != program.ModeStructural {
			if uint64(output.ValueSlot) >= uint64(len(reducer.Outputs)) {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
			}
			reducerOutput := reducer.Outputs[output.ValueSlot]
			if reducerOutput.Axis != output.Column.Axis || reducerOutput.Carrier != outputSignature.Fact {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
			}
		} else if len(reducer.Outputs) != 0 {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		frameIndex, frameOK, writerOK := findOutput(outputAxis, output.Column.Key, template.Writes())
		if !frameOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionIncomplete)
		}
		if !writerOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}

		routeJoin, routeJoinPresent, routeOK := routeJoinForOutput(declaration, output, compiled.joins)
		if output.Mode == program.ModeRoute && (!routeOK || !routeJoinPresent) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		destinationAxis, destinationCatalog, destinationAxisOrdinal, destinationFailure := resolveAxisMember(axisView, output.Destination.Axis, output.Destination.Member, memberProjection)
		if destinationFailure.Available() {
			destinationFailure.Entry = template.ID()
			destinationFailure.Law = rule.LawProgramOutput
			return Plan{}, destinationFailure
		}
		destination, destinationOK := destinationCatalog.Projection(output.Destination.Member)
		if !destinationOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionIncomplete)
		}
		// An exact write is one cell per candidate row, addressed through the
		// candidate relation's own coordinate space. An issued Program row has
		// no such space, so the exact form cannot be written down for it and
		// refuses here rather than borrowing another relation's coordinates.
		if output.Mode == program.ModeExact && declaration.Candidate.Issued() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		var destinationRelation schema.Key
		var destinationOwnerOrdinal uint32
		var destinationOwnerKey schema.Key
		if !declaration.Candidate.Issued() {
			destinationRelation = candidateRelation.Key
			destinationOwnerOrdinal = candidateOrdinal
			destinationOwnerKey = candidateAxis.Key()
		}
		if output.Mode == program.ModeRoute {
			routeJoinDeclaration := declaration.Joins[routeJoin]
			routeRelationAxis, routeRelationCatalog, routeRelationAxisOrdinal, routeRelationFailure := resolveAxisMember(axisView, routeJoinDeclaration.Relation.Axis, routeJoinDeclaration.Relation.Member, memberRelation)
			if routeRelationFailure.Available() {
				routeRelationFailure.Entry = template.ID()
				routeRelationFailure.Law = rule.LawProgramOutput
				return Plan{}, routeRelationFailure
			}
			routeRelation, routeRelationOK := routeRelationCatalog.Relation(routeJoinDeclaration.Relation.Member)
			if !routeRelationOK {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionIncomplete)
			}
			destinationRelation = routeRelation.Key
			destinationOwnerOrdinal = routeRelationAxisOrdinal
			destinationOwnerKey = routeRelationAxis.Key()
		}
		if destination.Role != member.Destination || destination.Result != outputSignature.Key {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		sameOwner := destination.Relation == destinationRelation && destinationAxisOrdinal == destinationOwnerOrdinal && destinationAxis.Key() == destinationOwnerKey
		if !sameOwner && !consumerProjectionOfCandidate(template, declaration, destinationAxis, destinationCatalog, destination, output.Mode) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramOutput, schema.DispositionMalformed)
		}
		compiled.outputs = append(compiled.outputs, Output{
			Address:          OutputAddr{Axis: outputAxisOrdinal, Frame: frameIndex},
			Destination:      ProjectionAddr{Axis: destinationAxisOrdinal, Member: mustProjectionOrdinal(destinationCatalog, output.Destination.Member)},
			Mode:             output.Mode,
			Slot:             uint32(output.ValueSlot),
			RouteJoin:        routeJoin,
			RouteJoinPresent: routeJoinPresent,
		})
	}

	if declaration.Carry != nil {
		// Program.Check proves a contiguous union of read/carry ports. Keep the
		// ordinal explicit at this boundary as well: a carry may not name a
		// port outside the sealed prefix, and no zero/default port is inferred.
		if uint64(declaration.Carry.Input) >= uint64(declaration.InputCount()) {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		compiled.carry = Carry{Input: uint32(declaration.Carry.Input), Mode: declaration.Carry.Mode}
		compiled.carryPresent = true
		if declaration.Carry.Mode == program.CarryTransform {
			transformAxis, transformCatalog, transformAxisOrdinal, transformFailure := resolveAxisMember(axisView, declaration.Carry.Transform.Axis, declaration.Carry.Transform.Member, memberCarryTransform)
			if transformFailure.Available() {
				transformFailure.Entry = template.ID()
				return Plan{}, transformFailure
			}
			transform, transformOK := transformCatalog.CarryTransform(declaration.Carry.Transform.Member)
			transformOrdinal, transformOrdinalOK := transformCatalog.CarryTransformOrdinal(declaration.Carry.Transform.Member)
			resolvedTransform, resolvedTransformOK := transformCatalog.CarryTransformAt(int(transformOrdinal))
			if !transformOK || !transformOrdinalOK || !resolvedTransformOK || resolvedTransform.Key != declaration.Carry.Transform.Member || transformAxis.Key() != template.Writes() || transform.Candidate != candidateCarrier || transform.Input != outputFact || transform.Output != outputFact {
				return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
			}
			compiled.carry.Transform = CarryTransformAddr{Axis: transformAxisOrdinal, Member: transformOrdinal}
			compiled.carry.TransformPresent = true
			compiled.carry.TransformAxis = transformAxis.Key()
			compiled.carry.TransformKey = resolvedTransform.Key
		}
	}

	for transportIndex := 0; transportIndex < declaration.TransportCount(); transportIndex++ {
		transport, transportOK := declaration.TransportAt(transportIndex)
		if !transportOK {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
		}
		_, transportAxisOrdinal, transportFailure := resolveAxis(axisView, transport.Axis.EntryReference())
		if transportFailure.Available() {
			transportFailure.Entry = template.ID()
			return Plan{}, transportFailure
		}
		compiled.transports = append(compiled.transports, Transport{Axis: transportAxisOrdinal, Exported: transport.Exported})
	}
	if len(compiled.transports) != 0 {
		// The family is resolved through the same role vocabulary as the rule
		// and operand identities above. Program.Check already holds the
		// declaration to the biconditional between the vector and the role, so
		// a vector that reaches here without a resolvable family is a role the
		// composition did not publish.
		activationSemantic, activationSemanticOK := roles.Key(declaration.ActivationRole)
		if !activationSemanticOK || !activationSemantic.Available() {
			return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionIncomplete)
		}
		compiled.activation = activationSemantic
		branch, branchFailure := compileActivationBranch(axisView, template, declaration, compiled)
		if branchFailure.Available() {
			return Plan{}, branchFailure
		}
		compiled.branch, compiled.branchPresent = branch, true
	}

	if !fitsUint32(len(compiled.sources)) || !fitsUint32(len(compiled.joins)) || !fitsUint32(len(compiled.foldInputs)) || !fitsUint32(len(compiled.outputs)) || !fitsUint32(len(compiled.transports)) {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	if !planAxesInRange(compiled, len(axisDirectory)) {
		return Plan{}, compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	return compiled, schema.SealFailure{}
}

// compileActivationBranch resolves the branch vocabulary of a structural
// publication into dense projection addresses.
//
// Each projection is authenticated against the relation it must belong to, the
// same way a Key or a Predicate is: the application is a column of the rule's
// own candidate relation, because every branch of one trigger is an
// alternative of the same application, and the other four are columns of the
// branch relation, because they are what distinguishes one branch from
// another. Every one of them must be declared in the member.Identity role - a
// local projected here would be a dense coordinate standing in for a module or
// a semantic axis.
func compileActivationBranch(axisView seal.View, template *rule.Template, declaration program.Program, compiled Plan) (Activation, schema.SealFailure) {
	malformed := func() schema.SealFailure {
		return compileFailure(template.ID(), rule.LawProgramShape, schema.DispositionMalformed)
	}
	if declaration.Activation == nil {
		return Activation{}, malformed()
	}
	decl := *declaration.Activation
	branchJoin, branchJoinOK := declaration.JoinAt(int(decl.Branch))
	if !branchJoinOK || !branchJoin.Parent.Declared() {
		return Activation{}, malformed()
	}
	identityColumn := func(reference member.ProjectionRef, relation schema.Key) (ProjectionAddr, schema.SealFailure) {
		axisRow, catalog, axisOrdinal, failure := resolveAxisMember(axisView, reference.Axis, reference.Member, memberProjection)
		if failure.Available() {
			failure.Entry = template.ID()
			return ProjectionAddr{}, failure
		}
		projection, projectionOK := catalog.Projection(reference.Member)
		if !projectionOK || projection.Role != member.Identity || projection.Relation != relation ||
			axisRow.Key() != declaration.Candidate.AxisRelation.Axis.Key {
			return ProjectionAddr{}, malformed()
		}
		return ProjectionAddr{Axis: axisOrdinal, Member: mustProjectionOrdinal(catalog, reference.Member)}, schema.SealFailure{}
	}
	if uint64(decl.Branch) > uint64(^uint32(0)) {
		return Activation{}, malformed()
	}
	branch := Activation{Branch: uint32(decl.Branch)}
	for _, column := range []struct {
		reference member.ProjectionRef
		relation  schema.Key
		address   *ProjectionAddr
	}{
		{decl.Application, declaration.Candidate.AxisRelation.Member, &branch.Application},
		{decl.Target, branchJoin.Relation.Member, &branch.Target},
		{decl.Endpoint, branchJoin.Relation.Member, &branch.Endpoint},
		{decl.Mount, branchJoin.Relation.Member, &branch.Mount},
		{decl.Body, branchJoin.Relation.Member, &branch.Body},
	} {
		address, failure := identityColumn(column.reference, column.relation)
		if failure.Available() {
			return Activation{}, failure
		}
		*column.address = address
	}
	if uint64(branch.Branch) >= uint64(len(compiled.joins)) {
		return Activation{}, malformed()
	}
	return branch, schema.SealFailure{}
}

// planAxesInRange is the final address fence between the dense compiler and
// the private axis directory. resolveAxis already proves each authored
// reference is present in axisView; this second check states explicitly that
// every emitted Plan ordinal is also bounded by the exact directory retained
// by Catalog.
func planAxesInRange(compiled Plan, axisCount int) bool {
	if axisCount < 0 {
		return false
	}
	inRange := func(ordinal uint32) bool { return uint64(ordinal) < uint64(axisCount) }
	if !inRange(compiled.candidate.Axis) || !inRange(compiled.reducer.Axis) {
		return false
	}
	for _, join := range compiled.joins {
		if !inRange(join.Relation.Axis) || !inRange(join.Key.Axis) || !inRange(join.ReadAxis) {
			return false
		}
		if join.PredicatePresent && !inRange(join.Predicate.Axis) {
			return false
		}
		if join.ParentPresent && !inRange(join.Parent.Axis) {
			return false
		}
	}
	for _, output := range compiled.outputs {
		if !inRange(output.Address.Axis) || !inRange(output.Destination.Axis) {
			return false
		}
	}
	if compiled.carryPresent && compiled.carry.TransformPresent && !inRange(compiled.carry.Transform.Axis) {
		return false
	}
	for _, transport := range compiled.transports {
		if !inRange(transport.Axis) {
			return false
		}
	}
	return true
}

// consumerProjectionOfCandidate admits the second exact-write normal form: a
// rule whose candidate is a foreign axis's sealed occurrence relation writing
// its OWN sealed projection of that same candidate.
//
// The first normal form - destination projection on the candidate relation
// itself - only covers a rule that writes the axis its candidate belongs to. A
// consumer keyed on another axis's occurrences has a coordinate space of its
// own, and the cell it writes is a projection its own owner declared for that
// candidate. That write is still exactly one cell per candidate row, which is
// what ModeExact means, so it needs neither a selected join nor a denominator:
// the candidate directory is the denominator.
//
// The fence is that the projection is the consumer's own and is declared for
// this exact candidate. A projection of some other relation, or one owned by a
// third axis, is refused as before.
func consumerProjectionOfCandidate(template *rule.Template, declaration program.Program, destinationAxis axisEntry, destinationCatalog member.Catalog, destination member.Projection, mode program.OutputMode) bool {
	if mode != program.ModeExact || destinationAxis == nil || declaration.Candidate.Issued() {
		return false
	}
	if destinationAxis.Key() != template.Writes() {
		return false
	}
	if declaration.Candidate.AxisRelation.Axis.Key == destinationAxis.Key() {
		// A same-axis candidate is already the first normal form; reaching here
		// means the projection is declared for some other relation of this axis.
		return false
	}
	relation, relationOK := destinationCatalog.Relation(destination.Relation)
	if !relationOK {
		return false
	}
	return relation.CandidateProvider == declaration.Candidate && destination.CandidateProvider == declaration.Candidate
}

// routeJoinForOutput validates the explicit route-producing JoinRef. It never
// infers a source from selected-join count or fact equality. A selected Many
// read is deliberately refused: it would make the Delta write width unbounded
// even when a denominator row is present.
func routeJoinForOutput(declaration program.Program, output program.OutputDecl, joins []Join) (uint32, bool, bool) {
	if output.Mode != program.ModeRoute {
		return 0, false, true
	}
	if !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(len(declaration.Joins)) || uint64(output.RouteJoin) >= uint64(len(joins)) {
		return 0, false, false
	}
	routeJoin := declaration.Joins[output.RouteJoin]
	if routeJoin.Read.Form != program.Selected || routeJoin.Read.Contract.Multiplicity == program.MultiplicityMany || !joins[output.RouteJoin].Denominator.Present {
		return 0, false, false
	}
	for _, input := range declaration.Fold.Inputs {
		if input == output.RouteJoin {
			return uint32(output.RouteJoin), true, true
		}
	}
	return 0, false, false
}

type memberKind uint8

const (
	memberRelation memberKind = iota + 1
	memberProjection
	memberReducer
	memberOutput
	memberCarryTransform
)

func resolveAxis(view seal.View, reference schema.EntryReference) (axisEntry, uint32, schema.SealFailure) {
	if reference.Surface != schema.SurfaceKindAxis || !reference.Key.Available() {
		return nil, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	entry, ok := view.ByID(schema.NewEntryID(schema.SurfaceKindAxis, reference.Key))
	if !ok {
		return nil, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}
	axisRow, ok := entry.(axisEntry)
	if !ok || axisRow == nil || !axisRow.EntryAvailable() {
		return nil, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	ordinal, ok := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, reference.Key))
	if !ok {
		return nil, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	return axisRow, ordinal, schema.SealFailure{}
}

func resolveAxisMember(view seal.View, reference schema.EntryReference, key schema.Key, kind memberKind) (axisEntry, member.Catalog, uint32, schema.SealFailure) {
	if key == "" {
		return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	axisRow, ordinal, failure := resolveAxis(view, reference)
	if failure.Available() {
		return nil, member.Catalog{}, 0, failure
	}
	catalog := axisRow.Catalog()
	if !catalog.Available() {
		return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}
	switch kind {
	case memberRelation:
		if _, ok := catalog.Relation(key); !ok {
			return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
		}
	case memberProjection:
		if _, ok := catalog.Projection(key); !ok {
			return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
		}
	case memberReducer:
		if _, ok := catalog.Reducer(key); !ok {
			return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
		}
	case memberOutput:
		// Output columns live in the axis frame, not in member.Catalog.
	case memberCarryTransform:
		if _, ok := catalog.CarryTransform(key); !ok {
			return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
		}
	default:
		return nil, member.Catalog{}, 0, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	return axisRow, catalog, ordinal, schema.SealFailure{}
}

func compileDenominator(view seal.View, readAxis schema.EntryReference, reference program.DenominatorRef) (DenominatorAddr, schema.SealFailure) {
	if !reference.Declared() {
		return DenominatorAddr{}, schema.SealFailure{}
	}
	if !reference.Available() {
		return DenominatorAddr{}, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	entry, ok := view.ByID(schema.NewEntryID(schema.SurfaceKindDenominator, reference.EntryReference().Key))
	if !ok {
		return DenominatorAddr{}, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionIncomplete)
	}
	closed, ok := entry.(*denominator.Entry)
	if !ok || closed == nil || !closed.EntryAvailable() {
		return DenominatorAddr{}, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	owner := closed.Owner()
	if owner.Surface != schema.SurfaceKindAxis || owner.Entry != readAxis.Key || closed.Phase() != denominator.PhasePublication {
		return DenominatorAddr{}, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	ordinal, ok := view.Ordinal(closed.ID())
	if !ok {
		return DenominatorAddr{}, compileFailure(schema.EntryID{}, rule.LawProgramShape, schema.DispositionMalformed)
	}
	return DenominatorAddr{Ordinal: ordinal, Present: true}, schema.SealFailure{}
}

func findOutput(axisRow axisEntry, key, writer schema.Key) (uint32, bool, bool) {
	count := axisRow.OutputCount()
	if !fitsUint32(count) {
		return 0, false, false
	}
	for index := 0; index < count; index++ {
		output, ok := axisRow.OutputAt(index)
		if !ok || !output.Available() {
			return 0, false, false
		}
		if output.Key == key {
			return uint32(index), true, output.Writer == writer
		}
	}
	return 0, false, false
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= maxUint32 }

func mustRelationOrdinal(catalog member.Catalog, key schema.Key) uint32 {
	ordinal, _ := catalog.RelationOrdinal(key)
	return ordinal
}

func mustProjectionOrdinal(catalog member.Catalog, key schema.Key) uint32 {
	ordinal, _ := catalog.ProjectionOrdinal(key)
	return ordinal
}

func mustReducerOrdinal(catalog member.Catalog, key schema.Key) uint32 {
	ordinal, _ := catalog.ReducerOrdinal(key)
	return ordinal
}

func mustCarryTransformOrdinal(catalog member.Catalog, key schema.Key) uint32 {
	ordinal, _ := catalog.CarryTransformOrdinal(key)
	return ordinal
}

// joinAddressingDirectory is the one candidate-authority law of a join, and
// the compiled statement of which directory the join's ordinal comes from.
//
// A rule resolves one dense candidate and every relation it joins is addressed
// with that ordinal. Which directory issued the ordinal a join is indexed by
// is a property of the join: a whole-vector read over a nested member set is
// indexed by the PARENT's ordinal, because the owner resolves the parent row
// and enumerates its members from it, and every other candidate-addressed read
// is indexed by the relation's own directory. Either way that directory must
// be the one the rule's candidate came from, or the two must be declared to
// enumerate the same subjects.
//
// Without this, two axes describing one subject enumerate independently and a
// join across their orders compiles and resolves against the wrong row. The
// only thing that refused it was a generated owner having no case to answer
// with, which is a refusal by absence rather than a law.
//
// The directory is RETURNED rather than discarded once the law holds. A
// declared correspondence says the two orders enumerate the same subjects, not
// that they enumerate them in the same positions - the shared address is the
// occurrence both directories are already addressed by - so a consumer that
// has to resolve the corresponded row needs to know which directory to resolve
// it in.
func joinAddressingDirectory(axisView seal.View, declaration program.Program, catalog member.Catalog, relation member.Relation, form program.ReadForm) (member.RelationRef, bool, bool) {
	if !program.ReadFormCandidateAddressed(form) {
		// A selected read is addressed by the selection its own family
		// resolves, and a closed-denominator read spans the denominator rather
		// than one candidate's rows, so no directory of this join is indexed by
		// the rule's ordinal and there is nothing to correspond.
		return member.RelationRef{}, false, true
	}
	addressing := relation
	if form == program.Summary && relation.Nested() {
		// A whole-vector read enumerates one parent row's members, so the
		// ordinal it is given is the PARENT's.
		parent, parentOK := catalog.Relation(relation.Parent.Member)
		if !parentOK {
			return member.RelationRef{}, false, false
		}
		addressing = parent
	}
	if addressing.CandidateProvider == declaration.Candidate {
		// An issued candidate is a Program row, not a relation: the join
		// borrows the rule's own mounted row and there is no directory to name.
		if declaration.Candidate.Issued() {
			return member.RelationRef{}, false, true
		}
		return declaration.Candidate.AxisRelation, true, true
	}
	// An issued candidate is a Program row, not a relation, so there is no
	// foreign order for a correspondence to name: a join addressed by another
	// authority has nothing that could pair the two.
	if declaration.Candidate.Issued() || !addressing.CandidateProvider.Available() || addressing.CandidateProvider.Issued() {
		return member.RelationRef{}, false, false
	}
	// The correspondence belongs to the relation that OWNS the directory being
	// indexed, which is the one a self-provided authority names, not the row
	// that borrows it.
	directory := addressing.CandidateProvider.AxisRelation
	_, directoryCatalog, _, failure := resolveAxisMember(axisView, directory.Axis, directory.Member, memberRelation)
	if failure.Available() {
		return member.RelationRef{}, false, false
	}
	owner, ownerOK := directoryCatalog.Relation(directory.Member)
	if !ownerOK {
		return member.RelationRef{}, false, false
	}
	for _, correspondence := range owner.Correspondences {
		if correspondence == declaration.Candidate.AxisRelation {
			return directory, true, true
		}
	}
	return member.RelationRef{}, false, false
}
