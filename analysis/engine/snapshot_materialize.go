package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	solvedQueryOutput       schema.Key = "engine/solved-query"
	solvedObservationOutput schema.Key = "engine/solved-observation"
	solvedPointOutput       schema.Key = "engine/solved-point-state"
	solvedColumnWriter      schema.Key = "engine/solved-publisher"
)

// One completed solve published as immutable snapshot columns. A publication
// is never a computation. beginSolvedPublication writes the declared axes,
// and a consumer reads a published answer through snapshot.OpenQuery and
// snapshot.Query alone.
//
// Every refusal here speaks the compile family of the public failure
// vocabulary: sealing a publication is a cold seal, and the site digest carries
// which boundary of the seal rejected.

const (
	solvedSchemaDomain    = "engine/solved-snapshot-schema"
	solvedAxisDomain      = "engine/solved-snapshot-axis"
	solvedRowKeyDomain    = "engine/solved-snapshot-row-key"
	solvedContentDomain   = "engine/solved-snapshot-content"
	solvedIdentityVersion = 1
)

const (
	solvedSitePublication = "solved-snapshot/publication"
	solvedSiteAxis        = "solved-snapshot/axis"
	solvedSiteRow         = "solved-snapshot/row"
	solvedSiteDeclare     = "solved-snapshot/declare"
	solvedSiteSeal        = "solved-snapshot/seal"
	solvedSiteContent     = "solved-snapshot/content"
	solvedSiteDelta       = "solved-snapshot/delta"
)

// solvedLaneWidth is the width of the result lane vocabulary, which is what
// indexes a materialization's per-lane family. solvedAxisCount is how many of
// those lanes name an axis, which is how many column slots a publication fills.
const (
	solvedLaneWidth    = int(resultLaneObservation) + 1
	solvedAxisCount    = solvedLaneWidth - 1
	solvedPointSlot    = uint32(solvedAxisCount)
	solvedStoreColumns = solvedAxisCount + 1
)

// solvedAxisRole separates the two identities one result axis publishes: the
// query family a consumer opens, and the denominator its declared key universe
// is sealed under. The two are framed apart, so neither a directory entry nor a
// membership authority is reachable under the other's identity.
type solvedAxisRole uint64

const (
	solvedAxisFamily solvedAxisRole = iota + 1
	solvedAxisDenominator
)

// solvedValue is the published result value one solve row carries. This alias
// is the single declaration that names the engine's value model: every type and
// function below names solvedValue, so a later value model reaches
// beginSolvedPublication through this line.
type solvedValue = frozenValue

// Answer is one published result row as a snapshot result column stores it. It
// carries the borrowed, transitively immutable value the solve published, and a
// consumer names Answer to open a result column and to read the typed value out
// of the row it answers.
type Answer struct{ value solvedValue }

// Available reports whether this Answer carries a published value.
func (answer Answer) Available() bool { return answer.value != nil }

// Fingerprint returns the freezer's fingerprint of the published value. It is
// the value's own content stamp, which is what a content address over published
// rows folds.
func (answer Answer) Fingerprint() uint64 {
	if answer.value == nil {
		return 0
	}
	return answer.value.fingerprint()
}

// Equal reports whether two answers carry values their freezer calls equal.
func (answer Answer) Equal(other Answer) bool {
	return answer.value != nil && other.value != nil && answer.value.equal(other.value)
}

// AnswerValue borrows the typed value out of a published answer. The value is
// the published one: reading it clones nothing, and a caller that needs an
// owned copy asks for one through DetachAnswer.
func AnswerValue[R any](answer Answer) (R, bool) {
	value, _, ok := typedResult[R](answer.value)
	return value, ok
}

// DetachAnswer returns the freezer's independent copy of the borrowed value.
// Detachment is charged only to the caller that asks for an owned value.
func DetachAnswer[R any](answer Answer) (R, bool) {
	value, freeze, ok := typedResult[R](answer.value)
	if !ok || freeze.Clone == nil {
		var zero R
		return zero, false
	}
	return freeze.Clone(value), true
}

// solvedResults is the overlay dump commit folds into a content identity:
// the publication identities a snapshot is sealed under and one ordered axis
// per declared result lane.
type solvedResults struct {
	schema     identity.ContentID
	store      identity.StoreID
	generation identity.Generation
	// axes are the declared result axes in publication order. The axis at
	// index i is sealed into column slot i, so the dense slot vector a snapshot
	// publishes is the solve's own axis order.
	axes []solvedAxis
}

