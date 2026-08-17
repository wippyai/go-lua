package composite

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	denominatorcounts "github.com/wippyai/go-lua/analysis/schema/denominator"
	denominatorpublication "github.com/wippyai/go-lua/analysis/schema/denominator/publication"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	executionowner "github.com/wippyai/go-lua/domain/execution/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// The materializer is the composition's publication driver: it walks the
// issuance requests the sealed table states, drives each writer's own
// contributor over the authority that writer's axis mounted, and seals one
// published value out of the whole catalog at once.
//
// It is composition-wide by construction. The requests it walks are the sealed
// table's own, and the sealed table is this package's; nothing below it can see
// the catalog, so the walk cannot live in a domain or in the engine. What lives
// below it is every decision about content: a column's rows and the key
// universe it is total over are the owning domain's contributor's statement,
// the answers of a query family are that family's own fold, and the counts of
// the neutral denominator column are the publication package's. This file
// decides order, address and admission and decides nothing else.
//
// It is inert. The lanes are parameters: a lane is one completed solve's facts
// read one coordinate at a time, and the materializer holds it exactly as the
// contributors' own laws state it. Nothing here reaches a solver, a binding or
// a runtime, so a publication is a function of the mounted record and the lanes
// it is handed.

// queryDenominatorDomain is the derivation domain of the key universe a query
// family's result column is total over. The subjects a family is asked at are
// the materializer's own statement, so the membership authority for a result
// column is derived here, domain separated from every contributor's column
// denominator.
const queryDenominatorDomain = "domain/composite/materializer/query-denominator/v1"

// PublishStage names the phase of the one publication that rejected.
type PublishStage uint8

const (
	PublishStageNone PublishStage = iota
	PublishStageTable
	PublishStageInput
	PublishStagePlan
	PublishStageColumn
	PublishStageQuery
	PublishStageSeal
)

func (stage PublishStage) String() string {
	switch stage {
	case PublishStageTable:
		return "table"
	case PublishStageInput:
		return "input"
	case PublishStagePlan:
		return "plan"
	case PublishStageColumn:
		return "column"
	case PublishStageQuery:
		return "query"
	case PublishStageSeal:
		return "seal"
	default:
		return "none"
	}
}

// PublishFailure is the closed verdict of one rejected publication. It names
// the phase, for a column phase the exact declared output, and for a query
// phase the exact family. No builder, no partially filled column and no
// contributor state escapes with it, and no snapshot is published beside it: a
// refused column is a refused publication.
type PublishFailure struct {
	Stage  PublishStage
	Output schema.Key
	Family schema.Key
}

func (failure PublishFailure) Available() bool { return failure.Stage != PublishStageNone }

func (failure PublishFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	if failure.Stage == PublishStageColumn && failure.Output.Available() {
		return failure.Stage.String() + "/" + string(failure.Output)
	}
	if failure.Stage == PublishStageQuery && failure.Family.Available() {
		return failure.Stage.String() + "/" + string(failure.Family)
	}
	return failure.Stage.String()
}

// ExecutionLane is the engine-published reachability lane: the key universe of
// mounted execution points the column is total over, the identity that
// membership is sealed under, and which of those points were reached.
//
// P is the caller's spelling of one mounted execution point. The declaring axis
// states that the pair travels with the publisher and the reader rather than
// with the declaration, so the spelling enters here as a parameter and the
// published value's checked recovery holds a reader to it.
type ExecutionLane[P comparable] struct {
	Universe identity.ContentID
	Points   []P
	Reached  func(point P) bool
}

func (lane ExecutionLane[P]) available() bool {
	return lane.Universe.Available() && len(lane.Points) > 0 && lane.Reached != nil
}

// SummarySubject is one subject the value-summary family is asked at and the
// lane its answer folds. A subject asked with no lane is declared and
// unanswered: it is a member of the result column's key universe and carries no
// row, so it reads back as a proven absence rather than as ignorance.
type SummarySubject struct {
	Subject identity.ContentID
	Lane    valueowner.Lane
}

// ExactSubject is one subject the effect-exact family is asked at: the subject,
// the root the family folds at, and the lane the fold reads. A subject asked
// with no lane is declared and unanswered exactly as a summary subject is.
type ExactSubject struct {
	Subject identity.ContentID
	Root    effectfactor.Root
	Lane    effectowner.Lane
}

