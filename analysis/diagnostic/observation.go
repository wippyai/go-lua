package diagnostic

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

const mountedRowIdentityDomain = "analysis/artifact-result/mounted-row/v1"

// RowID is the mount-qualified identity one Result or diagnostic row publishes
// under. Body, root, value, observation, and finding rows share this framing.
func RowID(role string, mount, artifact, local identity.ContentID) (identity.ContentID, bool) {
	if role == "" || !mount.Available() || !artifact.Available() || !local.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(mountedRowIdentityDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(role))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(mount[:])
	_, _ = hash.Write(artifact[:])
	_, _ = hash.Write(local[:])
	return identity.ContentID(hash.Sum(nil)), true
}

// DeclaredType is one declaration as the conformance judgment needs it: the
// runtime families the declared type admits, and the spelling a finding names
// it by. Both are projections of the declared type graph, which this layer
// does not hold: the owner that resolves that graph publishes them together so
// a judgment reads one column instead of rediscovering the declaration.
type DeclaredType struct {
	May      runtimekind.Set
	Spelling string
}

// Available reports a complete declaration: a may-set over the closed runtime
// vocabulary and a spelling the report can render as the declared type.
func (declared DeclaredType) Available() bool {
	return declared.May.Valid() && diagnosticTemplateTokenValid(declared.Spelling)
}

// DeclaredTypes is the published declared-type column, addressed by the static
// type node a conformance site was measured against. It carries every
// declaration the sealed sites name; a missing entry is an incomplete
// publication.
type DeclaredTypes map[identity.ContentID]DeclaredType

// Producer is one mounted branch execution/anchor pair.
type Producer struct {
	Key                       schema.Key
	Occurrence, Point, Anchor identity.ContentID
}

func (producer Producer) Available() bool {
	return producer.Key.Available() &&
		producer.Occurrence.Available() && producer.Point.Available() && producer.Anchor.Available()
}

// Branch is the mounted branch-condition payload.
type Branch struct {
	Points    []identity.ContentID
	Producers []Producer
	ValueID   identity.ContentID
}

func (payload Branch) Available() bool {
	return payload.ValueID.Available() && validProducerGeometry(payload.Points, payload.Producers)
}

// validProducerGeometry is the bijection every produced-value observation is
// admitted under, whatever population declares it: at least one base evidence
// point, one producer per point, no point, anchor, or execution key used twice,
// and every anchor naming a listed evidence point. A second producer sharing
// one base witness would make the anchor ambiguous, so the row fails closed
// rather than selecting one.
func validProducerGeometry(points []identity.ContentID, producers []Producer) bool {
	if len(points) == 0 || len(producers) == 0 || len(points) != len(producers) {
		return false
	}
	seenPoints := make(map[identity.ContentID]struct{}, len(points))
	for _, point := range points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenPoints[point]; duplicate {
			return false
		}
		seenPoints[point] = struct{}{}
	}
	seenAnchors := make(map[identity.ContentID]struct{}, len(producers))
	seenExecution := make(map[identity.ContentID]struct{}, len(producers))
	for _, producer := range producers {
		if !producer.Available() {
			return false
		}
		if _, known := seenPoints[producer.Anchor]; !known {
			return false
		}
		if _, duplicate := seenAnchors[producer.Anchor]; duplicate {
			return false
		}
		if _, duplicate := seenExecution[producer.Point]; duplicate {
			return false
		}
		seenAnchors[producer.Anchor] = struct{}{}
		seenExecution[producer.Point] = struct{}{}
	}
	return len(seenAnchors) == len(seenPoints)
}

