package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/canonical"
)

// The inert snapshot materializer: one completed solve published as immutable
// snapshot columns. A materialization is a publication and never a computation.
// It reads the solve's finished rows, seals one result column per declared
// result axis, and registers each column answerable under its own family
// identity, so a consumer reads a published answer through snapshot.OpenQuery
// and snapshot.Query alone.
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
	solvedSitePreflight      = "solved-snapshot/preflight"
	solvedSiteCompletion     = "solved-snapshot/completion"
	solvedSiteSchema         = "solved-snapshot/schema"
	solvedSiteColumnWidth    = "solved-snapshot/column-width"
	solvedSiteQueryRow       = "solved-snapshot/query-row"
	solvedSiteObservationRow = "solved-snapshot/observation-row"
	solvedSitePublication    = "solved-snapshot/publication"
	solvedSiteAxis           = "solved-snapshot/axis"
	solvedSiteRow            = "solved-snapshot/row"
	solvedSiteDeclare        = "solved-snapshot/declare"
	solvedSiteSeal           = "solved-snapshot/seal"
	solvedSiteContent        = "solved-snapshot/content"
	solvedSiteDelta          = "solved-snapshot/delta"
)

// solvedLaneWidth is the width of the result lane vocabulary, which is what
// indexes a materialization's per-lane family. solvedAxisCount is how many of
// those lanes name an axis, which is how many column slots a publication fills.
const (
	solvedLaneWidth = int(resultLaneObservation) + 1
	solvedAxisCount = solvedLaneWidth - 1
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
// function below names solvedValue, so a later value model reaches the
// materializer through this line and through solvedResultsOf.
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

// solvedResults is the whole input the materializer consumes: one completed
// solve, described as the publication identities its snapshot is sealed under
// and one ordered axis per declared result lane. Nothing below this value reads
// a Solver, a State, a runtime, or a carrier, so a change to the engine's row
// model reaches the materializer through solvedResultsOf and nowhere else.
type solvedResults struct {
	schema     identity.ContentID
	store      identity.StoreID
	generation identity.Generation
	// axes are the declared result axes in publication order. The axis at
	// index i is sealed into column slot i, so the dense slot vector a snapshot
	// publishes is the solve's own axis order.
	axes []solvedAxis
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
	published snapshot.Snapshot
	schema    identity.ContentID
	content   identity.ContentID
	families  [solvedLaneWidth]identity.ContentID
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

// materializeCompletedState publishes one completed solve as snapshot columns.
// It is the whole seam: the projection of the engine's result shapes onto the
// materializer's input, then the publication of that input.
func materializeCompletedState(solver *Solver, state *State) (SolvedSnapshot, SolveFailure) {
	results, failure := solvedResultsOf(solver, state)
	if failure.Available() {
		return SolvedSnapshot{}, failure
	}
	return materializeSolvedResults(results)
}

// solvedResultsOf projects one completed State onto the materializer's input.
// The query axis pairs the solver's declared query families with the State's
// published query column, and the observation axis pairs the declared
// observations with the observation column; a row the column does not answer
// stays declared and unanswered.
func solvedResultsOf(solver *Solver, state *State) (solvedResults, SolveFailure) {
	if solver == nil || state == nil || state.completion == nil || solver.runtime == nil {
		return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSitePreflight).failure()
	}
	if !solver.ownsCompletedState(state) {
		return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteCompletion).failure()
	}
	runtime := solver.runtime
	schema := solvedSchemaIdentity(runtime)
	if !schema.Available() {
		return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteSchema).failure()
	}
	if len(state.results) != len(runtime.queries) || len(state.observations) != len(runtime.observations) {
		return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteColumnWidth).failure()
	}

	queryRows := make([]solvedRow, 0, len(runtime.queries))
	for index, declared := range runtime.queries {
		if declared == nil {
			return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteQueryRow).failure()
		}
		semantic := declared.query().Key()
		row := solvedRow{key: solvedRowKey(semantic)}
		if !row.key.Available() {
			return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteQueryRow).failure()
		}
		if published := state.results[index]; published != nil {
			if published.key != semantic || published.value == nil {
				return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteQueryRow).failure()
			}
			row.value = published.value
		}
		queryRows = append(queryRows, row)
	}

	observationRows := make([]solvedRow, 0, len(runtime.observations))
	for index, declared := range runtime.observations {
		if declared == nil {
			return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteObservationRow).failure()
		}
		row := solvedRow{key: declared.observationID()}
		if !row.key.Available() {
			return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteObservationRow).failure()
		}
		if published := state.observations[index]; published != nil {
			if published.id != row.key || published.value == nil {
				return solvedResults{}, refused(SolveFailureFamilyCompile, solvedSiteObservationRow).failure()
			}
			row.value = published.value
		}
		observationRows = append(observationRows, row)
	}

	return solvedResults{
		schema:     schema,
		store:      state.completion.store,
		generation: state.completion.serial,
		axes: []solvedAxis{
			{lane: resultLaneQuery, rows: queryRows},
			{lane: resultLaneObservation, rows: observationRows},
		},
	}, SolveFailure{}
}

