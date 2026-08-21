package programdiagnostic

import (
	"crypto/sha256"
	"hash"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/internal/framing"
)

// These payloads are construction-local inputs to the canonical diagnostic
// identity equations. They are never retained by a DiagnosticObservation or
// exposed as a compiler-facing representation.
type branchConditionIdentityPayload struct {
	decision identity.ContentID
	value    identity.ContentID
	points   []identity.ContentID
}

type unresolvedTypeReferenceIdentityPayload struct {
	reference identity.ContentID
	root      identity.ContentID
	path      []string
}

type unresolvedValueReferenceIdentityPayload struct {
	read identity.ContentID
	cell identity.ContentID
	name string
}

type typeConformanceIdentityPayload struct {
	site     DiagnosticObservationSite
	owner    identity.ContentID
	measured identity.ContentID
	declared identity.ContentID
	span     identity.ContentID
	position uint32
	points   []identity.ContentID
}

func (payload branchConditionIdentityPayload) available() bool {
	return payload.decision.Available() && payload.value.Available() && validEvidencePoints(payload.points)
}

func (payload unresolvedTypeReferenceIdentityPayload) available() bool {
	if !payload.reference.Available() || len(payload.path) == 0 {
		return false
	}
	for _, component := range payload.path {
		if component == "" {
			return false
		}
	}
	return (len(payload.path) == 1 && !payload.root.Available()) ||
		(len(payload.path) > 1 && payload.root.Available())
}

func (payload unresolvedValueReferenceIdentityPayload) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload typeConformanceIdentityPayload) available() bool {
	return payload.site.Valid() && payload.owner.Available() && payload.measured.Available() &&
		payload.declared.Available() && payload.span.Available() && validEvidencePoints(payload.points)
}

var diagnosticHashSinks sync.Pool

func acquireDiagnosticHash() hash.Hash {
	if hashValue, ok := diagnosticHashSinks.Get().(hash.Hash); ok && hashValue != nil {
		hashValue.Reset()
		return hashValue
	}
	return sha256.New()
}

func releaseDiagnosticHash(hashValue hash.Hash) {
	if hashValue != nil {
		diagnosticHashSinks.Put(hashValue)
	}
}

// BranchConditionIdentity returns the canonical identity of one branch
// diagnostic observation. The points are consumed synchronously in their
// supplied order and are not retained.
func BranchConditionIdentity(programID identity.ContentID, location programsource.Span, decision, value identity.ContentID, points []identity.ContentID) identity.ContentID {
	return diagnosticObservationIdentity(
		programID,
		structure.DiagnosticObservationBranchCondition,
		location,
		branchConditionIdentityPayload{decision: decision, value: value, points: points},
		unresolvedTypeReferenceIdentityPayload{},
		unresolvedValueReferenceIdentityPayload{},
		typeConformanceIdentityPayload{},
	)
}

// TypeReferenceUnresolvedIdentity returns the canonical identity of one
// unresolved type-reference observation. A one-component path has no root;
// a qualified path requires an available root identity.
func TypeReferenceUnresolvedIdentity(programID identity.ContentID, location programsource.Span, reference, root identity.ContentID, path []string) identity.ContentID {
	return diagnosticObservationIdentity(
		programID,
		structure.DiagnosticObservationTypeReferenceUnresolved,
		location,
		branchConditionIdentityPayload{},
		unresolvedTypeReferenceIdentityPayload{reference: reference, root: root, path: path},
		unresolvedValueReferenceIdentityPayload{},
		typeConformanceIdentityPayload{},
	)
}

// ValueReferenceUnresolvedIdentity returns the canonical identity of one
// unresolved value-reference observation.
func ValueReferenceUnresolvedIdentity(programID identity.ContentID, location programsource.Span, read, cell identity.ContentID, name string) identity.ContentID {
	return diagnosticObservationIdentity(
		programID,
		structure.DiagnosticObservationValueReferenceUnresolved,
		location,
		branchConditionIdentityPayload{},
		unresolvedTypeReferenceIdentityPayload{},
		unresolvedValueReferenceIdentityPayload{read: read, cell: cell, name: name},
		typeConformanceIdentityPayload{},
	)
}

