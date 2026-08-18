package analysis

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	publication "github.com/wippyai/go-lua/domain/composite/publication"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

type artifactDiagnosticObservationPublication struct {
	point artifactResultPoint
	key   identity.ContentID
}

func validMountedDiagnosticSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}

// compiledBranchObservation is the mounted form of the branch payload. It
// deliberately contains no static payload fields.
type compiledBranchObservation struct {
	points     []identity.ContentID
	producers  []compiledObservationProducer
	valueIndex uint32
}

func (payload compiledBranchObservation) available() bool {
	if len(payload.points) == 0 || len(payload.producers) == 0 {
		return false
	}
	if len(payload.points) != len(payload.producers) {
		return false
	}
	seenPoints := make(map[identity.ContentID]struct{}, len(payload.points))
	for _, point := range payload.points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenPoints[point]; duplicate {
			return false
		}
		seenPoints[point] = struct{}{}
	}
	seenAnchors := make(map[identity.ContentID]struct{}, len(payload.producers))
	for _, producer := range payload.producers {
		if !producer.available() {
			return false
		}
		if _, known := seenPoints[producer.anchor]; !known {
			return false
		}
		if _, duplicate := seenAnchors[producer.anchor]; duplicate {
			// The evidence point set and the producer anchors form a bijection.
			// A second execution producer cannot silently reuse one base witness.
			return false
		}
		seenAnchors[producer.anchor] = struct{}{}
	}
	if len(seenAnchors) != len(seenPoints) {
		return false
	}
	seenExecution := make(map[identity.ContentID]struct{}, len(payload.producers))
	for _, producer := range payload.producers {
		if _, duplicate := seenExecution[producer.point]; duplicate {
			return false
		}
		seenExecution[producer.point] = struct{}{}
	}
	return true
}

func (payload compiledBranchObservation) empty() bool {
	return len(payload.points) == 0 && len(payload.producers) == 0
}

type compiledUnresolvedTypeReference struct {
	// These remain owner-issued proof atoms copied from the sealed artifact;
	// only compiledObservation.id is used as a mount-qualified identity.
	reference identity.ContentID
	root      identity.ContentID
	path      []string
}

func (payload compiledUnresolvedTypeReference) available() bool {
	if !payload.reference.Available() || len(payload.path) == 0 {
		return false
	}
	for _, component := range payload.path {
		if component == "" {
			return false
		}
	}
	return (len(payload.path) == 1 && !payload.root.Available()) || (len(payload.path) > 1 && payload.root.Available())
}

func (payload compiledUnresolvedTypeReference) empty() bool {
	return !payload.reference.Available() && !payload.root.Available() && len(payload.path) == 0
}

type compiledUnresolvedValueReference struct {
	read identity.ContentID
	cell identity.ContentID
	name string
}

func (payload compiledUnresolvedValueReference) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload compiledUnresolvedValueReference) empty() bool {
	return !payload.read.Available() && !payload.cell.Available() && payload.name == ""
}

// compiledObservation is the one mounted observation carrier. Its payload is
// a strict tagged union: branch geometry is embedded only for branch rows and
// static unresolved references only for static rows. The validity check below
// enforces the exact mask before the row is projected into result geometry.
type compiledObservation struct {
	id       identity.ContentID
	mount    identity.ContentID
	artifact identity.ContentID
	local    identity.ContentID
	kind     structure.DiagnosticObservationKind
	location programsource.Span
	compiledBranchObservation
	compiledUnresolvedTypeReference
	compiledUnresolvedValueReference
	compiledTypeConformance
}

type compiledTypeConformance struct {
	site        diagnostic.Site
	call        identity.ContentID
	argument    identity.ContentID
	declared    identity.ContentID
	span        identity.ContentID
	position    uint32
	actual      uint32
	declaredMay runtimekind.Set
	target      string
	evidence    []identity.ContentID
}

func compiledTypeConformanceSite(site uint8) diagnostic.Site {
	switch site {
	case 1:
		return diagnostic.SiteCallArgument
	case 2:
		return diagnostic.SiteAssignment
	default:
		return diagnostic.SiteNone
	}
}