// solvedPublicationPlan is the runtime-owned publication authority.  It is
// sealed with the runtime that owns the graph: the column binding and its
// write capabilities, the two result key universes, and the point-state
// denominator/key index all belong to this one immutable plan.  A solve opens
// a private builder against the plan; it never re-admits columns or
// reconstructs a denominator authority.
//
// The point rows themselves are solve-local values and therefore cannot be
// sealed here.  The point key universe, its dense key index, and the
// denominator identity are structural and are sealed here once.
type solvedPublicationPlan struct {
	sealed bool
	schema identity.ContentID

	binding         *SchemaBinding
	queryWrite      ColumnWrite[identity.ContentID, Answer]
	obsWrite        ColumnWrite[identity.ContentID, Answer]
	pointWrite      ColumnWrite[composition.Key, carrier.PointState]
	queryKeys       []identity.ContentID
	observationKeys []identity.ContentID

	families         [solvedLaneWidth]identity.ContentID
	denominators     [solvedLaneWidth]identity.ContentID
	pointAxis        snapshot.Axis[composition.Key, carrier.PointState]
	pointDenominator identity.ContentID
	pointMembers     []composition.Key
	pointKeys        []composition.Key
	pointIndex       map[composition.Key]int
}

func (plan *solvedPublicationPlan) available() bool {
	return plan != nil && plan.sealed && plan.schema.Available() && plan.binding != nil && plan.binding.Sealed() &&
		plan.queryWrite.Available() && plan.obsWrite.Available() && plan.pointWrite.Available() &&
		plan.pointAxis.Available() && plan.pointDenominator.Available() && len(plan.pointMembers) != 0 &&
		len(plan.pointIndex) == len(plan.pointMembers) && len(plan.pointKeys) > 0
}

// solvedAxis is one declared result lane: every row the lane declares, in the
// solve's own row order. The declared keys are the axis denominator, so a
// declared row that carries no value publishes an absence a consumer proves
// rather than an answer a consumer fails to find.
type solvedAxis struct {
	lane resultLane
	rows []solvedRow
}

// solvedRow is one declared result row: the canonical identity a consumer
// addresses it by, and the value the solve published for it. A row whose value
// is unavailable is declared and unanswered.
type solvedRow struct {
	key   identity.ContentID
	value solvedValue
}

// SolvedSnapshot is one materialized publication: the immutable Snapshot a
// consumer holds, and the content identity of the rows it publishes. It hands
// out the family identities a consumer needs to open a result column and hands
// out no column, no row, and no engine runtime.
type SolvedSnapshot struct {
	published       snapshot.Snapshot
	schema          identity.ContentID
	content         identity.ContentID
	families        [solvedLaneWidth]identity.ContentID
	queryPlan       snapshot.QueryPlan[identity.ContentID, Answer]
	observationPlan snapshot.QueryPlan[identity.ContentID, Answer]
	pointAxis       snapshot.Axis[composition.Key, carrier.PointState]
}

// Available reports whether this value is a sealed materialization.
func (materialized SolvedSnapshot) Available() bool {
	return materialized.published.Published() && materialized.content.Available()
}

// Snapshot returns the published value. Copying a Snapshot shares its published
// structure and copies no rows, so handing one out costs a value copy.
func (materialized SolvedSnapshot) Snapshot() snapshot.Snapshot { return materialized.published }

// Content returns the content identity of the published rows. Two solves that
// publish the same answers under the same schema mint the same identity,
// whatever store or generation published them.
func (materialized SolvedSnapshot) Content() identity.ContentID { return materialized.content }

// QueryFamily returns the query family that answers the solve's query result
// axis.
func (materialized SolvedSnapshot) QueryFamily() identity.ContentID {
	return materialized.family(resultLaneQuery)
}

// ObservationFamily returns the query family that answers the solve's
// observation result axis.
func (materialized SolvedSnapshot) ObservationFamily() identity.ContentID {
	return materialized.family(resultLaneObservation)
}

// family returns the published family identity of one result lane. It reads a
// sealed field and allocates nothing, so opening a query costs no publication
// work on the read path.
func (materialized SolvedSnapshot) family(lane resultLane) identity.ContentID {
	if lane == resultLaneNone || int(lane) >= solvedLaneWidth {
		return identity.ContentID{}
	}
	return materialized.families[lane]
}