// TypeConformanceIdentity returns the canonical identity of one type
// conformance observation. The site ordinal is the canonical
// DiagnosticObservationSite vocabulary and is committed as its historical
// unsigned value.
func TypeConformanceIdentity(programID identity.ContentID, location programsource.Span, site DiagnosticObservationSite, owner, measured, declared, span identity.ContentID, position uint32, points []identity.ContentID) identity.ContentID {
	return diagnosticObservationIdentity(
		programID,
		structure.DiagnosticObservationTypeConformance,
		location,
		branchConditionIdentityPayload{},
		unresolvedTypeReferenceIdentityPayload{},
		unresolvedValueReferenceIdentityPayload{},
		typeConformanceIdentityPayload{site: site, owner: owner, measured: measured, declared: declared, span: span, position: position, points: points},
	)
}

func diagnosticObservationIdentity(
	programID identity.ContentID,
	kind structure.DiagnosticObservationKind,
	location programsource.Span,
	branch branchConditionIdentityPayload,
	unresolved unresolvedTypeReferenceIdentityPayload,
	value unresolvedValueReferenceIdentityPayload,
	conformance typeConformanceIdentityPayload,
) identity.ContentID {
	if !programID.Available() || !kind.Available() || !ValidSpan(location) {
		return identity.ContentID{}
	}
	hashValue := acquireDiagnosticHash()
	defer releaseDiagnosticHash(hashValue)
	var writer framing.Writer
	if writer.Reset(hashValue, "program/transformer/diagnostic-observation", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(programID[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.String(location.File) != nil ||
		writer.Uint(uint64(location.StartLine)) != nil || writer.Uint(uint64(location.StartCol)) != nil ||
		writer.Uint(uint64(location.EndLine)) != nil || writer.Uint(uint64(location.EndCol)) != nil {
		return identity.ContentID{}
	}
	switch kind {
	case structure.DiagnosticObservationBranchCondition:
		if !branch.available() || writer.Bytes(branch.decision[:]) != nil || writer.Bytes(branch.value[:]) != nil ||
			writer.Count(uint64(len(branch.points))) != nil || !writeEvidencePoints(&writer, branch.points) {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		if !unresolved.available() || writer.Bytes(unresolved.reference[:]) != nil || writer.Bytes(unresolved.root[:]) != nil ||
			writer.Count(uint64(len(unresolved.path))) != nil {
			return identity.ContentID{}
		}
		for _, component := range unresolved.path {
			if writer.String(component) != nil {
				return identity.ContentID{}
			}
		}
	case structure.DiagnosticObservationValueReferenceUnresolved:
		if !value.available() || writer.Bytes(value.read[:]) != nil || writer.Bytes(value.cell[:]) != nil || writer.String(value.name) != nil {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeConformance:
		if !conformance.available() || writer.Uint(uint64(conformance.site)) != nil ||
			writer.Bytes(conformance.owner[:]) != nil || writer.Bytes(conformance.measured[:]) != nil ||
			writer.Bytes(conformance.declared[:]) != nil || writer.Bytes(conformance.span[:]) != nil ||
			writer.Uint(uint64(conformance.position)) != nil || writer.Count(uint64(len(conformance.points))) != nil ||
			!writeEvidencePoints(&writer, conformance.points) {
			return identity.ContentID{}
		}
	default:
		return identity.ContentID{}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	if sum := hashValue.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func validEvidencePoints(points []identity.ContentID) bool {
	if len(points) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(points))
	for _, point := range points {
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

func writeEvidencePoints(writer *framing.Writer, points []identity.ContentID) bool {
	for _, point := range points {
		if !point.Available() || writer.Bytes(point[:]) != nil {
			return false
		}
	}
	return true
}
