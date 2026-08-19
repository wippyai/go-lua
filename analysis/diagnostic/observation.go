package diagnostic

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/analysis/schema"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
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

// MountedCensus is one sealed ingress mount the site projector reads.
type MountedCensus struct {
	ModuleKey identity.ContentID
	Snapshot  *ingress.Snapshot
}

// ValueCoordinate is one Link-substituted Value cell the census joins.
type ValueCoordinate struct {
	Mount, ID identity.ContentID
}

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
	Points     []identity.ContentID
	Producers  []Producer
	ValueIndex uint32
}

func (payload Branch) Available() bool {
	if len(payload.Points) == 0 || len(payload.Producers) == 0 || len(payload.Points) != len(payload.Producers) {
		return false
	}
	seenPoints := make(map[identity.ContentID]struct{}, len(payload.Points))
	for _, point := range payload.Points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenPoints[point]; duplicate {
			return false
		}
		seenPoints[point] = struct{}{}
	}
	seenAnchors := make(map[identity.ContentID]struct{}, len(payload.Producers))
	for _, producer := range payload.Producers {
		if !producer.Available() {
			return false
		}
		if _, known := seenPoints[producer.Anchor]; !known {
			return false
		}
		if _, duplicate := seenAnchors[producer.Anchor]; duplicate {
			return false
		}
		seenAnchors[producer.Anchor] = struct{}{}
	}
	if len(seenAnchors) != len(seenPoints) {
		return false
	}
	seenExecution := make(map[identity.ContentID]struct{}, len(payload.Producers))
	for _, producer := range payload.Producers {
		if _, duplicate := seenExecution[producer.Point]; duplicate {
			return false
		}
		seenExecution[producer.Point] = struct{}{}
	}
	return true
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
	Call        identity.ContentID
	Argument    identity.ContentID
	Declared    identity.ContentID
	Span        identity.ContentID
	Position    uint32
	Actual      uint32
	DeclaredMay runtimekind.Set
	Target      string
	Evidence    []identity.ContentID
}

func (payload Conformance) Available() bool {
	if !payload.Site.Available() || !payload.Call.Available() || !payload.Argument.Available() || !payload.Declared.Available() ||
		!payload.Span.Available() || !payload.DeclaredMay.Valid() || len(payload.Evidence) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(payload.Evidence))
	for _, point := range payload.Evidence {
		if !point.Available() {
			return false
		}
		if _, duplicate := seen[point]; duplicate {
			return false
		}
		seen[point] = struct{}{}
	}
	return true
}

