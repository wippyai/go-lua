package result

// This file projects immutable ProgramArtifact geometry and committed Snapshot
// rows into the public Result.

import (
	"bytes"
	"sort"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// artifactResultProjection is an immutable, detached result. Result
// is retained separately so a future caller can attach diagnostics without
// introducing engine or domain handles into the public projection.
// Projection is a detached Result plus an optional diagnostic report.
type Projection struct {
	Result *Result
	Report *anadiag.DiagnosticReport
}

// detachArtifactResult consumes owner-issued publication keys and the exact
// committed Snapshot. It is intentionally unexported: the
// production Solve lane remains the owner of the transaction and must choose
// when this detached projection becomes public.
//
// Root rows come only from the mount-qualified result geometry. The
// projection never reopens Link, Program, Source, or Flow to recover them.
func Detach(
	geometry Geometry,
	mounts []Mount,
	valueSchema *valuedomain.Schema,
	policy *anadiag.DiagnosticPolicy,
	queries []composite.QueryPublication,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects anadiag.ChannelSelectInput,
) (*Projection, bool) {
	if !geometry.Valid() || len(queries) == 0 || published == nil || !published.Published() || !queryPlan.Available() || !observationPlan.Available() {
		return nil, false
	}
	if !anadiag.BindConditionCoordinates(geometry.BranchObservations, valueSchema) {
		return nil, false
	}
	diagnosticObservations, publicationsOK := anadiag.Publications(geometry.BranchObservations)
	if !publicationsOK {
		return nil, false
	}
	native, nativeOK := buildNativeBranchPublication(geometry, mounts, diagnosticObservations, valueSchema, published, observationPlan)
	if !nativeOK {
		return nil, false
	}
	result, ok := buildDetachedArtifactResult(geometry, queries, published, queryPlan, native)
	if !ok || result == nil {
		return nil, false
	}
	projection := &Projection{Result: result}
	if policy != nil && len(policy.Enabled) != 0 {
		report := anadiag.NewReport(result.SourceID(), result.ContentID())
		summaries, summariesOK := anadiag.QuerySummaries(queries)
		if !summariesOK || !anadiag.CollectReport(report, *policy, geometry.BranchObservations, geometry.StaticObservations, diagnosticObservations, summaries, len(geometry.values), valueSchema, published, queryPlan, observationPlan, selects) {
			return nil, false
		}
		if !report.Available() {
			return nil, false
		}
		projection.Report = report
	}
	return projection, true
}

func buildDetachedArtifactResult(
	geometry Geometry,
	queries []composite.QueryPublication,
	published *snapshot.Snapshot,
	plan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	native *nativePublicationReceipt,
) (*Result, bool) {
	if !geometry.Valid() || published == nil || !published.Published() || !plan.Available() || native == nil || !native.valid() {
		return nil, false
	}
	values := append([]identity.ContentID(nil), geometry.values...)
	bodies := make([]resultBody, len(geometry.bodies))
	for index, body := range geometry.bodies {
		bodies[index] = resultBody{id: body.id, roots: append([]resultRoot(nil), body.roots...), valuePresence: make([]uint64, resultValueWordCount(len(values)))}
	}
	for _, query := range queries {
		key := Point{Mount: query.Site.Mount, Point: query.Site.Point}
		indexes := geometry.PointBodies[key]
		answer, status := snapshot.Query(published, plan, query.Key)
		if status == snapshot.ReadProvenAbsent {
			continue
		}
		if status != snapshot.ReadHit || !answer.Available() {
			return nil, false
		}
		switch query.Site.Projection {
		case queryschema.ProjectionSummary:
			observation, readable := engine.AnswerValue[valuedomain.ValueSummaryObservation](answer)
			if !readable || !observation.Valid {
				return nil, false
			}
			count := len(observation.Values)
			if len(observation.Present) != count || count != len(geometry.values) || observation.Rows > 1 {
				return nil, false
			}
			if observation.Rows == 0 {
				continue
			}
			if len(indexes) == 0 {
				for _, present := range observation.Present {
					if present {
						return nil, false
					}
				}
				continue
			}
			for _, bodyIndex := range indexes {
				if bodyIndex < 0 || bodyIndex >= len(bodies) || len(observation.Present) != len(values) {
					return nil, false
				}
				for valueIndex, present := range observation.Present {
					if present && !setResultValuePresent(bodies[bodyIndex].valuePresence, valueIndex) {
						return nil, false
					}
				}
			}
		case queryschema.ProjectionExact:
			observation, readable := engine.AnswerValue[effectfactor.EffectObservation](answer)
			if !readable || !observation.Valid {
				return nil, false
			}
			if observation.Rows == 0 {
				continue
			}
			if len(indexes) == 0 {
				if observation.Present {
					return nil, false
				}
				continue
			}
			if observation.Rows != 1 {
				return nil, false
			}
			for _, bodyIndex := range indexes {
				bodies[bodyIndex].effectPresent = bodies[bodyIndex].effectPresent || observation.Present
				bodies[bodyIndex].effectTop = bodies[bodyIndex].effectTop || observation.Top
				if !observation.Top {
					bodies[bodyIndex].effects = appendUniqueIDs(bodies[bodyIndex].effects, observation.Atoms)
				}
			}
		default:
			return nil, false
		}
	}
	for index := range bodies {
		if bodies[index].effectTop {
			bodies[index].effects = nil
		} else {
			sort.Slice(bodies[index].effects, func(left, right int) bool {
				return bytes.Compare(bodies[index].effects[left][:], bodies[index].effects[right][:]) < 0
			})
		}
	}
	content, ok := analysisResultIDWithPublication(geometry.source, values, bodies, native)
	if !ok {
		return nil, false
	}
	result := &Result{source: geometry.source, content: content, values: values, bodies: bodies, native: native}
	if !result.validPayload() {
		return nil, false
	}
	result.sealed = true
	return result, true
}

type Point struct {
	Mount identity.ContentID
	Point identity.ContentID
}

type artifactResultBody struct {
	mount identity.ContentID
	body  identity.ContentID
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueIDs(values, additions []identity.ContentID) []identity.ContentID {
	for _, addition := range additions {
		if !addition.Available() {
			continue
		}
		seen := false
		for _, value := range values {
			if value == addition {
				seen = true
				break
			}
		}
		if !seen {
			values = append(values, addition)
		}
	}
	return values
}

func mountedResultID(role string, mount, artifact, local identity.ContentID) (identity.ContentID, bool) {
	return anadiag.RowID(role, mount, artifact, local)
}

// Geometry is the mount-qualified body/value/observation projection
// built from sealed ingress mounts and Link value substitution. It is
// computed when Result is detached; compiledState does not retain it.
type Geometry struct {
	source             identity.ContentID
	bodies             []GeometryBody
	values             []identity.ContentID
	BranchObservations []anadiag.Observation
	StaticObservations []anadiag.Observation
	PointBodies        map[Point][]int
}

type GeometryBody struct {
	key   artifactResultBody
	id    identity.ContentID
	roots []resultRoot
}

func Project(
	sourceID identity.ContentID,
	mounts []Mount,
	coordinates []ValueCoordinate,
	observations []anadiag.Observation,
) (Geometry, bool) {
	if !sourceID.Available() || len(mounts) == 0 || len(coordinates) == 0 {
		return Geometry{}, false
	}
	geometry := Geometry{
		source:             sourceID,
		bodies:             make([]GeometryBody, 0),
		values:             make([]identity.ContentID, len(coordinates)),
		BranchObservations: make([]anadiag.Observation, 0, len(observations)),
		StaticObservations: make([]anadiag.Observation, 0, len(observations)),
		PointBodies:        make(map[Point][]int),
	}
	for _, observation := range observations {
		if !observation.Available() {
			return Geometry{}, false
		}
		copy := observation
		copy.Points = append([]identity.ContentID(nil), observation.Points...)
		copy.Producers = append([]anadiag.Producer(nil), observation.Producers...)
		copy.Path = append([]string(nil), observation.Path...)
		switch observation.Kind {
		case structure.DiagnosticObservationBranchCondition:
			geometry.BranchObservations = append(geometry.BranchObservations, copy)
		case structure.DiagnosticObservationTypeReferenceUnresolved, structure.DiagnosticObservationValueReferenceUnresolved, structure.DiagnosticObservationTypeConformance:
			geometry.StaticObservations = append(geometry.StaticObservations, copy)
		default:
			return Geometry{}, false
		}
	}
	artifactIDs := make(map[identity.ContentID]identity.ContentID, len(mounts))
	bodyIndexes := make(map[artifactResultBody]int)
	for _, mount := range mounts {
		if !mount.Valid() {
			return Geometry{}, false
		}
		if _, duplicate := artifactIDs[mount.Program.ModuleKey]; duplicate {
			return Geometry{}, false
		}
		artifactID := mount.Snapshot.ArtifactID()
		artifactIDs[mount.Program.ModuleKey] = artifactID
		localBodies := make(map[identity.ContentID]int)
		for bodyIndex := 0; bodyIndex < mount.Snapshot.BodyCount(); bodyIndex++ {
			body, bodyOK := mount.Snapshot.BodyAt(bodyIndex)
			if !bodyOK || !body.ID().Available() {
				return Geometry{}, false
			}
			key := artifactResultBody{mount: mount.Program.ModuleKey, body: body.ID()}
			id, idOK := mountedResultID("body", mount.Program.ModuleKey, artifactID, body.ID())
			if !idOK {
				return Geometry{}, false
			}
			if _, duplicate := localBodies[body.ID()]; duplicate {
				return Geometry{}, false
			}
			if _, duplicate := bodyIndexes[key]; duplicate {
				return Geometry{}, false
			}
			localBodies[body.ID()] = len(geometry.bodies)
			bodyIndexes[key] = len(geometry.bodies)
			roots := make([]resultRoot, body.RootCount())
			seenRoots := make(map[identity.ContentID]struct{}, len(roots))
			for rootIndex := range roots {
				root, rootOK := body.RootAt(rootIndex)
				if !rootOK || !root.Available() || root.Family() == keyspace.FamilyInvalid {
					return Geometry{}, false
				}
				rootID, rootIDOK := mountedResultID("root", mount.Program.ModuleKey, artifactID, root.ID())
				if !rootIDOK {
					return Geometry{}, false
				}
				if _, duplicate := seenRoots[rootID]; duplicate {
					return Geometry{}, false
				}
				seenRoots[rootID] = struct{}{}
				roots[rootIndex] = resultRoot{id: rootID, family: root.Family()}
			}
			geometry.bodies = append(geometry.bodies, GeometryBody{key: key, id: id, roots: roots})
			if body.Callable() {
				continue
			}
			entryBody := localBodies[body.ID()]
			for entryIndex := 0; entryIndex < body.EntryCount(); entryIndex++ {
				entry, entryOK := body.EntryAt(entryIndex)
				if !entryOK || !entry.Available() {
					continue
				}
				pointKey := Point{Mount: mount.Program.ModuleKey, Point: entry}
				geometry.PointBodies[pointKey] = appendUniqueInt(geometry.PointBodies[pointKey], entryBody)
			}
		}
		for occurrenceIndex := 0; occurrenceIndex < mount.Snapshot.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := mount.Snapshot.OccurrenceAt(occurrenceIndex)
			if !occurrenceOK || !occurrence.ID().Available() {
				return Geometry{}, false
			}
			bodyID, bodyOK := occurrence.BodyID()
			if !bodyOK {
				continue
			}
			mapped, bodyKnown := localBodies[bodyID]
			if !bodyKnown {
				return Geometry{}, false
			}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return Geometry{}, false
				}
				pointKey := Point{Mount: mount.Program.ModuleKey, Point: point}
				geometry.PointBodies[pointKey] = appendUniqueInt(geometry.PointBodies[pointKey], mapped)
			}
		}
	}
	for index, coordinate := range coordinates {
		if !coordinate.id.Available() || !coordinate.mount.Available() {
			return Geometry{}, false
		}
		artifactID, artifactOK := artifactIDs[coordinate.mount]
		id, idOK := mountedResultID("value", coordinate.mount, artifactID, coordinate.id)
		if !artifactOK || !idOK {
			return Geometry{}, false
		}
		geometry.values[index] = id
	}
	for _, observation := range geometry.BranchObservations {
		if !observation.ID.Available() || !observation.Mount.Available() || !observation.Artifact.Available() ||
			!observation.Local.Available() || len(observation.Producers) == 0 || uint64(observation.ValueIndex) >= uint64(len(coordinates)) ||
			observation.Kind != structure.DiagnosticObservationBranchCondition {
			return Geometry{}, false
		}
		artifactID, artifactOK := artifactIDs[observation.Mount]
		if !artifactOK || artifactID != observation.Artifact {
			return Geometry{}, false
		}
		coordinate := coordinates[observation.ValueIndex]
		if coordinate.mount != observation.Mount {
			return Geometry{}, false
		}
		seenPoints := make(map[identity.ContentID]struct{}, len(observation.Points))
		for _, point := range observation.Points {
			if !point.Available() {
				return Geometry{}, false
			}
			if _, duplicate := seenPoints[point]; duplicate {
				return Geometry{}, false
			}
			seenPoints[point] = struct{}{}
		}
		seenAnchors := make(map[identity.ContentID]struct{}, len(observation.Producers))
		seenExecution := make(map[identity.ContentID]struct{}, len(observation.Producers))
		for _, producer := range observation.Producers {
			if !producer.Key.Available() || !producer.Occurrence.Available() || !producer.Point.Available() || !producer.Anchor.Available() {
				return Geometry{}, false
			}
			if _, known := seenPoints[producer.Anchor]; !known {
				return Geometry{}, false
			}
			if _, duplicate := seenAnchors[producer.Anchor]; duplicate {
				return Geometry{}, false
			}
			if _, duplicate := seenExecution[producer.Point]; duplicate {
				return Geometry{}, false
			}
			seenAnchors[producer.Anchor] = struct{}{}
			seenExecution[producer.Point] = struct{}{}
		}
		if len(seenAnchors) != len(seenPoints) {
			return Geometry{}, false
		}
	}
	for _, observation := range geometry.StaticObservations {
		if !observation.Available() {
			return Geometry{}, false
		}
		switch observation.Kind {
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			if !observation.Reference.Available() || len(observation.Path) == 0 || observation.UnresolvedType.Name == "" {
				return Geometry{}, false
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			if !observation.Read.Available() || !observation.Cell.Available() || observation.UnresolvedValue.Name == "" {
				return Geometry{}, false
			}
		case structure.DiagnosticObservationTypeConformance:
			if !observation.Conformance.Available() {
				return Geometry{}, false
			}
		default:
			return Geometry{}, false
		}
		artifactID, artifactOK := artifactIDs[observation.Mount]
		if !artifactOK || artifactID != observation.Artifact {
			return Geometry{}, false
		}
	}
	return geometry, geometry.Valid()
}

func publishedObservation[R any](published *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], id identity.ContentID) (R, bool) {
	var zero R
	if published == nil || !published.Published() || !plan.Available() || !id.Available() {
		return zero, false
	}
	answer, status := snapshot.Query(published, plan, id)
	if status != snapshot.ReadHit || !answer.Available() {
		return zero, false
	}
	return engine.AnswerValue[R](answer)
}

func (geometry Geometry) Valid() bool {
	return geometry.source.Available() && len(geometry.bodies) != 0 && len(geometry.values) != 0 &&
		geometry.PointBodies != nil
}

// ValueCount is the sealed Value-axis width the geometry projects.
func (geometry Geometry) ValueCount() int {
	if !geometry.Valid() {
		return 0
	}
	return len(geometry.values)
}

// BodySite is the mount-qualified Program body identity at index.
func (geometry Geometry) BodySite(index int) (mount, body identity.ContentID, ok bool) {
	if index < 0 || index >= len(geometry.bodies) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	key := geometry.bodies[index].key
	return key.mount, key.body, key.mount.Available() && key.body.Available()
}