func (payload compiledTypeConformance) available() bool {
	if !payload.site.Available() || !payload.call.Available() || !payload.argument.Available() || !payload.declared.Available() ||
		!payload.span.Available() || !payload.declaredMay.Valid() || len(payload.evidence) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(payload.evidence))
	for _, point := range payload.evidence {
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

func (payload compiledTypeConformance) empty() bool {
	return !payload.site.Declared() && !payload.call.Available() && !payload.argument.Available() && !payload.declared.Available() &&
		!payload.span.Available() && payload.position == 0 && payload.actual == 0 &&
		payload.declaredMay == 0 && payload.target == "" && len(payload.evidence) == 0
}

func (observation compiledObservation) available() bool {
	if !observation.id.Available() || !observation.mount.Available() || !observation.artifact.Available() ||
		!observation.local.Available() || !validMountedDiagnosticSpan(observation.location) {
		return false
	}
	switch observation.kind {
	case structure.DiagnosticObservationBranchCondition:
		return observation.compiledBranchObservation.available() && observation.compiledUnresolvedTypeReference.empty() && observation.compiledUnresolvedValueReference.empty() && observation.compiledTypeConformance.empty()
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		return observation.compiledUnresolvedTypeReference.available() && observation.compiledBranchObservation.empty() && observation.compiledUnresolvedValueReference.empty() && observation.compiledTypeConformance.empty()
	case structure.DiagnosticObservationValueReferenceUnresolved:
		return observation.compiledUnresolvedValueReference.available() && observation.compiledBranchObservation.empty() && observation.compiledUnresolvedTypeReference.empty() && observation.compiledTypeConformance.empty()
	case structure.DiagnosticObservationTypeConformance:
		return observation.compiledTypeConformance.available() && observation.compiledBranchObservation.empty() && observation.compiledUnresolvedTypeReference.empty() && observation.compiledUnresolvedValueReference.empty()
	default:
		return false
	}
}

type compiledObservationProducer struct {
	key        schema.Key
	occurrence identity.ContentID
	point      identity.ContentID
	anchor     identity.ContentID
}

func (producer compiledObservationProducer) available() bool {
	return producer.key.Available() &&
		producer.occurrence.Available() && producer.point.Available() && producer.anchor.Available()
}

// diagnosticEvidenceAnchor resolves a mounted producer's execution point to
// the Program-issued base evidence point used by the branch observation. A
// producer may execute directly at that base point, or after an acyclic chain
// of exact Local stages whose sealed full-environment transfers lead back to
// that base point.
// Factor transports are deliberately not interchangeable with the full
// transfer proof required by branch evidence.
func diagnosticEvidenceAnchor(evidence []identity.ContentID, execution identity.ContentID, transfers map[identity.ContentID]programartifact.LocalTransfer) (identity.ContentID, bool) {
	if !execution.Available() || len(evidence) == 0 {
		return identity.ContentID{}, false
	}
	for _, point := range evidence {
		if execution == point {
			return point, true
		}
	}
	seen := make(map[identity.ContentID]struct{}, len(transfers))
	current := execution
	for steps := 0; steps <= len(transfers); steps++ {
		if _, duplicate := seen[current]; duplicate {
			return identity.ContentID{}, false
		}
		seen[current] = struct{}{}
		edge, found := transfers[current]
		if !found || !edge.Available() || !edge.FullEnvironment() || edge.To() != current {
			return identity.ContentID{}, false
		}
		current = edge.From()
		for _, point := range evidence {
			if current == point {
				return point, true
			}
		}
	}
	return identity.ContentID{}, false
}

// diagnosticLocalTransfersByDestination builds the sealed full-environment
// destination index once per mounted artifact. Destination identity is the
// lookup key; two full rows targeting one execution point are ambiguous even
// when their sources happen to match, so admission rejects them instead of
// selecting one. Factor transports are outside this bridge by design.
func diagnosticLocalTransfersByDestination(artifact *programartifact.Artifact) (map[identity.ContentID]programartifact.LocalTransfer, bool) {
	if artifact == nil || !artifact.Available() {
		return nil, false
	}
	transfers := make(map[identity.ContentID]programartifact.LocalTransfer, artifact.LocalTransferCount())
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		edge, edgeOK := artifact.LocalTransferAt(index)
		if !edgeOK || !addDiagnosticFullLocalTransfer(transfers, edge) {
			return nil, false
		}
	}
	return transfers, true
}