// solvedPublication is one epoch's query/observation write onto a private
// Snapshot transaction: NewBuilder on the first generation, NewDelta after.
type solvedPublication struct {
	plan       *solvedPublicationPlan
	builder    snapshot.Builder
	solved     SolvedSnapshot
	store      identity.StoreID
	generation identity.Generation
	pointAxis  snapshot.Axis[composition.Key, carrier.PointState]
	queryWrite ColumnWrite[identity.ContentID, Answer]
	obsWrite   ColumnWrite[identity.ContentID, Answer]
	pointWrite ColumnWrite[composition.Key, carrier.PointState]
}

// sealSolvedPublicationPlan builds the one publication authority attached to
// a sealed runtime.  Every value in this plan is derived from immutable
// runtime/schema rows; no epoch or solve-local state is consulted, so this is
// the only place publication admissions, result universes, and point
// denominator membership are constructed.
func sealSolvedPublicationPlan(runtime *solverRuntime) (*solvedPublicationPlan, bool) {
	if runtime == nil || runtime.graph == nil || len(runtime.activePoints) != runtime.graph.PointCount() {
		return nil, false
	}
	schema := solvedSchemaIdentity(runtime)
	if !schema.Available() {
		return nil, false
	}
	queryKeys, queryOK := declaredQueryKeys(runtime)
	observationKeys, observationOK := declaredObservationKeys(runtime)
	pointMembers, pointKeys, pointIndex, pointDenominator, pointOK := sealPointUniverse(runtime, schema)
	if !queryOK || !observationOK || !pointOK {
		return nil, false
	}
	binding := NewColumnBinding()
	if binding == nil || !AdmitColumns(binding, []ColumnAdmission{
		{Schema: schema, Output: solvedQueryOutput, Writer: solvedColumnWriter, Slot: 0},
		{Schema: schema, Output: solvedObservationOutput, Writer: solvedColumnWriter, Slot: 1},
		{Schema: schema, Output: solvedPointOutput, Writer: solvedColumnWriter, Slot: solvedPointSlot},
	}) || !binding.Seal() {
		return nil, false
	}
	queryWrite, queryMinted := MintColumnWrite[identity.ContentID, Answer](binding, solvedQueryOutput, solvedColumnWriter)
	observationWrite, observationMinted := MintColumnWrite[identity.ContentID, Answer](binding, solvedObservationOutput, solvedColumnWriter)
	pointWrite, pointMinted := MintColumnWrite[composition.Key, carrier.PointState](binding, solvedPointOutput, solvedColumnWriter)
	if !queryMinted || !observationMinted || !pointMinted {
		return nil, false
	}
	plan := &solvedPublicationPlan{
		sealed:           true,
		schema:           schema,
		binding:          binding,
		queryWrite:       queryWrite,
		obsWrite:         observationWrite,
		pointWrite:       pointWrite,
		queryKeys:        queryKeys,
		observationKeys:  observationKeys,
		pointAxis:        pointStateAxis(schema),
		pointDenominator: pointDenominator,
		pointMembers:     pointMembers,
		pointKeys:        pointKeys,
		pointIndex:       pointIndex,
	}
	plan.families[resultLaneQuery] = solvedAxisIdentity(schema, resultLaneQuery, solvedAxisFamily)
	plan.families[resultLaneObservation] = solvedAxisIdentity(schema, resultLaneObservation, solvedAxisFamily)
	plan.denominators[resultLaneQuery] = solvedAxisIdentity(schema, resultLaneQuery, solvedAxisDenominator)
	plan.denominators[resultLaneObservation] = solvedAxisIdentity(schema, resultLaneObservation, solvedAxisDenominator)
	if !plan.families[resultLaneQuery].Available() || !plan.families[resultLaneObservation].Available() || !plan.denominators[resultLaneQuery].Available() || !plan.denominators[resultLaneObservation].Available() || !plan.pointAxis.Available() || !plan.available() {
		return nil, false
	}
	return plan, true
}