// LaneSet is one completed solve as this publication reads it: one lane per
// declared writer, and the subjects each sealed query family is asked at. Every
// member is supplied by the caller, which is what makes the materializer inert:
// it holds lanes and never produces one.
type LaneSet[P comparable] struct {
	Value  valueowner.Lane
	Pack   packowner.Lane
	Heap   heapowner.Lane
	Call   callowner.Lane
	Effect effectowner.Lane
	// Execution is the reachability column's lane. The pass that derives it is
	// the engine's own demand pass, so it enters as a lane like any other.
	Execution ExecutionLane[P]
	// Counts are the owner-local relation cardinalities the neutral denominator
	// column publishes. Completeness over the generated catalog is that
	// column's own publication authority, which is what refuses a partial set.
	Counts []denominatorcounts.CountRows
	// ValueSummary and EffectExact are the subjects the two sealed families are
	// asked at, in the order their result columns declare them.
	ValueSummary []SummarySubject
	EffectExact  []ExactSubject
}

// Materialization is the whole input one publication consumes: the mounted Link
// record every writer's sealed authority is read from, the store and generation
// the publication is addressed by, and the lanes it publishes.
//
// The record is the mount phase's own output. A caller cannot assemble one that
// skipped a mount, because the record admits itself only when every declared
// mount and both post-mount derivations have sealed into it.
type Materialization[P comparable] struct {
	Link       LinkInputs
	Store      identity.StoreID
	Generation identity.Generation
	Lanes      LaneSet[P]
}

func (inputs Materialization[P]) available() bool {
	return inputs.Link.available() && inputs.Store.Available() && inputs.Generation.Available()
}

// materializer is one publication in progress: the schema every address is
// minted under, the mounted authorities, the lanes, and the builder the columns
// are sealed into. It never escapes Materialize, so a refused publication takes
// its half-filled builder with it.
type materializer[P comparable] struct {
	schema  identity.ContentID
	link    LinkInputs
	lanes   LaneSet[P]
	builder snapshot.Builder
}

// columnWriter publishes one declared column. The closure is erased in neither
// key nor value: each writer names its own domain's coordinate and fact types
// inside its body, so the checked recovery a reader performs is checked against
// the very types the publisher sealed.
type columnWriter[P comparable] func(*materializer[P], WriteRequest) bool

// queryWriter answers one sealed query family into its result column, typed in
// the same sense a column writer is.
type queryWriter[P comparable] func(*materializer[P], QueryRequest) bool

// Materialize publishes one solve over the sealed catalog. It walks the
// issuance requests in slot order, drives each writer's own contributor through
// the lane the caller supplied, answers every sealed query family into its
// result column, and seals.
//
// It is fail-closed at every stage: a contributor that refuses, a lane that
// holds a fact its authority does not admit, a coverage authority that cannot
// prove the universe a total column claims, or a family that cannot answer
// leaves no snapshot at all. A publication is the whole catalog or none of it.
func Materialize[P comparable](inputs Materialization[P]) (snapshot.Snapshot, PublishFailure) {
	sealPublication()
	if !publication.ok {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStageTable}
	}
	schemaID, schemaOK := PublicationSchema()
	columns, columnsOK := WriteRequests()
	queries, queriesOK := QueryRequests()
	if !schemaOK || !columnsOK || !queriesOK {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStagePlan}
	}
	// The published slot range is exactly the two request sets. A slot no
	// request names is a column no writer fills, and a request outside the
	// range addresses storage the table never declared; either one is a plan
	// this publication cannot discharge rather than a column it may skip.
	if len(columns)+len(queries) != PublicationColumns() {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStagePlan}
	}
	if !inputs.available() {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStageInput}
	}
	writers, writersOK := columnWriters[P](columns)
	answers, answersOK := queryWriters[P](queries)
	if !writersOK || !answersOK {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStagePlan}
	}
	run := &materializer[P]{
		schema:  schemaID,
		link:    inputs.Link,
		lanes:   inputs.Lanes,
		builder: snapshot.NewBuilder(schemaID, inputs.Store, inputs.Generation),
	}
	for index, request := range columns {
		if !writers[index](run, request) {
			return snapshot.Snapshot{}, PublishFailure{Stage: PublishStageColumn, Output: request.Output}
		}
	}
	for index, request := range queries {
		if !answers[index](run, request) {
			return snapshot.Snapshot{}, PublishFailure{Stage: PublishStageQuery, Family: request.Family}
		}
	}
	sealed, err := run.builder.Seal()
	if err != nil {
		return snapshot.Snapshot{}, PublishFailure{Stage: PublishStageSeal}
	}
	return sealed, PublishFailure{}
}