// addDiagnosticFullLocalTransfer admits one sealed transfer into the branch
// destination index. It is intentionally a map mutation rather than a second
// transfer slice: factor rows are ignored and duplicate full destinations are
// rejected at the point of admission.
func addDiagnosticFullLocalTransfer(transfers map[identity.ContentID]programartifact.LocalTransfer, edge programartifact.LocalTransfer) bool {
	if transfers == nil || !edge.Available() {
		return false
	}
	// Branch evidence requires a complete environment witness. Factor
	// transports may legitimately share a destination with other stage rows
	// and are not candidates for this index.
	if !edge.FullEnvironment() {
		return true
	}
	if _, duplicate := transfers[edge.To()]; duplicate {
		return false
	}
	transfers[edge.To()] = edge
	return true
}

// compileDiagnosticObservations emits the one generic mounted observation
// carrier. No family-specific row type crosses the artifact boundary.
func compileDiagnosticObservations(source *link.Link, artifacts *compiledArtifactSet, coordinates []compiledValueCoordinate) ([]compiledObservation, bool) {
	if source == nil || source.Boundary() == nil || artifacts == nil || len(artifacts.mounts) == 0 {
		return nil, false
	}
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil {
		return nil, false
	}
	type valueKey struct {
		mount identity.ContentID
		id    identity.ContentID
	}
	coordinateByID := make(map[valueKey]uint32, len(coordinates))
	for index, coordinate := range coordinates {
		if uint64(index) > uint64(^uint32(0)) || !coordinate.mount.Available() || !coordinate.id.Available() {
			return nil, false
		}
		key := valueKey{mount: coordinate.mount, id: coordinate.id}
		if _, duplicate := coordinateByID[key]; duplicate {
			return nil, false
		}
		coordinateByID[key] = uint32(index)
	}
	values := source.Boundary().Values()
	rows := make([]compiledObservation, 0)
	seen := make(map[struct {
		mount identity.ContentID
		local identity.ContentID
	}]struct{})
	for _, mount := range artifacts.mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() || !mount.artifact.ID().Available() {
			return nil, false
		}
		var localTransfers map[identity.ContentID]programartifact.LocalTransfer
		var producersByValue map[identity.ContentID][]compiledObservationProducer
		hasBranchObservation := false
		for observationIndex := 0; observationIndex < mount.artifact.DiagnosticObservationCount(); observationIndex++ {
			observation, observationOK := mount.artifact.DiagnosticObservationAt(observationIndex)
			if !observationOK {
				return nil, false
			}
			if observation.Kind() == structure.DiagnosticObservationBranchCondition {
				hasBranchObservation = true
				break
			}
		}
		// Build the mounted Value-producer inverse once. Boundary is the exact
		// Link owner of both relations: observation span -> Value and the
		// Program-issued rule-output semantic ID -> Value.
		if hasBranchObservation {
			producersByValue = make(map[identity.ContentID][]compiledObservationProducer)
		}
		for ruleIndex := 0; hasBranchObservation && ruleIndex < mount.artifact.RulePlacementCount(); ruleIndex++ {
			rule, ruleOK := mount.artifact.RulePlacementAt(ruleIndex)
			if !ruleOK || !rule.Available() {
				return nil, false
			}
			if _, value := rule.OutputSemanticID(); !value {
				continue
			}
			outputID, outputOK := rule.OutputSemanticID()
			if !outputOK {
				continue
			}
			value, valueOK := values.ForMountedSemantic(mount.moduleKey, outputID)
			// Computation families write the operator's Program-owned Span identity.
			// Boundary already seals the exact mounted span substitution.
			if programartifact.SpanResultOccurrence(rule.OccurrenceKind()) {
				value, valueOK = values.ForMountedSpan(mount.moduleKey, outputID)
			}
			point, pointOK := rule.PointAt(0)
			if !valueOK {
				continue
			}
			valueID, valueIDOK := values.ID(value)
			if !valueIDOK || !pointOK || !point.Available() || rule.PointCount() != 1 || !rule.Key().Available() {
				return nil, false
			}
			producers := producersByValue[valueID]
			duplicate := false
			for _, prior := range producers {
				if prior.key == rule.Key() && prior.occurrence == rule.ID() && prior.point == point {
					duplicate = true
					break
				}
			}
			if !duplicate {
				producersByValue[valueID] = append(producers, compiledObservationProducer{key: rule.Key(), occurrence: rule.ID(), point: point})
			}
		}
		for index := 0; index < mount.artifact.DiagnosticObservationCount(); index++ {
			observation, observationOK := mount.artifact.DiagnosticObservationAt(index)
			if !observationOK || !observation.Available() {
				return nil, false
			}
			localID := observation.ID()
			id, idOK := mountedResultID("diagnostic-observation", mount.moduleKey, mount.artifact.ID(), localID)
			key := struct {
				mount identity.ContentID
				local identity.ContentID
			}{mount: mount.moduleKey, local: localID}
			if !idOK {
				return nil, false
			}
			if _, duplicate := seen[key]; duplicate {
				return nil, false
			}
			seen[key] = struct{}{}
			location, locationOK := observation.Location()
			if !locationOK {
				return nil, false
			}
			row := compiledObservation{id: id, mount: mount.moduleKey, artifact: mount.artifact.ID(), local: localID, kind: observation.Kind(), location: location}
			switch observation.Kind() {
			case structure.DiagnosticObservationBranchCondition:
				branch, branchOK := observation.BranchCondition()
				if !branchOK {
					return nil, false
				}
				value, valueOK := values.ForMountedSpan(mount.moduleKey, branch.ValueSpanID())
				valueID, valueIDOK := values.ID(value)
				valueIndex, valueIndexOK := coordinateByID[valueKey{mount: mount.moduleKey, id: valueID}]
				if !valueOK || !valueIDOK || !valueIndexOK {
					return nil, false
				}
				producers := append([]compiledObservationProducer(nil), producersByValue[valueID]...)
				points, pointsOK := branch.EvidencePoints()
				if !pointsOK || uint64(valueIndex) >= uint64(len(coordinates)) {
					return nil, false
				}
				// A Program branch is a complete semantic fact even when this
				// mounted target has no producer role for its value (for example
				// unary/claim families).  Such a row is unsupported by this
				// diagnostic projector, not malformed artifact geometry; omit it
				// without making the whole plan unsupported.
				if len(producers) == 0 {
					continue
				}
				if localTransfers == nil {
					var localTransfersOK bool
					localTransfers, localTransfersOK = diagnosticLocalTransfersByDestination(mount.artifact)
					if !localTransfersOK {
						return nil, false
					}
				}
				anchors := make(map[identity.ContentID]struct{}, len(points))
				executions := make(map[identity.ContentID]struct{}, len(producers))
				for producerIndex := range producers {
					anchor, anchorOK := diagnosticEvidenceAnchor(points, producers[producerIndex].point, localTransfers)
					if !anchorOK {
						return nil, false
					}
					if _, duplicate := executions[producers[producerIndex].point]; duplicate {
						return nil, false
					}
					executions[producers[producerIndex].point] = struct{}{}
					if _, duplicate := anchors[anchor]; duplicate {
						return nil, false
					}
					anchors[anchor] = struct{}{}
					producers[producerIndex].anchor = anchor
				}
				if len(anchors) != len(points) {
					return nil, false
				}
				row.compiledBranchObservation = compiledBranchObservation{points: points, producers: producers, valueIndex: valueIndex}
			case structure.DiagnosticObservationTypeReferenceUnresolved:
				unresolved, unresolvedOK := observation.UnresolvedTypeReference()
				path, pathOK := unresolved.Path()
				if !unresolvedOK || !pathOK || !unresolved.StaticReferenceID().Available() {
					return nil, false
				}
				row.compiledUnresolvedTypeReference = compiledUnresolvedTypeReference{reference: unresolved.StaticReferenceID(), root: unresolved.RootID(), path: path}
			case structure.DiagnosticObservationValueReferenceUnresolved:
				unresolved, unresolvedOK := observation.UnresolvedValueReference()
				name, nameOK := unresolved.Name()
				if !unresolvedOK || !nameOK || !unresolved.ReadID().Available() || !unresolved.CellID().Available() {
					return nil, false
				}
				// Program proves a binder-implicit global candidate; Link owns the
				// final absence judgment. A configured initial binding suppresses
				// the candidate before it enters result geometry.
				if _, _, _, _, configured := contract.InitialBinding(name); configured {
					continue
				}
				row.compiledUnresolvedValueReference = compiledUnresolvedValueReference{read: unresolved.ReadID(), cell: unresolved.CellID(), name: name}
			case structure.DiagnosticObservationTypeConformance:
				conformance, conformanceOK := observation.TypeConformance()
				if !conformanceOK || !conformance.CallID().Available() || !conformance.ArgumentID().Available() ||
					!conformance.DeclaredStaticTypeID().Available() || !conformance.SpanID().Available() {
					return nil, false
				}
				position, positionOK := conformance.Position()
				points, pointsOK := conformance.EvidencePoints()
				memberID, spanID, argumentOK := mountedCallArgumentIdentities(mount.artifact, conformance.ArgumentID())
				if !positionOK || !pointsOK || !argumentOK || spanID != conformance.SpanID() {
					return nil, false
				}
				value, valueOK := values.ForMountedSemantic(mount.moduleKey, memberID)
				if !valueOK {
					value, valueOK = values.ForMountedSpan(mount.moduleKey, spanID)
				}
				valueID, valueIDOK := values.ID(value)
				valueIndex, valueIndexOK := coordinateByID[valueKey{mount: mount.moduleKey, id: valueID}]
				declaredMay, target, declaredOK := mountedDeclaredMay(mount.artifact, conformance.DeclaredStaticTypeID())
				if !valueOK || !valueIDOK || !valueIndexOK || !declaredOK || uint64(valueIndex) >= uint64(len(coordinates)) {
					return nil, false
				}
				row.compiledTypeConformance = compiledTypeConformance{
					site: compiledTypeConformanceSite(conformance.Site()),
					call: conformance.CallID(), argument: conformance.ArgumentID(),
					declared: conformance.DeclaredStaticTypeID(), span: conformance.SpanID(), position: position,
					actual: valueIndex, declaredMay: declaredMay, target: target,
					evidence: append([]identity.ContentID(nil), points...),
				}
			default:
				return nil, false
			}
			if !row.available() {
				return nil, false
			}
			rows = append(rows, row)
		}
	}
	return rows, true
}