// materializeSolvedResults publishes results as snapshot columns: one result
// column per declared axis, keyed by the canonical row identities, declared
// answerable under the axis family. The declared keys of an axis are its
// denominator, so a withdrawn or unanswered row reads as a proven absence and a
// key the axis never declared reads as a miss.
func materializeSolvedResults(results solvedResults) (SolvedSnapshot, SolveFailure) {
	if !results.schema.Available() || !results.store.Available() || !results.generation.Available() || len(results.axes) == 0 || len(results.axes) > solvedAxisCount {
		return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSitePublication).failure()
	}
	materialized := SolvedSnapshot{schema: results.schema}
	builder := snapshot.NewBuilder(results.schema, results.store, results.generation)
	for slot, axis := range results.axes {
		if axis.lane == resultLaneNone || int(axis.lane) >= solvedLaneWidth || materialized.families[axis.lane].Available() {
			return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteAxis).failure()
		}
		family := solvedAxisIdentity(results.schema, axis.lane, solvedAxisFamily)
		denominator := solvedAxisIdentity(results.schema, axis.lane, solvedAxisDenominator)
		if !family.Available() || !denominator.Available() {
			return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteAxis).failure()
		}
		content := snapshot.Content[identity.ContentID, Answer]{
			Rows:        make(map[identity.ContentID]Answer, len(axis.rows)),
			Denominator: denominator,
			Members:     make([]identity.ContentID, 0, len(axis.rows)),
		}
		declared := make(map[identity.ContentID]struct{}, len(axis.rows))
		for _, row := range axis.rows {
			if !row.key.Available() {
				return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteRow).failure()
			}
			if _, duplicate := declared[row.key]; duplicate {
				return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteRow).failure()
			}
			declared[row.key] = struct{}{}
			content.Members = append(content.Members, row.key)
			if row.value != nil {
				content.Rows[row.key] = Answer{value: row.value}
			}
		}
		plan, err := snapshot.DeclareQuery(&builder, family, uint32(slot), content)
		if err != nil || !plan.Available() {
			return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteDeclare).failure()
		}
		materialized.families[axis.lane] = family
	}
	published, err := builder.Seal()
	if err != nil {
		return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteSeal).failure()
	}
	content, minted := solvedContentIdentity(results)
	if !minted {
		return SolvedSnapshot{}, refused(SolveFailureFamilyCompile, solvedSiteContent).failure()
	}
	materialized.published = published
	materialized.content = content
	return materialized, SolveFailure{}
}

// republish publishes the snapshot that follows a materialization with one
// answer changed on lane at generation. An available value publishes the
// answer; an unavailable one withdraws it, which leaves the declared key proven
// absent. The edit copies the changed key's path and shares every other node,
// every other column, and the denominator with the publication it derives from.
//
// The axis it edits is the plan the snapshot itself published, so an edit
// reaches only a column that was published as an answer.
func (materialized SolvedSnapshot) republish(lane resultLane, key identity.ContentID, value solvedValue, generation identity.Generation) (snapshot.Snapshot, SolveFailure) {
	if !materialized.Available() || !key.Available() || !generation.Available() {
		return snapshot.Snapshot{}, refused(SolveFailureFamilyCompile, solvedSiteDelta).failure()
	}
	plan, opened := snapshot.OpenQuery[identity.ContentID, Answer](&materialized.published, materialized.family(lane))
	if !opened {
		return snapshot.Snapshot{}, refused(SolveFailureFamilyCompile, solvedSiteDelta).failure()
	}
	delta := snapshot.NewDelta(materialized.published, generation)
	var edited error
	if value != nil {
		edited = snapshot.SetRow(&delta, plan.Axis(), key, Answer{value: value})
	} else {
		edited = snapshot.RemoveRow(&delta, plan.Axis(), key)
	}
	if edited != nil {
		return snapshot.Snapshot{}, refused(SolveFailureFamilyCompile, solvedSiteDelta).failure()
	}
	published, err := delta.Seal()
	if err != nil {
		return snapshot.Snapshot{}, refused(SolveFailureFamilyCompile, solvedSiteSeal).failure()
	}
	return published, SolveFailure{}
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