// validProducerCoverage is the weaker law a conformance row is admitted under.
// One measured value may be established on several paths, so its producers are
// not in bijection with the base evidence points: what is required is that no
// execution point is claimed twice, that every producer anchors to a listed
// evidence point, and that every evidence point is reached by a producer. The
// collector joins the value over all of them, so a missed producer would read
// a value the program can also carry and abstain on a real violation.
func validProducerCoverage(points []identity.ContentID, producers []Producer) bool {
	if len(points) == 0 || len(producers) == 0 {
		return false
	}
	seenPoints := make(map[identity.ContentID]struct{}, len(points))
	for _, point := range points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenPoints[point]; duplicate {
			return false
		}
		seenPoints[point] = struct{}{}
	}
	anchored := make(map[identity.ContentID]struct{}, len(points))
	seenExecution := make(map[identity.ContentID]struct{}, len(producers))
	for _, producer := range producers {
		if !producer.Available() {
			return false
		}
		if _, known := seenPoints[producer.Anchor]; !known {
			return false
		}
		if _, duplicate := seenExecution[producer.Point]; duplicate {
			return false
		}
		anchored[producer.Anchor] = struct{}{}
		seenExecution[producer.Point] = struct{}{}
	}
	return len(anchored) == len(seenPoints)
}

func (payload Branch) empty() bool {
	return len(payload.Points) == 0 && len(payload.Producers) == 0
}

// UnresolvedType is the sealed unresolved-type payload.
type UnresolvedType struct {
	Reference identity.ContentID
	Root      identity.ContentID
	Path      []string
	Name      string
}

func (payload UnresolvedType) Available() bool {
	if !payload.Reference.Available() || len(payload.Path) == 0 || payload.Name == "" {
		return false
	}
	for _, component := range payload.Path {
		if component == "" {
			return false
		}
	}
	return (len(payload.Path) == 1 && !payload.Root.Available()) || (len(payload.Path) > 1 && payload.Root.Available())
}

func (payload UnresolvedType) empty() bool {
	return !payload.Reference.Available() && !payload.Root.Available() && len(payload.Path) == 0 && payload.Name == ""
}

// UnresolvedValue is the sealed unresolved-value payload.
type UnresolvedValue struct {
	Read identity.ContentID
	Cell identity.ContentID
	Name string
}

func (payload UnresolvedValue) Available() bool {
	return payload.Read.Available() && payload.Cell.Available() && payload.Name != ""
}

func (payload UnresolvedValue) empty() bool {
	return !payload.Read.Available() && !payload.Cell.Available() && payload.Name == ""
}

// Conformance is the sealed TypeConformance payload.
type Conformance struct {
	Site        schemadiag.Site
	Owner       identity.ContentID
	Measured    identity.ContentID
	Declared    identity.ContentID
	Span        identity.ContentID
	Position    uint32
	ValueID     identity.ContentID
	DeclaredMay runtimekind.Set
	Target      string
	// Member is the declared field text a structural site names: the field the
	// constructor did not establish, or the field its member was measured
	// against. It is read from the declared record's own field row, which is
	// the one place the analyzer holds a member's authored spelling after the
	// program's source arena is dropped. A site measured against an array
	// element or a map value names no field and carries none.
	Member string
	// Subject is the authored spelling of the measured expression: the binder
	// name, access path, or call form the source wrote. It is published by the
	// compiler that still holds the authored relations, because this layer
	// holds none of them. A subject the authored projection does not spell -
	// a dynamic key, a literal with no name of its own - carries none, and the
	// finding names its subject by the geometry's own word instead.
	Subject string
	// Callee is the authored spelling of the direct-call target for a call
	// argument site. The Program owner issues it with the observation; this
	// projection never reconstructs it from source or result data.
	Callee   string
	Evidence []identity.ContentID
	// Producers is the execution geometry of the measured value: the rule
	// occurrences that produce it and the base evidence point each one anchors
	// to. It is the same geometry a branch condition carries, because the
	// measured fact is the same one - the value summary at the occurrence that
	// produced the value - and a point-keyed query column is not published at
	// that occurrence.
	Producers []Producer
}

func (payload Conformance) Available() bool {
	if !payload.Site.Available() || !payload.Owner.Available() || !payload.Measured.Available() || !payload.Declared.Available() ||
		!payload.Span.Available() || !payload.ValueID.Available() || !payload.DeclaredMay.Valid() || len(payload.Evidence) == 0 {
		return false
	}
	if payload.Site == schemadiag.SiteCallArgument {
		if !diagnosticTemplateTokenValid(payload.Callee) {
			return false
		}
	} else if payload.Callee != "" {
		return false
	}
	return validProducerCoverage(payload.Evidence, payload.Producers)
}