func (payload Conformance) empty() bool {
	return !payload.Site.Declared() && !payload.Call.Available() && !payload.Argument.Available() && !payload.Declared.Available() &&
		!payload.Span.Available() && payload.Position == 0 && payload.Actual == 0 &&
		payload.DeclaredMay == 0 && payload.Target == "" && len(payload.Evidence) == 0
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
func ProjectSites(sites mounted.ObservationSites, mounts []MountedCensus, coordinates []ValueCoordinate) ([]Observation, bool) {
	if !sites.Available() || len(mounts) == 0 {
		return nil, false
	}
	type valueKey struct {
		mount identity.ContentID
		id    identity.ContentID
	}
	coordinateByID := make(map[valueKey]uint32, len(coordinates))
	for index, coordinate := range coordinates {
		if uint64(index) > uint64(^uint32(0)) || !coordinate.Mount.Available() || !coordinate.ID.Available() {
			return nil, false
		}
		key := valueKey{mount: coordinate.Mount, id: coordinate.ID}
		if _, duplicate := coordinateByID[key]; duplicate {
			return nil, false
		}
		coordinateByID[key] = uint32(index)
	}
	mountByKey := make(map[identity.ContentID]MountedCensus, len(mounts))
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
		observation, observationOK := mount.Snapshot.DiagnosticObservationForID(site.Local)
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
			valueIndex, valueOK := coordinateByID[valueKey{mount: site.Mount, id: site.ValueID}]
			branch, branchOK := observation.BranchCondition()
			points, pointsOK := branch.EvidencePoints()
			if !valueOK || !branchOK || !pointsOK || uint64(valueIndex) >= uint64(len(coordinates)) || site.ProducerCount() == 0 {
				return nil, false
			}
			producers := make([]Producer, 0, site.ProducerCount())
			for producerIndex := 0; producerIndex < site.ProducerCount(); producerIndex++ {
				producer, producerOK := site.ProducerAt(producerIndex)
				if !producerOK || !producer.Available() {
					return nil, false
				}
				producers = append(producers, Producer{
					Key: producer.Key, Occurrence: producer.Occurrence, Point: producer.Point, Anchor: producer.Anchor,
				})
			}
			row.Branch = Branch{Points: append([]identity.ContentID(nil), points...), Producers: producers, ValueIndex: valueIndex}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			unresolved, unresolvedOK := observation.UnresolvedTypeReference()
			path, pathOK := unresolved.Path()
			name, nameOK := unresolved.Name()
			if !unresolvedOK || !pathOK || !nameOK || !unresolved.StaticReferenceID().Available() {
				return nil, false
			}
			row.UnresolvedType = UnresolvedType{Reference: unresolved.StaticReferenceID(), Root: unresolved.RootID(), Path: path, Name: name}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			unresolved, unresolvedOK := observation.UnresolvedValueReference()
			name, nameOK := unresolved.Name()
			if !unresolvedOK || !nameOK || !unresolved.ReadID().Available() || !unresolved.CellID().Available() {
				return nil, false
			}
			row.UnresolvedValue = UnresolvedValue{Read: unresolved.ReadID(), Cell: unresolved.CellID(), Name: name}
		case structure.DiagnosticObservationTypeConformance:
			conformance, conformanceOK := observation.TypeConformance()
			if !conformanceOK || !conformance.CallID().Available() || !conformance.ArgumentID().Available() ||
				!conformance.DeclaredStaticTypeID().Available() || !conformance.SpanID().Available() {
				return nil, false
			}
			position, positionOK := conformance.Position()
			points, pointsOK := conformance.EvidencePoints()
			valueIndex, valueOK := coordinateByID[valueKey{mount: site.Mount, id: site.ValueID}]
			declaredMay, target, declaredOK := declaredMay(mount.Snapshot, conformance.DeclaredStaticTypeID())
			if !positionOK || !pointsOK || !valueOK || !declaredOK || uint64(valueIndex) >= uint64(len(coordinates)) {
				return nil, false
			}
			row.Conformance = Conformance{
				Site: conformance.Site(),
				Call: conformance.CallID(), Argument: conformance.ArgumentID(),
				Declared: conformance.DeclaredStaticTypeID(), Span: conformance.SpanID(), Position: position,
				Actual: valueIndex, DeclaredMay: declaredMay, Target: target,
				Evidence: append([]identity.ContentID(nil), points...),
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

func declaredMay(snapshot *ingress.Snapshot, declared identity.ContentID) (runtimekind.Set, string, bool) {
	if snapshot == nil || !declared.Available() {
		return 0, "", false
	}
	node, nodeOK := snapshot.StaticTypeNodeForID(declared)
	if !nodeOK {
		return 0, "", false
	}
	if node.Kind() != uint8(programartifact.StaticNodePrimitive) {
		return runtimekind.All, "", true
	}
	return primitiveDeclaredMay(statictypes.PrimitiveKind(node.LiteralKind()))
}

func primitiveDeclaredMay(kind statictypes.PrimitiveKind) (runtimekind.Set, string, bool) {
	switch kind {
	case statictypes.PrimitiveNil:
		return runtimekind.Bit(runtimekind.Nil), "nil", true
	case statictypes.PrimitiveBoolean:
		return runtimekind.Bit(runtimekind.Boolean), "boolean", true
	case statictypes.PrimitiveNumber:
		return runtimekind.Bit(runtimekind.Number), "number", true
	case statictypes.PrimitiveInteger:
		return runtimekind.Bit(runtimekind.Number), "integer", true
	case statictypes.PrimitiveString:
		return runtimekind.Bit(runtimekind.String), "string", true
	case statictypes.PrimitiveFunction:
		return runtimekind.Bit(runtimekind.Function), "function", true
	case statictypes.PrimitiveAny:
		return runtimekind.All, "any", true
	case statictypes.PrimitiveUnknown:
		return runtimekind.All, "unknown", true
	case statictypes.PrimitiveNever:
		return 0, "never", true
	case statictypes.PrimitiveSelf:
		return runtimekind.All, "self", true
	default:
		return 0, "", false
	}
}