// columnWriters projects the issuance requests onto the dense writer table this
// publication walks: one writer per request, in request order. The table is
// built once per publication and indexed by position, so the walk performs no
// lookup by key and no request is answered by a writer of another axis.
func columnWriters[P comparable](requests []WriteRequest) ([]columnWriter[P], bool) {
	table := make([]columnWriter[P], len(requests))
	for index, request := range requests {
		writer, declared := columnWriterFor[P](request.Writer)
		if !declared {
			return nil, false
		}
		table[index] = writer
	}
	return table, true
}

// queryWriters projects the materialization requests onto the dense answer
// table, exactly as columnWriters projects the issuance requests.
func queryWriters[P comparable](requests []QueryRequest) ([]queryWriter[P], bool) {
	table := make([]queryWriter[P], len(requests))
	for index, request := range requests {
		writer, declared := queryWriterFor[P](request.Family)
		if !declared {
			return nil, false
		}
		table[index] = writer
	}
	return table, true
}

// columnWriterFor is the publication's typed instantiation table: one writer per
// principal the sealed table admits to a column, named by that principal's own
// declared key.
//
// The table's membership is the publication's coverage law. A writer the table
// admits and this table has no row for is a column nothing can fill, and a row
// here for a principal no axis is declared as would publish a column no
// declaration named. The surface's own law states both.
func columnWriterFor[P comparable](writer schema.Key) (columnWriter[P], bool) {
	switch writer {
	case axisKeyValue:
		return writeValueColumn[P], true
	case axisKeyPack:
		return writePackColumn[P], true
	case axisKeyHeap:
		return writeHeapColumn[P], true
	case axisKeyCall:
		return writeCallColumn[P], true
	case axisKeyEffect:
		return writeEffectColumn[P], true
	case executionowner.AxisKey:
		return writeExecutionColumn[P], true
	case denominatorpublication.AxisKey:
		return writeDenominatorColumn[P], true
	default:
		return nil, false
	}
}

// queryWriterFor is the answering half of the same table: one writer per sealed
// query family, named by the key its owning domain declared it under.
func queryWriterFor[P comparable](family schema.Key) (queryWriter[P], bool) {
	switch family {
	case QueryFamilyValueSummary:
		return answerValueSummary[P], true
	case QueryFamilyEffectExact:
		return answerEffectExact[P], true
	default:
		return nil, false
	}
}

// publishColumn seals one writer's content into the address the declaration
// projects for its output. The address is minted from the declared output alone
// and is held to the request that named it, so a writer cannot publish into a
// slot the table issued to another column.
func publishColumn[P comparable, K comparable, V any](run *materializer[P], request WriteRequest, content snapshot.Content[K, V]) bool {
	column, projected := ProjectAxis[K, V](request.Output)
	if !projected || column.SchemaID != request.Schema || column.Slot != request.Slot {
		return false
	}
	if !admitsCoverage(request.Output, content.Denominator) {
		return false
	}
	return snapshot.PutColumn(&run.builder, column, content) == nil
}

// admitsCoverage holds a column to what its declaration says an absent row
// means. A total column is published together with the key universe it is total
// over, so a writer that can prove no universe publishes nothing rather than a
// column whose silence a consumer would read as a fact.
func admitsCoverage(output schema.Key, denominator identity.ContentID) bool {
	coverage, declared := PublicationCoverage(output)
	if !declared || !coverage.Available() {
		return false
	}
	return coverage != axis.CoverageTotal || denominator.Available()
}

func writeValueColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	authority := run.link.ValueSchema
	denominator, members, sealed := valueowner.Denominator(authority)
	if !sealed {
		return false
	}
	rows := make(map[valuedomain.Coordinate]valuedomain.Value, len(members))
	published := valueowner.Contribute(authority, run.lanes.Value, func(coordinate valuedomain.Coordinate, fact valuedomain.Value) bool {
		if _, held := rows[coordinate]; held {
			return false
		}
		rows[coordinate] = fact
		return true
	})
	if !published {
		return false
	}
	return publishColumn(run, request, snapshot.Content[valuedomain.Coordinate, valuedomain.Value]{
		Rows: rows, Denominator: denominator, Members: members,
	})
}

func writePackColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	authority := run.link.PackSchema
	denominator, members, sealed := packowner.Denominator(authority)
	if !sealed {
		return false
	}
	rows := make(map[packdomain.Root]packdomain.Value, len(members))
	published := packowner.Contribute(authority, run.lanes.Pack, func(root packdomain.Root, fact packdomain.Value) bool {
		if _, held := rows[root]; held {
			return false
		}
		rows[root] = fact
		return true
	})
	if !published {
		return false
	}
	return publishColumn(run, request, snapshot.Content[packdomain.Root, packdomain.Value]{
		Rows: rows, Denominator: denominator, Members: members,
	})
}

func writeHeapColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	authority := run.link.HeapSchema
	denominator, members, sealed := heapowner.Denominator(authority)
	if !sealed {
		return false
	}
	rows := make(map[heapdomain.Key]heapdomain.Value, len(members))
	published := heapowner.Contribute(authority, run.lanes.Heap, func(key heapdomain.Key, fact heapdomain.Value) bool {
		if _, held := rows[key]; held {
			return false
		}
		rows[key] = fact
		return true
	})
	if !published {
		return false
	}
	return publishColumn(run, request, snapshot.Content[heapdomain.Key, heapdomain.Value]{
		Rows: rows, Denominator: denominator, Members: members,
	})
}

func writeCallColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	authority := run.link.CallAlgebra
	denominator, members, sealed := callowner.Denominator(authority)
	if !sealed {
		return false
	}
	rows := make(map[calldomain.Key]calldomain.Value, len(members))
	published := callowner.Contribute(authority, run.lanes.Call, func(key calldomain.Key, fact calldomain.Value) bool {
		if _, held := rows[key]; held {
			return false
		}
		rows[key] = fact
		return true
	})
	if !published {
		return false
	}
	return publishColumn(run, request, snapshot.Content[calldomain.Key, calldomain.Value]{
		Rows: rows, Denominator: denominator, Members: members,
	})
}

func writeEffectColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	authority := run.link.EffectAlgebra
	denominator, members, sealed := effectowner.Denominator(authority)
	if !sealed {
		return false
	}
	rows := make(map[effectfactor.Root]effectfactor.Value, len(members))
	published := effectowner.Contribute(authority, run.lanes.Effect, func(root effectfactor.Root, fact effectfactor.Value) bool {
		if _, held := rows[root]; held {
			return false
		}
		rows[root] = fact
		return true
	})
	if !published {
		return false
	}
	return publishColumn(run, request, snapshot.Content[effectfactor.Root, effectfactor.Value]{
		Rows: rows, Denominator: denominator, Members: members,
	})
}

// writeExecutionColumn publishes the engine-published reachability column. The
// column carries presence alone: a reached point costs one unit row, and a
// covered point with no row is unreachable as a published fact.
func writeExecutionColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	lane := run.lanes.Execution
	if !lane.available() {
		return false
	}
	rows := make(map[P]executionowner.Reachable, len(lane.Points))
	members := make([]P, 0, len(lane.Points))
	covered := make(map[P]struct{}, len(lane.Points))
	for _, point := range lane.Points {
		if _, duplicate := covered[point]; duplicate {
			return false
		}
		covered[point] = struct{}{}
		members = append(members, point)
		if lane.Reached(point) {
			rows[point] = executionowner.Reachable{}
		}
	}
	return publishColumn(run, request, snapshot.Content[P, executionowner.Reachable]{
		Rows: rows, Denominator: lane.Universe, Members: members,
	})
}