// attachBranchValueObservations creates the one solve-local Value read shared
// by native publication and optional diagnostics. Static observations never
// enter this function. Diagnostic flags control only report collectors; they
// cannot create a second observation authority or alter native facts.
func attachBranchValueObservations(compilation *engine.ProgramConstruction, graph *engine.ReceiptGraph, binding *composite.ProgramBinding, geometry resultGeometry) ([]artifactDiagnosticObservationPublication, engine.ReceiptObservationAttachFailure, bool) {
	if compilation == nil || graph == nil || binding == nil || binding.ValueQuery() == nil || !geometry.valid() {
		return nil, engine.ReceiptObservationAttachFailureArguments, false
	}
	family, familyOK := composite.ObservationProducerForPopulationKind(structure.DiagnosticObservationBranchCondition.Key())
	if !familyOK {
		return nil, engine.ReceiptObservationAttachFailureArguments, false
	}
	rows := make([]artifactDiagnosticObservationPublication, 0)
	seen := make(map[artifactResultPoint]publication.BranchValueObservationAttachment)
	for _, observation := range geometry.branchObservations {
		if observation.kind != structure.DiagnosticObservationBranchCondition || !observation.mount.Available() || len(observation.points) == 0 || len(observation.producers) == 0 {
			return nil, engine.ReceiptObservationAttachFailureArguments, false
		}
		for _, producer := range observation.producers {
			executionKey := artifactResultPoint{mount: observation.mount, point: producer.point}
			anchorKey := artifactResultPoint{mount: observation.mount, point: producer.anchor}
			if !executionKey.mount.Available() || !executionKey.point.Available() || !anchorKey.point.Available() {
				return nil, engine.ReceiptObservationAttachFailureArguments, false
			}
			role, roleOK := mountedCapability(binding, producer.key)
			if !roleOK {
				return nil, engine.ReceiptObservationAttachFailurePoint, false
			}
			if attached, duplicate := seen[executionKey]; duplicate {
				// Two branch subjects may share one evidence point. The later
				// producer names its own member there, so the attachment already
				// bound to that point reauthenticates those coordinates instead
				// of adopting them unchecked.
				if !attached.MemberAdmitted(compilation, role, producer.occurrence) {
					return nil, engine.ReceiptObservationAttachFailurePoint, false
				}
				continue
			}
			attachment, failure, attached := publication.AttachBranchValueObservation(compilation, binding.ValueQuery(), role, family, executionKey.mount, producer.point, producer.occurrence)
			if !attached {
				return nil, failure, false
			}
			key, keyed := attachment.ContentID()
			if !keyed {
				return nil, engine.ReceiptObservationAttachFailurePoint, false
			}
			seen[executionKey] = attachment
			rows = append(rows, artifactDiagnosticObservationPublication{point: anchorKey, key: key})
		}
	}
	return rows, engine.ReceiptObservationAttachFailureNone, true
}