func (payload Conformance) empty() bool {
	return !payload.Site.Declared() && !payload.Owner.Available() && !payload.Measured.Available() && !payload.Declared.Available() &&
		!payload.Span.Available() && payload.Position == 0 && !payload.ValueID.Available() &&
		payload.DeclaredMay == 0 && payload.Target == "" && payload.Member == "" && payload.Subject == "" && payload.Callee == "" &&
		len(payload.Evidence) == 0 && len(payload.Producers) == 0
}

// Observation is one mounted diagnostic observation row projected from a
// sealed Snapshot site.
type Observation struct {
	ID, Mount, Artifact, Local identity.ContentID
	Kind                       structure.DiagnosticObservationKind
	Location                   programsource.Span
	Branch
	UnresolvedType
	UnresolvedValue
	Conformance
}

// MeasuredValueID returns the owner-issued portable Value identity measured by
// a value-bearing observation. Static observations do not measure a Value.
func (observation Observation) MeasuredValueID() (identity.ContentID, bool) {
	switch observation.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return observation.Branch.ValueID, observation.Branch.ValueID.Available()
	case structure.DiagnosticObservationTypeConformance:
		return observation.Conformance.ValueID, observation.Conformance.ValueID.Available()
	default:
		return identity.ContentID{}, false
	}
}

func validMountedSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}

// Available reports the tagged-union mask for this row's kind.
func (observation Observation) Available() bool {
	if !observation.ID.Available() || !observation.Mount.Available() || !observation.Artifact.Available() ||
		!observation.Local.Available() || !validMountedSpan(observation.Location) {
		return false
	}
	switch observation.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return observation.Branch.Available() && observation.UnresolvedType.empty() && observation.UnresolvedValue.empty() && observation.Conformance.empty()
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		return observation.UnresolvedType.Available() && observation.Branch.empty() && observation.UnresolvedValue.empty() && observation.Conformance.empty()
	case structure.DiagnosticObservationValueReferenceUnresolved:
		return observation.UnresolvedValue.Available() && observation.Branch.empty() && observation.UnresolvedType.empty() && observation.Conformance.empty()
	case structure.DiagnosticObservationTypeConformance:
		return observation.Conformance.Available() && observation.Branch.empty() && observation.UnresolvedType.empty() && observation.UnresolvedValue.empty()
	default:
		return false
	}
}