// writeDenominatorColumn publishes the neutral relation-count column. The
// content is built by the package that owns the count universe, so completeness
// over the generated catalog is stated there and a partial set of owner counts
// refuses here rather than publishing an implicit zero.
func writeDenominatorColumn[P comparable](run *materializer[P], request WriteRequest) bool {
	content, built := denominatorpublication.BuildContent(run.schema, run.lanes.Counts...)
	if !built {
		return false
	}
	return publishColumn(run, request, content)
}

// answerValueSummary folds one lane per subject through the value domain's own
// summary contributor and publishes the answers as the family's result column.
func answerValueSummary[P comparable](run *materializer[P], request QueryRequest) bool {
	asked := run.lanes.ValueSummary
	if len(asked) == 0 {
		return false
	}
	rows := make(map[identity.ContentID]valuedomain.ValueSummaryObservation, len(asked))
	members := make([]identity.ContentID, 0, len(asked))
	covered := make(map[identity.ContentID]struct{}, len(asked))
	for _, subject := range asked {
		if !subject.Subject.Available() {
			return false
		}
		if _, duplicate := covered[subject.Subject]; duplicate {
			return false
		}
		covered[subject.Subject] = struct{}{}
		members = append(members, subject.Subject)
		if subject.Lane == nil {
			continue
		}
		answered := valueowner.ContributeSummary(run.link.ValueSchema, subject.Subject, subject.Lane,
			func(key identity.ContentID, answer valuedomain.ValueSummaryObservation) bool {
				rows[key] = answer
				return true
			})
		if !answered {
			return false
		}
	}
	return declareAnswers(run, request, rows, members)
}

// answerEffectExact folds one lane per subject through the effect domain's own
// exact contributor and publishes the answers as the family's result column.
func answerEffectExact[P comparable](run *materializer[P], request QueryRequest) bool {
	asked := run.lanes.EffectExact
	if len(asked) == 0 {
		return false
	}
	rows := make(map[identity.ContentID]effectfactor.EffectObservation, len(asked))
	members := make([]identity.ContentID, 0, len(asked))
	covered := make(map[identity.ContentID]struct{}, len(asked))
	for _, subject := range asked {
		if !subject.Subject.Available() {
			return false
		}
		if _, duplicate := covered[subject.Subject]; duplicate {
			return false
		}
		covered[subject.Subject] = struct{}{}
		members = append(members, subject.Subject)
		if subject.Lane == nil {
			continue
		}
		answered := effectowner.ContributeExact(run.link.EffectAlgebra, subject.Subject, subject.Root, subject.Lane,
			func(key identity.ContentID, answer effectfactor.EffectObservation) bool {
				rows[key] = answer
				return true
			})
		if !answered {
			return false
		}
	}
	return declareAnswers(run, request, rows, members)
}

// declareAnswers seals one family's answers as its result column and registers
// the family answerable under the identity the table sealed it as. The subjects
// the family was asked at are the column's key universe, so a subject that was
// asked and not answered reads back as a proven absence and a subject the
// family was never asked at reads back as a miss.
func declareAnswers[P comparable, O any](run *materializer[P], request QueryRequest, rows map[identity.ContentID]O, members []identity.ContentID) bool {
	if request.Schema != run.schema || !request.ID.Available() {
		return false
	}
	universe, derived := subjectUniverse(request.ID, members)
	if !derived {
		return false
	}
	plan, err := snapshot.DeclareQuery(&run.builder, request.ID, request.Slot, snapshot.Content[identity.ContentID, O]{
		Rows: rows, Denominator: universe, Members: members,
	})
	return err == nil && plan.Available()
}

// subjectUniverse derives the identity the subjects of one family are sealed
// under. The family's own published identity enters the derivation, so two
// families asked at one subject set are total over two universes and neither
// reads the other's membership authority.
func subjectUniverse(family identity.ContentID, members []identity.ContentID) (identity.ContentID, bool) {
	if !family.Available() || len(members) == 0 {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, len(members)+1)
	parts = append(parts, family[:])
	for index := range members {
		parts = append(parts, members[index][:])
	}
	return identity.DeriveContentID(queryDenominatorDomain, parts...)
}
