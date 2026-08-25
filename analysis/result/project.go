package result

// This file projects immutable ProgramArtifact geometry and committed Snapshot
// rows into the public Result.

import (
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
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
	compilation composite.Compilation,
	geometry Geometry,
	mounts []programmount.MountedArtifact,
	policy *anadiag.DiagnosticPolicy,
	queries []composite.QueryPublication,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects anadiag.ChannelSelectInput,
	vocabulary structure.Table,
	declarations schemadiag.Table,
	collections composite.DiagnosticCollections,
	contexts executioncontext.Directory,
) (*Projection, bool) {
	if !compilation.Available() || !geometry.Valid() || len(queries) == 0 || published == nil || !published.Published() || !queryPlan.Available() || !observationPlan.Available() || !declarations.Available() || !collections.Available() || !contexts.Available() {
		return nil, false
	}
	diagnosticObservations, publicationsOK := anadiag.Publications(compilation, contexts, geometry.BranchObservations)
	if !publicationsOK {
		return nil, false
	}
	nativeRows, nativeOK := buildNativeBranchPublication(geometry, mounts, diagnosticObservations, published, observationPlan)
	if !nativeOK {
		return nil, false
	}
	result, ok := buildDetachedArtifactResult(geometry, queries, published, queryPlan, nativeRows)
	if !ok || result == nil {
		return nil, false
	}
	projection := &Projection{Result: result}
	if policy != nil && len(policy.Enabled) != 0 {
		report := anadiag.NewReport(result.SourceID(), result.ContentID(), compilation, vocabulary, declarations, collections)
		if !anadiag.CollectReport(report, *policy, geometry.BranchObservations, geometry.ConformanceObservations, geometry.StaticObservations,
			diagnosticObservations, published, observationPlan, selects, contexts) {
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
	nativeRows []nativePublicationRow,
) (*Result, bool) {
	if !geometry.Valid() || published == nil || !published.Published() || !plan.Available() || nativeRows == nil {
		return nil, false
	}
	bodies := make([]resultBody, len(geometry.bodies))
	for index, body := range geometry.bodies {
		bodies[index] = resultBody{id: body.id, roots: append([]resultRoot(nil), body.roots...)}
	}
	// Geometry is the owner of body membership. Validate every membership row
	// before any query is detached, including points that happen not to have a
	// publication in this pass.
	for _, indexes := range geometry.PointBodies {
		seen := make(map[int]struct{}, len(indexes))
		for _, bodyIndex := range indexes {
			if bodyIndex < 0 || bodyIndex >= len(bodies) {
				return nil, false
			}
			if _, duplicate := seen[bodyIndex]; duplicate {
				return nil, false
			}
			seen[bodyIndex] = struct{}{}
		}
	}

	points := make([]resultPoint, 0)
	pointOrdinals := make(map[detachedPointKey]uint32)
	familiesByOrdinal := make(map[uint32]resultFamily)
	maxFamilyOrdinal := uint32(0)
	seenSites := make(map[identity.ContentID]struct{}, len(queries))
	seenPublicationKeys := make(map[identity.ContentID]struct{}, len(queries))
	for _, publication := range queries {
		contextID := publication.Site.Context.ID()
		if !publication.Site.ID.Available() || !publication.Site.Mount.Available() || !publication.Site.Point.Available() ||
			!publication.Site.Context.Available() || !contextID.Available() || publication.Site.Context.ModuleKey() != publication.Site.Mount ||
			!publication.Site.Family.Available() || !publication.Key.Available() {
			return nil, false
		}
		familyOrdinal := publication.FamilyOrdinal()
		if familyOrdinal == 0 || uint64(familyOrdinal) > uint64(len(queries)) {
			return nil, false
		}
		contract := publication.Contract()
		if !contract.Available() {
			return nil, false
		}
		if _, duplicate := seenSites[publication.Site.ID]; duplicate {
			return nil, false
		}
		if _, duplicate := seenPublicationKeys[publication.Key]; duplicate {
			return nil, false
		}
		seenSites[publication.Site.ID] = struct{}{}
		seenPublicationKeys[publication.Key] = struct{}{}

		// Geometry remains the generic mount/point plane. The detached Result
		// point table must additionally retain the canonical context carried by
		// this publication; never recover it from opaque SiteID or Key values.
		geometryPointKey := Point{Mount: publication.Site.Mount, Point: publication.Site.Point}
		pointKey := detachedPointKey{context: contextID, mount: publication.Site.Mount, point: publication.Site.Point}
		pointOrdinal, pointKnown := pointOrdinals[pointKey]
		if !pointKnown {
			indexes := geometry.PointBodies[geometryPointKey]
			bodyOrdinals := make([]uint32, len(indexes))
			for index, bodyIndex := range indexes {
				if bodyIndex < 0 || bodyIndex >= len(bodies) || uint64(bodyIndex+1) > uint64(^uint32(0)) {
					return nil, false
				}
				bodyOrdinals[index] = uint32(bodyIndex + 1)
			}
			if uint64(len(points)+1) > uint64(^uint32(0)) {
				return nil, false
			}
			pointOrdinal = uint32(len(points) + 1)
			pointOrdinals[pointKey] = pointOrdinal
			points = append(points, resultPoint{context: pointKey.context, mount: pointKey.mount, point: pointKey.point, bodies: bodyOrdinals})
		}

		family, familyKnown := familiesByOrdinal[familyOrdinal]
		if !familyKnown {
			family = resultFamily{ordinal: familyOrdinal, key: publication.Site.Family, contract: contract}
			familiesByOrdinal[familyOrdinal] = family
			if familyOrdinal > maxFamilyOrdinal {
				maxFamilyOrdinal = familyOrdinal
			}
		} else if family.key != publication.Site.Family || family.contract != contract {
			return nil, false
		}

		answer, status := snapshot.Query(published, plan, publication.Key)
		row := resultQuery{site: publication.Site.ID, key: publication.Key, point: pointOrdinal}
		switch status {
		case snapshot.ReadProvenAbsent:
			row.status = QueryProvenAbsent
		case snapshot.ReadHit:
			if !answer.Available() {
				return nil, false
			}
			// CanonicalCell is the sole owner callback and is intentionally
			// invoked exactly once for each hit.
			cell, encoded := publication.CanonicalCell(answer)
			if !encoded {
				return nil, false
			}
			row.status, row.cell = QueryHit, cell
		default:
			return nil, false
		}
		if !row.valid(points, family.contract) {
			return nil, false
		}
		family.queries = append(family.queries, row)
		familiesByOrdinal[familyOrdinal] = family
	}
	if maxFamilyOrdinal == 0 {
		return nil, false
	}
	families := make([]resultFamily, int(maxFamilyOrdinal))
	for ordinal := uint32(1); ordinal <= maxFamilyOrdinal; ordinal++ {
		family, present := familiesByOrdinal[ordinal]
		if !present || family.ordinal != ordinal || len(family.queries) == 0 {
			return nil, false
		}
		families[ordinal-1] = family
	}
	nativeContent, nativeRows, nativeByID, nativePublished := sealNativePublication(nativeRows)
	if !nativePublished {
		return nil, false
	}
	content, ok := analysisResultIDWithPublication(geometry.source, bodies, points, families, nativePublished, nativeContent, nativeRows, nativeByID)
	if !ok {
		return nil, false
	}
	result := &Result{
		source:          geometry.source,
		content:         content,
		bodies:          bodies,
		points:          points,
		families:        families,
		nativeContent:   nativeContent,
		nativeRows:      nativeRows,
		nativeByID:      nativeByID,
		nativePublished: nativePublished,
	}
	if !result.validPayload() {
		return nil, false
	}
	result.sealed = true
	return result, true
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

func mountedResultID(role string, mount, artifact, local identity.ContentID) (identity.ContentID, bool) {
	return anadiag.RowID(role, mount, artifact, local)
}

// Geometry is the mount-qualified body/value/observation projection
// built from sealed ingress mounts and Link value substitution. Workspace
// compilation admits one immutable Geometry value; Result detachment consumes
// that owner value without reopening Link, mounts, or diagnostic sites.
type Geometry struct {
	source                  identity.ContentID
	bodies                  []GeometryBody
	values                  map[geometryValueKey]identity.ContentID
	BranchObservations      []anadiag.Observation
	ConformanceObservations []anadiag.Observation
	StaticObservations      []anadiag.Observation
	PointBodies             map[Point][]int
}

// geometryValueKey is the owner-issued mounted Value identity a detached
// Result row is reached by. It is deliberately not Value's private dense
// coordinate: the diagnostic populations carry this portable key from the
// sealed observation site, and Result resolves it once, at its own boundary.
type geometryValueKey struct {
	mount, value identity.ContentID
}

type GeometryBody struct {
	key   artifactResultBody
	id    identity.ContentID
	roots []resultRoot
}

func Project(
	sourceID identity.ContentID,
	mounts []programmount.MountedArtifact,
	coordinates []ValueCoordinate,
	observations []anadiag.Observation,
) (Geometry, bool) {
	if !sourceID.Available() || len(mounts) == 0 || len(coordinates) == 0 {
		return Geometry{}, false
	}
	geometry := Geometry{
		source:                  sourceID,
		bodies:                  make([]GeometryBody, 0),
		values:                  make(map[geometryValueKey]identity.ContentID, len(coordinates)),
		BranchObservations:      make([]anadiag.Observation, 0, len(observations)),
		ConformanceObservations: make([]anadiag.Observation, 0, len(observations)),
		StaticObservations:      make([]anadiag.Observation, 0, len(observations)),
		PointBodies:             make(map[Point][]int),
	}
	for _, observation := range observations {
		if !observation.Available() {
			return Geometry{}, false
		}
		copy := observation
		copy.Branch.Points = append([]identity.ContentID(nil), observation.Branch.Points...)
		copy.Branch.Producers = append([]anadiag.Producer(nil), observation.Branch.Producers...)
		copy.Conformance.Evidence = append([]identity.ContentID(nil), observation.Conformance.Evidence...)
		copy.Conformance.Producers = append([]anadiag.Producer(nil), observation.Conformance.Producers...)
		copy.Path = append([]string(nil), observation.Path...)
		switch observation.Kind {
		case structure.DiagnosticObservationBranchCondition:
			geometry.BranchObservations = append(geometry.BranchObservations, copy)
		case structure.DiagnosticObservationTypeConformance:
			geometry.ConformanceObservations = append(geometry.ConformanceObservations, copy)
		case structure.DiagnosticObservationTypeReferenceUnresolved, structure.DiagnosticObservationValueReferenceUnresolved:
			geometry.StaticObservations = append(geometry.StaticObservations, copy)
		default:
			return Geometry{}, false
		}
	}
	artifactIDs := make(map[identity.ContentID]identity.ContentID, len(mounts))
	bodyIndexes := make(map[artifactResultBody]int)
	for _, mount := range mounts {
		if !mount.Available() {
			return Geometry{}, false
		}
		if _, duplicate := artifactIDs[mount.ModuleKey]; duplicate {
			return Geometry{}, false
		}
		artifactID := mount.Snapshot.ArtifactID()
		artifactIDs[mount.ModuleKey] = artifactID
		localBodies := make(map[identity.ContentID]int)
		bodyCount, bodiesPublished := mount.Program.BodyCount()
		if !bodiesPublished {
			return Geometry{}, false
		}
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, bodyOK := mount.Program.BodyAt(bodyIndex)
			if !bodyOK || !body.ID().Available() {
				return Geometry{}, false
			}
			key := artifactResultBody{mount: mount.ModuleKey, body: body.ID()}
			id, idOK := mountedResultID("body", mount.ModuleKey, artifactID, body.ID())
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
				root, rootOK := mount.Program.BodyRootFor(bodyIndex, rootIndex)
				family := keyspace.Family(root.Family())
				if !rootOK || !root.Available() || family == keyspace.FamilyInvalid {
					return Geometry{}, false
				}
				rootID, rootIDOK := mountedResultID("root", mount.ModuleKey, artifactID, root.ID())
				if !rootIDOK {
					return Geometry{}, false
				}
				if _, duplicate := seenRoots[rootID]; duplicate {
					return Geometry{}, false
				}
				seenRoots[rootID] = struct{}{}
				roots[rootIndex] = resultRoot{id: rootID, family: family}
			}
			geometry.bodies = append(geometry.bodies, GeometryBody{key: key, id: id, roots: roots})
			if body.Callable() {
				continue
			}
			entryBody := localBodies[body.ID()]
			for entryIndex := 0; entryIndex < body.EntryCount(); entryIndex++ {
				entryRow, entryOK := mount.Program.BodyEntryFor(bodyIndex, entryIndex)
				entry := entryRow.PointID()
				if !entryOK || !entry.Available() {
					continue
				}
				pointKey := Point{Mount: mount.ModuleKey, Point: entry}
				geometry.PointBodies[pointKey] = appendUniqueInt(geometry.PointBodies[pointKey], entryBody)
			}
		}
		program := mount.Program.Program
		occurrenceCount, occurrencesPublished := program.OccurrenceCount()
		if !occurrencesPublished {
			return Geometry{}, false
		}
		for occurrenceIndex := 0; occurrenceIndex < occurrenceCount; occurrenceIndex++ {
			occurrence, occurrenceOK := program.OccurrenceAt(occurrenceIndex)
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
			_, pointCount, spanOK := occurrence.PointSpan()
			if !spanOK {
				return Geometry{}, false
			}
			for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
				pointRow, pointOK := program.OccurrencePointFor(occurrenceIndex, pointIndex)
				point := pointRow.PointID()
				if !pointOK || !pointRow.Available() || !point.Available() {
					return Geometry{}, false
				}
				pointKey := Point{Mount: mount.ModuleKey, Point: point}
				geometry.PointBodies[pointKey] = appendUniqueInt(geometry.PointBodies[pointKey], mapped)
			}
		}
	}
	for _, coordinate := range coordinates {
		if !coordinate.id.Available() || !coordinate.mount.Available() {
			return Geometry{}, false
		}
		artifactID, artifactOK := artifactIDs[coordinate.mount]
		id, idOK := mountedResultID("value", coordinate.mount, artifactID, coordinate.id)
		if !artifactOK || !idOK {
			return Geometry{}, false
		}
		key := geometryValueKey{mount: coordinate.mount, value: coordinate.id}
		if _, duplicate := geometry.values[key]; duplicate {
			return Geometry{}, false
		}
		geometry.values[key] = id
	}
	// Branch conditions and conformance subjects are the produced-value
	// populations: both name a Value coordinate and the occurrences that
	// produce it. Their geometry law is stated once, on the observation row
	// itself, and checked above; what remains here is the join this projection
	// owns - the mount's artifact and the coordinate's mount.
	for _, produced := range [][]anadiag.Observation{geometry.BranchObservations, geometry.ConformanceObservations} {
		for _, observation := range produced {
			valueID, measured := observation.MeasuredValueID()
			if !observation.ID.Available() || !observation.Mount.Available() || !observation.Artifact.Available() ||
				!observation.Local.Available() || !measured {
				return Geometry{}, false
			}
			artifactID, artifactOK := artifactIDs[observation.Mount]
			if !artifactOK || artifactID != observation.Artifact {
				return Geometry{}, false
			}
			if _, known := geometry.values[geometryValueKey{mount: observation.Mount, value: valueID}]; !known {
				return Geometry{}, false
			}
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

// valueResultID resolves one mounted portable Value identity to the detached
// Result row it names. A key the census never issued has no row, and none is
// invented for it.
func (geometry Geometry) valueResultID(mount, value identity.ContentID) (identity.ContentID, bool) {
	if !geometry.Valid() || !mount.Available() || !value.Available() {
		return identity.ContentID{}, false
	}
	result, ok := geometry.values[geometryValueKey{mount: mount, value: value}]
	return result, ok && result.Available()
}

// BodySite is the mount-qualified Program body identity at index.
func (geometry Geometry) BodySite(index int) (mount, body identity.ContentID, ok bool) {
	if index < 0 || index >= len(geometry.bodies) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	key := geometry.bodies[index].key
	return key.mount, key.body, key.mount.Available() && key.body.Available()
}