// ProjectSites materializes mounted observation rows from sealed Snapshot
// sites. Compile-time reconstruction is not a source.
func ProjectSites(sites mounted.ObservationSites, mounts []programmount.MountedArtifact, declared DeclaredTypes) ([]Observation, bool) {
	if !sites.Available() || len(mounts) == 0 {
		return nil, false
	}
	mountByKey := make(map[identity.ContentID]programmount.MountedArtifact, len(mounts))
	for _, mount := range mounts {
		if mount.Snapshot == nil || !mount.Snapshot.Available() || !mount.ModuleKey.Available() {
			return nil, false
		}
		if _, duplicate := mountByKey[mount.ModuleKey]; duplicate {
			return nil, false
		}
		mountByKey[mount.ModuleKey] = mount
	}
	rows := make([]Observation, 0, sites.Count())
	for index := 0; index < sites.Count(); index++ {
		site, siteOK := sites.At(index)
		if !siteOK || !site.Available() {
			return nil, false
		}
		mount, mountOK := mountByKey[site.Mount]
		if !mountOK {
			return nil, false
		}
		program := mount.Snapshot.Program()
		cold, coldOK := program.ColdState()
		view, viewOK := programdiagnostic.NewView(cold)
		if !coldOK || !viewOK {
			return nil, false
		}
		observationIndex, observationOK := diagnosticObservationIndex(view, site.Local)
		observation, observationHeld := view.DiagnosticObservationAt(observationIndex)
		observationOK = observationOK && observationHeld
		if !observationOK || observation.Kind() != site.Kind {
			return nil, false
		}
		artifactID := mount.Snapshot.ArtifactID()
		id, idOK := RowID("diagnostic-observation", site.Mount, artifactID, site.Local)
		if !idOK {
			return nil, false
		}
		row := Observation{
			ID: id, Mount: site.Mount, Artifact: artifactID, Local: site.Local,
			Kind: site.Kind, Location: site.Location,
		}
		switch site.Kind {
		case structure.DiagnosticObservationBranchCondition:
			points, pointsOK := programDiagnosticEvidence(view, observationIndex)
			producers, producersOK := siteProducers(site)
			if !site.ValueID.Available() || !pointsOK || !producersOK {
				return nil, false
			}
			row.Branch = Branch{Points: append([]identity.ContentID(nil), points...), Producers: producers, ValueID: site.ValueID}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			path, pathOK := programDiagnosticPath(view, observationIndex)
			name, nameOK := view.DiagnosticPathName(observationIndex)
			if !pathOK || !nameOK || !observation.StaticReferenceID().Available() {
				return nil, false
			}
			row.UnresolvedType = UnresolvedType{Reference: observation.StaticReferenceID(), Root: observation.RootID(), Path: path, Name: name}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			name := observation.Name()
			if name == "" || !observation.ReadID().Available() || !observation.CellID().Available() {
				return nil, false
			}
			row.UnresolvedValue = UnresolvedValue{Read: observation.ReadID(), Cell: observation.CellID(), Name: name}
		case structure.DiagnosticObservationTypeConformance:
			if !observation.OwnerID().Available() || !observation.MeasuredValueID().Available() ||
				!observation.DeclaredStaticTypeID().Available() || !observation.SpanID().Available() {
				return nil, false
			}
			position, positionOK := observation.Position()
			points, pointsOK := programDiagnosticEvidence(view, observationIndex)
			producers, producersOK := siteProducers(site)
			declaredMay, target, declaredOK := declaredMay(declared, observation.DeclaredStaticTypeID())
			if !positionOK || !pointsOK || !producersOK || !site.ValueID.Available() || !declaredOK {
				return nil, false
			}
			row.Conformance = Conformance{
				Site:  diagnosticObservationSite(observation.Site()),
				Owner: observation.OwnerID(), Measured: observation.MeasuredValueID(),
				Declared: observation.DeclaredStaticTypeID(), Span: observation.SpanID(), Position: position,
				ValueID: site.ValueID, DeclaredMay: declaredMay, Target: target,
				Member:  conformanceMemberText(cold, observation),
				Subject: observation.Name(), Callee: observation.CalleeName(),
				Evidence:  append([]identity.ContentID(nil), points...),
				Producers: producers,
			}
		default:
			return nil, false
		}
		if !row.Available() {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

// siteProducers copies one sealed site's execution geometry. Every population
// that measures a produced value carries it; the copy is per row so no
// projected observation aliases the sealed census.
func siteProducers(site mounted.ObservationSite) ([]Producer, bool) {
	if site.ProducerCount() == 0 {
		return nil, false
	}
	producers := make([]Producer, 0, site.ProducerCount())
	for index := 0; index < site.ProducerCount(); index++ {
		producer, producerOK := site.ProducerAt(index)
		if !producerOK || !producer.Available() {
			return nil, false
		}
		producers = append(producers, Producer{
			Key: producer.Key, Occurrence: producer.Occurrence, Point: producer.Point, Anchor: producer.Anchor,
		})
	}
	return producers, true
}

func diagnosticObservationIndex(view programdiagnostic.View, id identity.ContentID) (int, bool) {
	if !view.Available() || !id.Available() {
		return 0, false
	}
	count, published := view.DiagnosticObservationCount()
	if !published {
		return 0, false
	}
	for index := 0; index < count; index++ {
		row, held := view.DiagnosticObservationAt(index)
		if held && row.ID() == id {
			return index, true
		}
	}
	return 0, false
}

func programDiagnosticEvidence(view programdiagnostic.View, observationIndex int) ([]identity.ContentID, bool) {
	row, held := view.DiagnosticObservationAt(observationIndex)
	if !held {
		return nil, false
	}
	offset, count, spanOK := row.EvidenceSpan()
	if !spanOK || count == 0 {
		return nil, false
	}
	points := make([]identity.ContentID, count)
	for index := uint32(0); index < count; index++ {
		child, childOK := view.DiagnosticEvidenceAt(int(offset + index))
		if !childOK || !child.Available() {
			return nil, false
		}
		points[index] = child.PointID()
	}
	return points, true
}

func programDiagnosticPath(view programdiagnostic.View, observationIndex int) ([]string, bool) {
	row, held := view.DiagnosticObservationAt(observationIndex)
	if !held {
		return nil, false
	}
	offset, count, spanOK := row.PathSpan()
	if !spanOK || count == 0 {
		return nil, false
	}
	path := make([]string, count)
	for index := uint32(0); index < count; index++ {
		child, childOK := view.DiagnosticPathAt(int(offset + index))
		if !childOK || !child.Available() {
			return nil, false
		}
		path[index] = child.Component()
	}
	return path, true
}

func diagnosticObservationSite(site programdiagnostic.DiagnosticObservationSite) schemadiag.Site {
	switch site {
	case programdiagnostic.DiagnosticObservationSiteCallArgument:
		return schemadiag.SiteCallArgument
	case programdiagnostic.DiagnosticObservationSiteAssignment:
		return schemadiag.SiteAssignment
	case programdiagnostic.DiagnosticObservationSiteMember:
		return schemadiag.SiteMember
	case programdiagnostic.DiagnosticObservationSiteMemberAbsent:
		return schemadiag.SiteMemberAbsent
	default:
		return schemadiag.SiteNone
	}
}

// conformanceMemberText is the authored field name a structural conformance
// site names. The site publishes the member's own declared node and, for an
// absent member, that member's ordinal in the declared record, so the field row
// is recovered by matching both columns against the declared record-field
// plane.
//
// An element of an array and a value of a map are declared by no field row, so
// they name no member and the column stays empty rather than borrowing a
// field's spelling from a sibling declaration.
func conformanceMemberText(cold programstate.State, observation programdiagnostic.DiagnosticObservation) string {
	if !observation.Site().Structural() {
		return ""
	}
	view, viewOK := staticnode.NewView(cold)
	count, countOK := view.StaticTypeNodeRecordFieldCount()
	position, positionOK := observation.Position()
	if !viewOK || !countOK || !positionOK {
		return ""
	}
	matched := ""
	for index := 0; index < count; index++ {
		field, held := view.StaticTypeNodeRecordFieldAt(index)
		if !held || !field.Available() || field.ChildID() != observation.DeclaredStaticTypeID() {
			continue
		}
		// An absent member names its ordinal in the declared record, so the two
		// columns together select one field row. An established member names the
		// allocation's ordinal instead, which is a different denominator, so the
		// child node alone selects it - and a node several records share names
		// no single field, which is answered as no member rather than as one of
		// them.
		if observation.Site() == programdiagnostic.DiagnosticObservationSiteMemberAbsent {
			if field.Position() != position {
				continue
			}
			return field.Text()
		}
		if matched != "" && matched != field.Text() {
			return ""
		}
		matched = field.Text()
	}
	return matched
}

// declaredMay reads one declaration out of the published declared-type column.
// The projection itself belongs to the type domain and is performed by the
// owner that holds the whole declared graph; a site whose declaration the
// column does not carry is a defect of that publication, not a declaration
// this row may guess at.
func declaredMay(declared DeclaredTypes, id identity.ContentID) (runtimekind.Set, string, bool) {
	if !id.Available() {
		return 0, "", false
	}
	row, published := declared[id]
	if !published || !row.Available() {
		return 0, "", false
	}
	return row.May, row.Spelling, true
}