func mintSolvedColumnWrites(schema identity.ContentID, includePoint bool) (query ColumnWrite[identity.ContentID, Answer], observation ColumnWrite[identity.ContentID, Answer], point ColumnWrite[composition.Key, carrier.PointState], ok bool) {
	admissions := []ColumnAdmission{
		{Schema: schema, Output: solvedQueryOutput, Writer: solvedColumnWriter, Slot: 0},
		{Schema: schema, Output: solvedObservationOutput, Writer: solvedColumnWriter, Slot: 1},
	}
	if includePoint {
		admissions = append(admissions, ColumnAdmission{Schema: schema, Output: solvedPointOutput, Writer: solvedColumnWriter, Slot: solvedPointSlot})
	}
	binding := NewColumnBinding()
	if !AdmitColumns(binding, admissions) || !binding.Seal() {
		return ColumnWrite[identity.ContentID, Answer]{}, ColumnWrite[identity.ContentID, Answer]{}, ColumnWrite[composition.Key, carrier.PointState]{}, false
	}
	query, queryOK := MintColumnWrite[identity.ContentID, Answer](binding, solvedQueryOutput, solvedColumnWriter)
	observation, observationOK := MintColumnWrite[identity.ContentID, Answer](binding, solvedObservationOutput, solvedColumnWriter)
	if !queryOK || !observationOK {
		return ColumnWrite[identity.ContentID, Answer]{}, ColumnWrite[identity.ContentID, Answer]{}, ColumnWrite[composition.Key, carrier.PointState]{}, false
	}
	if !includePoint {
		return query, observation, ColumnWrite[composition.Key, carrier.PointState]{}, true
	}
	point, pointOK := MintColumnWrite[composition.Key, carrier.PointState](binding, solvedPointOutput, solvedColumnWriter)
	return query, observation, point, pointOK
}

func beginSolvedPublication(solver *Solver, epoch *executorEpoch, generation identity.Generation) (*solvedPublication, bool) {
	if solver == nil || solver.runtime == nil || epoch == nil || epoch.runtime != solver.runtime || !solver.store.Available() || !generation.Available() {
		return nil, false
	}
	plan := solver.runtime.publication
	if plan == nil || !plan.available() {
		return nil, false
	}
	publication := &solvedPublication{
		plan:       plan,
		solved:     SolvedSnapshot{schema: plan.schema, families: plan.families, pointAxis: plan.pointAxis},
		store:      solver.store,
		generation: generation,
		queryWrite: plan.queryWrite,
		obsWrite:   plan.obsWrite,
		pointWrite: plan.pointWrite,
		pointAxis:  plan.pointAxis,
	}
	if canDeltaSolvedPublication(solver.lastSolved, plan.schema, solver.store, generation, plan.queryKeys, plan.observationKeys, plan.pointMembers) {
		publication.builder = snapshot.NewDelta(solver.lastSolved.published, generation)
		publication.solved.families = solver.lastSolved.families
		publication.solved.queryPlan = solver.lastSolved.queryPlan
		publication.solved.observationPlan = solver.lastSolved.observationPlan
		publication.pointAxis = solver.lastSolved.pointAxis
		publication.solved.pointAxis = solver.lastSolved.pointAxis
		if !publication.solved.queryPlan.Available() || !publication.solved.observationPlan.Available() || !publication.pointAxis.Available() {
			return nil, false
		}
		return publication, true
	}
	pointRows, pointOK := collectActivePointRows(plan, epoch)
	if !pointOK {
		return nil, false
	}
	publication.builder = snapshot.NewBuilder(plan.schema, solver.store, generation)
	queryPlan, queryDeclared := declareSolvedFamily(plan, &publication.builder, resultLaneQuery)
	obsPlan, obsDeclared := declareSolvedFamily(plan, &publication.builder, resultLaneObservation)
	pointAxis, pointDeclared := declarePointColumnFromPlan(plan, &publication.builder, pointRows)
	if !queryDeclared || !obsDeclared || !pointDeclared {
		return nil, false
	}
	publication.solved.families = plan.families
	publication.solved.queryPlan = queryPlan
	publication.solved.observationPlan = obsPlan
	publication.pointAxis = pointAxis
	publication.solved.pointAxis = pointAxis
	return publication, true
}

