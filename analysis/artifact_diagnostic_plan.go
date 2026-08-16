package analysis

import (
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	programsource "github.com/wippyai/go-lua/program/source"
)

type artifactDiagnosticObservationReceipt struct {
	point       artifactResultPoint
	observation engine.ReceiptObservation[valueSummaryObservation]
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
	points     []keyspace.ContentID
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
	seenPoints := make(map[keyspace.ContentID]struct{}, len(payload.points))
	for _, point := range payload.points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenPoints[point]; duplicate {
			return false
		}
		seenPoints[point] = struct{}{}
	}
	seenAnchors := make(map[keyspace.ContentID]struct{}, len(payload.producers))
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
	seenExecution := make(map[keyspace.ContentID]struct{}, len(payload.producers))
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
	reference keyspace.ContentID
	root      keyspace.ContentID
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
	read keyspace.ContentID
	cell keyspace.ContentID
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
// enforces the exact mask before the row reaches the persistent result receipt.
type compiledObservation struct {
	id       keyspace.ContentID
	mount    keyspace.ContentID
	artifact keyspace.ContentID
	local    keyspace.ContentID
	kind     programartifact.DiagnosticObservationKind
	location programsource.Span
	compiledBranchObservation
	compiledUnresolvedTypeReference
	compiledUnresolvedValueReference
}

func (observation compiledObservation) available() bool {
	if !observation.id.Available() || !observation.mount.Available() || !observation.artifact.Available() ||
		!observation.local.Available() || !validMountedDiagnosticSpan(observation.location) {
		return false
	}
	switch observation.kind {
	case programartifact.DiagnosticObservationBranchCondition:
		return observation.compiledBranchObservation.available() && observation.compiledUnresolvedTypeReference.empty() && observation.compiledUnresolvedValueReference.empty()
	case programartifact.DiagnosticObservationTypeReferenceUnresolved:
		return observation.compiledUnresolvedTypeReference.available() && observation.compiledBranchObservation.empty() && observation.compiledUnresolvedValueReference.empty()
	case programartifact.DiagnosticObservationValueReferenceUnresolved:
		return observation.compiledUnresolvedValueReference.available() && observation.compiledBranchObservation.empty() && observation.compiledUnresolvedTypeReference.empty()
	default:
		return false
	}
}

type compiledObservationProducer struct {
	role       programartifact.RuleRole
	occurrence keyspace.ContentID
	point      keyspace.ContentID
	anchor     keyspace.ContentID
}

func (producer compiledObservationProducer) available() bool {
	return mountedDiagnosticRuleRole(producer.role) &&
		producer.occurrence.Available() && producer.point.Available() && producer.anchor.Available()
}

// diagnosticEvidenceAnchor resolves a mounted producer's execution point to
// the Program-issued base evidence point used by the branch observation. A
// producer may execute directly at that base point, or after an acyclic chain
// of exact Local stages whose sealed full-environment transfers lead back to
// that base point.
// Factor transports are deliberately not interchangeable with the full
// transfer proof required by branch evidence.
func diagnosticEvidenceAnchor(evidence []keyspace.ContentID, execution keyspace.ContentID, transfers map[keyspace.ContentID]programartifact.LocalTransfer) (keyspace.ContentID, bool) {
	if !execution.Available() || len(evidence) == 0 {
		return keyspace.ContentID{}, false
	}
	for _, point := range evidence {
		if execution == point {
			return point, true
		}
	}
	seen := make(map[keyspace.ContentID]struct{}, len(transfers))
	current := execution
	for steps := 0; steps <= len(transfers); steps++ {
		if _, duplicate := seen[current]; duplicate {
			return keyspace.ContentID{}, false
		}
		seen[current] = struct{}{}
		edge, found := transfers[current]
		if !found || !edge.Available() || !edge.FullEnvironment() || edge.To() != current {
			return keyspace.ContentID{}, false
		}
		current = edge.From()
		for _, point := range evidence {
			if current == point {
				return point, true
			}
		}
	}
	return keyspace.ContentID{}, false
}