func mountedCallArgumentIdentities(artifact *programartifact.Artifact, argumentID identity.ContentID) (identity.ContentID, identity.ContentID, bool) {
	if artifact == nil || !artifact.Available() || !argumentID.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	for index := 0; index < artifact.CallArgumentCount(); index++ {
		row, rowOK := artifact.CallArgumentAt(index)
		if !rowOK || row.ID() != argumentID {
			continue
		}
		return row.MemberID(), row.SpanID(), row.MemberID().Available() && row.SpanID().Available()
	}
	return identity.ContentID{}, identity.ContentID{}, false
}

func mountedDeclaredMay(artifact *programartifact.Artifact, declared identity.ContentID) (runtimekind.Set, string, bool) {
	if artifact == nil || !artifact.Available() || !declared.Available() {
		return 0, "", false
	}
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		node, nodeOK := artifact.StaticTypeNodeAt(index)
		if !nodeOK || node.ID() != declared {
			continue
		}
		if node.Kind() != programartifact.StaticNodePrimitive {
			return runtimekind.All, "", true
		}
		return primitiveDeclaredMay(programstatic.PrimitiveKind(node.LiteralKind()))
	}
	return 0, "", false
}

func primitiveDeclaredMay(kind programstatic.PrimitiveKind) (runtimekind.Set, string, bool) {
	switch kind {
	case programstatic.PrimitiveNil:
		return runtimekind.Bit(runtimekind.Nil), "nil", true
	case programstatic.PrimitiveBoolean:
		return runtimekind.Bit(runtimekind.Boolean), "boolean", true
	case programstatic.PrimitiveNumber:
		return runtimekind.Bit(runtimekind.Number), "number", true
	case programstatic.PrimitiveInteger:
		return runtimekind.Bit(runtimekind.Number), "integer", true
	case programstatic.PrimitiveString:
		return runtimekind.Bit(runtimekind.String), "string", true
	case programstatic.PrimitiveFunction:
		return runtimekind.Bit(runtimekind.Function), "function", true
	case programstatic.PrimitiveAny:
		return runtimekind.All, "any", true
	case programstatic.PrimitiveUnknown:
		return runtimekind.All, "unknown", true
	case programstatic.PrimitiveNever:
		return 0, "never", true
	case programstatic.PrimitiveSelf:
		return runtimekind.All, "self", true
	default:
		return 0, "", false
	}
}