func declaredQueryKeys(runtime *solverRuntime) ([]identity.ContentID, bool) {
	if runtime == nil {
		return nil, false
	}
	keys := make([]identity.ContentID, 0, len(runtime.queries))
	seen := make(map[identity.ContentID]struct{}, len(runtime.queries))
	for _, declared := range runtime.queries {
		if declared == nil {
			return nil, false
		}
		key, keyed := declared.PublicationKey()
		if !keyed {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, true
}

func declaredObservationKeys(runtime *solverRuntime) ([]identity.ContentID, bool) {
	if runtime == nil {
		return nil, false
	}
	keys := make([]identity.ContentID, 0, len(runtime.observations))
	seen := make(map[identity.ContentID]struct{}, len(runtime.observations))
	for _, declared := range runtime.observations {
		if declared == nil {
			return nil, false
		}
		key := declared.observationID()
		if !key.Available() {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, true
}

func canDeltaSolvedPublication(base SolvedSnapshot, schema identity.ContentID, store identity.StoreID, generation identity.Generation, queryKeys, obsKeys []identity.ContentID, pointMembers []composition.Key) bool {
	return base.Available() && base.schema == schema && base.published.Store() == store && base.published.Generation().Precedes(generation) && base.queryPlan.Available() && base.observationPlan.Available() && base.pointAxis.Available() && publicationCovers(base.published, base.queryPlan.Axis(), queryKeys) && publicationCovers(base.published, base.observationPlan.Axis(), obsKeys) && publicationCovers(base.published, base.pointAxis, pointMembers)
}

func publicationCovers[K comparable, V any](published snapshot.Snapshot, axis snapshot.Axis[K, V], keys []K) bool {
	if !published.Published() || !axis.Available() {
		return false
	}
	for _, key := range keys {
		_, status := snapshot.Read(&published, axis, key)
		if status != snapshot.ReadHit && status != snapshot.ReadProvenAbsent {
			return false
		}
	}
	return true
}

func declareSolvedFamily(plan *solvedPublicationPlan, builder *snapshot.Builder, lane resultLane) (snapshot.QueryPlan[identity.ContentID, Answer], bool) {
	if plan == nil || builder == nil || lane == resultLaneNone || int(lane) >= solvedLaneWidth || !plan.families[lane].Available() || !plan.denominators[lane].Available() || !plan.available() {
		return snapshot.QueryPlan[identity.ContentID, Answer]{}, false
	}
	write, members := plan.queryWrite, plan.queryKeys
	if lane == resultLaneObservation {
		write, members = plan.obsWrite, plan.observationKeys
	}
	queryPlan, err := PublishQueryColumn(write, builder, plan.families[lane], snapshot.Content[identity.ContentID, Answer]{
		Denominator: plan.denominators[lane],
		Members:     members,
	})
	return queryPlan, err == nil && queryPlan.Available()
}

func (publication *solvedPublication) writeQuery(key identity.ContentID, value solvedValue) bool {
	return publication.write(publication.queryWrite, key, value)
}

func (publication *solvedPublication) writeObservation(key identity.ContentID, value solvedValue) bool {
	return publication.write(publication.obsWrite, key, value)
}

func (publication *solvedPublication) write(write ColumnWrite[identity.ContentID, Answer], key identity.ContentID, value solvedValue) bool {
	if publication == nil || !write.Available() || !key.Available() {
		return false
	}
	var err error
	if value != nil {
		err = PublishRow(write, &publication.builder, key, Answer{value: value})
	} else {
		err = WithdrawRow(write, &publication.builder, key)
	}
	return err == nil
}

func (publication *solvedPublication) overlayRows(plan snapshot.QueryPlan[identity.ContentID, Answer], keys []identity.ContentID) ([]solvedRow, bool) {
	if publication == nil || !plan.Available() {
		return nil, false
	}
	out := make([]solvedRow, len(keys))
	for index, key := range keys {
		if !key.Available() {
			return nil, false
		}
		out[index].key = key
		answer, status := snapshot.ReadOverlay(&publication.builder, plan.Axis(), key)
		switch status {
		case snapshot.ReadHit:
			if !answer.Available() {
				return nil, false
			}
			out[index].value = answer.value
		case snapshot.ReadProvenAbsent:
		default:
			return nil, false
		}
	}
	return out, true
}

func (publication *solvedPublication) commit(solver *Solver) (SolvedSnapshot, bool) {
	if publication == nil || publication.plan == nil || solver == nil {
		return SolvedSnapshot{}, false
	}
	if solver.runtime == nil || solver.runtime.publication != publication.plan || !publication.plan.available() {
		return SolvedSnapshot{}, false
	}
	queryRows, queryOK := publication.overlayRows(publication.solved.queryPlan, publication.plan.queryKeys)
	obsRows, obsOK := publication.overlayRows(publication.solved.observationPlan, publication.plan.observationKeys)
	if !queryOK || !obsOK {
		return SolvedSnapshot{}, false
	}
	published, err := publication.builder.Seal()
	if err != nil || !published.Published() {
		return SolvedSnapshot{}, false
	}
	content, minted := solvedContentIdentity(solvedResults{
		schema:     publication.solved.schema,
		store:      publication.store,
		generation: publication.generation,
		axes: []solvedAxis{
			{lane: resultLaneQuery, rows: queryRows},
			{lane: resultLaneObservation, rows: obsRows},
		},
	})
	if !minted {
		return SolvedSnapshot{}, false
	}
	publication.solved.published = published
	publication.solved.content = content
	publication.solved.pointAxis = publication.pointAxis
	solver.lastSolved = publication.solved
	return publication.solved, publication.solved.Available()
}

// solvedSchemaIdentity frames the sealing schema identity a published snapshot
// is addressed under. It is derived from the sealed graph's composition
// identity and framed apart from it, so a snapshot schema identity and a
// composition identity are two identities of one schema rather than one, and an
// axis of one composition never validates against another's publication.
func solvedSchemaIdentity(runtime *solverRuntime) identity.ContentID {
	if runtime == nil || runtime.graph == nil {
		return identity.ContentID{}
	}
	composed := runtime.graph.CompositionID()
	if !composed.Available() {
		return identity.ContentID{}
	}
	return framedContentID(solvedSchemaDomain, solvedIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(composed[:]) == nil
	})
}

// solvedAxisIdentity frames one identity of one result axis. The schema and the
// lane both enter the preimage, so two schemas never share a family and the two
// lanes of one schema never share a column.
func solvedAxisIdentity(schema identity.ContentID, lane resultLane, role solvedAxisRole) identity.ContentID {
	if !schema.Available() || lane == resultLaneNone {
		return identity.ContentID{}
	}
	return framedContentID(solvedAxisDomain, solvedIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(schema[:]) == nil && writer.Uint(uint64(lane)) == nil && writer.Uint(uint64(role)) == nil
	})
}

// solvedRowKey frames the identity a consumer addresses one query row by. The
// query lane carries a versioned internal declaration key, and the framing is
// what turns it into the content identity that crosses the published boundary.
func solvedRowKey(semantic composition.Key) identity.ContentID {
	if !semantic.Available() {
		return identity.ContentID{}
	}
	return framedContentID(solvedRowKeyDomain, solvedIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(semantic.ID[:]) == nil && writer.Uint(semantic.Version) == nil
	})
}

// solvedContentIdentity frames the content address of one materialization: the
// schema, then every axis in publication order, then every row of that axis in
// ascending key order with the freezer fingerprint of the value it answers.
// Ordering the rows by key is what makes the identity a function of the
// published content rather than of the solve's row order.
func solvedContentIdentity(results solvedResults) (identity.ContentID, bool) {
	widest := 0
	for _, axis := range results.axes {
		if len(axis.rows) > widest {
			widest = len(axis.rows)
		}
	}
	ordered := make([]solvedRow, 0, widest)
	minted := framedContentID(solvedContentDomain, solvedIdentityVersion, func(writer *canonical.DigestWriter) bool {
		if writer.Bytes(results.schema[:]) != nil || writer.Count(uint64(len(results.axes))) != nil {
			return false
		}
		for _, axis := range results.axes {
			ordered = append(ordered[:0], axis.rows...)
			identity.SortByContentID(ordered, func(row solvedRow) identity.ContentID { return row.key })
			if writer.Uint(uint64(axis.lane)) != nil || writer.Count(uint64(len(ordered))) != nil {
				return false
			}
			for _, row := range ordered {
				answered, fingerprint := uint64(0), uint64(0)
				if row.value != nil {
					answered, fingerprint = 1, row.value.fingerprint()
				}
				if writer.Bytes(row.key[:]) != nil || writer.Uint(answered) != nil || writer.Uint(fingerprint) != nil {
					return false
				}
			}
		}
		return true
	})
	return minted, minted.Available()
}