// diagnosticLocalTransfersByDestination builds the sealed full-environment
// destination index once per mounted artifact. Destination identity is the
// lookup key; two full rows targeting one execution point are ambiguous even
// when their sources happen to match, so admission rejects them instead of
// selecting one. Factor transports are outside this bridge by design.
func diagnosticLocalTransfersByDestination(artifact *programartifact.Artifact) (map[keyspace.ContentID]programartifact.LocalTransfer, bool) {
	if artifact == nil || !artifact.Available() {
		return nil, false
	}
	transfers := make(map[keyspace.ContentID]programartifact.LocalTransfer, artifact.LocalTransferCount())
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
func addDiagnosticFullLocalTransfer(transfers map[keyspace.ContentID]programartifact.LocalTransfer, edge programartifact.LocalTransfer) bool {
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

func mountedDiagnosticRuleRole(role programartifact.RuleRole) bool {
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		candidate, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			return false
		}
		if candidate == role {
			return true
		}
	}
	return false
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
		mount keyspace.ContentID
		id    keyspace.ContentID
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
		mount keyspace.ContentID
		local keyspace.ContentID
	}]struct{})
	for _, mount := range artifacts.mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() || !mount.artifact.ID().Available() {
			return nil, false
		}
		var localTransfers map[keyspace.ContentID]programartifact.LocalTransfer
		var producersByValue map[keyspace.ContentID][]compiledObservationProducer
		hasBranchObservation := false
		for observationIndex := 0; observationIndex < mount.artifact.DiagnosticObservationCount(); observationIndex++ {
			observation, observationOK := mount.artifact.DiagnosticObservationAt(observationIndex)
			if !observationOK {
				return nil, false
			}
			if observation.Kind() == programartifact.DiagnosticObservationBranchCondition {
				hasBranchObservation = true
				break
			}
		}
		// Build the mounted Value-producer inverse once. Boundary is the exact
		// Link owner of both relations: observation span -> Value and the
		// Program-issued rule-output semantic ID -> Value.
		if hasBranchObservation {
			producersByValue = make(map[keyspace.ContentID][]compiledObservationProducer)
		}
		for roleIndex := 0; hasBranchObservation && roleIndex < programartifact.MountedRuleRoleCount(); roleIndex++ {
			role, roleOK := programartifact.MountedRuleRoleAt(roleIndex)
			if !roleOK {
				return nil, false
			}
			for ruleIndex := 0; ruleIndex < mount.artifact.RuleOccurrenceCount(role); ruleIndex++ {
				rule, ruleOK := mount.artifact.RuleOccurrenceAt(role, ruleIndex)
				if !ruleOK || !rule.Available() {
					return nil, false
				}
				if rule.OutputKind() != programartifact.RuleOutputValue {
					continue
				}
				outputID, outputOK := rule.OutputSemanticID()
				if !outputOK {
					continue
				}
				value, valueOK := values.ForMountedSemantic(mount.moduleKey, outputID)
				// Binary equality's result ID is its Program-owned Span identity.
				// Boundary already seals the exact mounted span substitution.
				if role == programartifact.RuleRoleValueBinaryArithmetic || role == programartifact.RuleRoleValueBinaryEquality || role == programartifact.RuleRoleValueBinaryOrder {
					value, valueOK = values.ForMountedSpan(mount.moduleKey, outputID)
				}
				point, pointOK := rule.PointAt(0)
				if !valueOK {
					continue
				}
				valueID, valueIDOK := values.ID(value)
				if !valueIDOK || !pointOK || !point.Available() || rule.PointCount() != 1 {
					return nil, false
				}
				producers := producersByValue[valueID]
				duplicate := false
				for _, prior := range producers {
					if prior.role == role && prior.occurrence == rule.ID() && prior.point == point {
						duplicate = true
						break
					}
				}
				if !duplicate {
					producersByValue[valueID] = append(producers, compiledObservationProducer{role: role, occurrence: rule.ID(), point: point})
				}
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
				mount keyspace.ContentID
				local keyspace.ContentID
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
			case programartifact.DiagnosticObservationBranchCondition:
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
				anchors := make(map[keyspace.ContentID]struct{}, len(points))
				executions := make(map[keyspace.ContentID]struct{}, len(producers))
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
			case programartifact.DiagnosticObservationTypeReferenceUnresolved:
				unresolved, unresolvedOK := observation.UnresolvedTypeReference()
				path, pathOK := unresolved.Path()
				if !unresolvedOK || !pathOK || !unresolved.StaticReferenceID().Available() {
					return nil, false
				}
				row.compiledUnresolvedTypeReference = compiledUnresolvedTypeReference{reference: unresolved.StaticReferenceID(), root: unresolved.RootID(), path: path}
			case programartifact.DiagnosticObservationValueReferenceUnresolved:
				unresolved, unresolvedOK := observation.UnresolvedValueReference()
				name, nameOK := unresolved.Name()
				if !unresolvedOK || !nameOK || !unresolved.ReadID().Available() || !unresolved.CellID().Available() {
					return nil, false
				}
				// Program proves a binder-implicit global candidate; Link owns the
				// final absence judgment. A configured initial binding suppresses
				// the candidate before it enters the detached result receipt.
				if _, _, _, _, configured := contract.InitialBinding(name); configured {
					continue
				}
				row.compiledUnresolvedValueReference = compiledUnresolvedValueReference{read: unresolved.ReadID(), cell: unresolved.CellID(), name: name}
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
func attachBranchValueObservations(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, binding *programBinding, receipt *artifactResultReceipt) ([]artifactDiagnosticObservationReceipt, engine.ReceiptObservationAttachFailure, bool) {
	if compilation == nil || graph == nil || binding == nil || binding.valueQuery == nil || !receipt.valid() {
		return nil, engine.ReceiptObservationAttachFailureArguments, false
	}
	rows := make([]artifactDiagnosticObservationReceipt, 0)
	seen := make(map[artifactResultPoint]struct{})
	for _, observation := range receipt.branchObservations {
		if observation.kind != programartifact.DiagnosticObservationBranchCondition || !observation.mount.Available() || len(observation.points) == 0 || len(observation.producers) == 0 {
			return nil, engine.ReceiptObservationAttachFailureArguments, false
		}
		for _, producer := range observation.producers {
			executionKey := artifactResultPoint{mount: observation.mount, point: producer.point}
			anchorKey := artifactResultPoint{mount: observation.mount, point: producer.anchor}
			if !executionKey.mount.Available() || !executionKey.point.Available() || !anchorKey.point.Available() {
				return nil, engine.ReceiptObservationAttachFailureArguments, false
			}
			role, roleOK := mountedRole(binding, producer.role)
			member, memberOK := graph.MountedRuleMember(role, observation.mount, producer.point, producer.occurrence)
			if !roleOK || !memberOK {
				return nil, engine.ReceiptObservationAttachFailurePoint, false
			}
			if _, duplicate := seen[executionKey]; duplicate {
				continue
			}
			id, idOK := analysisContentID("analysis/branch-value-observation/v1", executionKey.mount[:], executionKey.point[:], []byte("value-summary"))
			if !idOK {
				return nil, engine.ReceiptObservationAttachFailureArguments, false
			}
			observation, failure := engine.AttachRuleSummaryObservationWithFailure[valuedomain.Value, valueSummaryObservation](compilation, binding.valueQuery, id, member)
			if failure != engine.ReceiptObservationAttachFailureNone || !observation.Available() {
				return nil, failure, false
			}
			seen[executionKey] = struct{}{}
			rows = append(rows, artifactDiagnosticObservationReceipt{point: anchorKey, observation: observation})
		}
	}
	return rows, engine.ReceiptObservationAttachFailureNone, true
}
