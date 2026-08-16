package program

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// DiagnosticObservationKind is the closed Program-owned vocabulary of
// semantic observations that may produce a source diagnostic. An observation
// is proof, not diagnostic policy or rendered text.
type DiagnosticObservationKind uint8

const (
	DiagnosticObservationInvalid DiagnosticObservationKind = iota
	DiagnosticObservationBranchCondition
	DiagnosticObservationTypeReferenceUnresolved
	DiagnosticObservationValueReferenceUnresolved
)

func (kind DiagnosticObservationKind) valid() bool {
	return kind == DiagnosticObservationBranchCondition || kind == DiagnosticObservationTypeReferenceUnresolved || kind == DiagnosticObservationValueReferenceUnresolved
}

// DiagnosticBranchCondition is the typed payload of a branch observation.
// Both arms of one branch intentionally issue the same identity and evidence
// attachment set.
type DiagnosticBranchCondition struct {
	decision keyspace.ContentID
	value    keyspace.ContentID
	points   []keyspace.ContentID
}

func (payload DiagnosticBranchCondition) available() bool {
	return payload.decision.Available() && payload.value.Available() && validDiagnosticEvidencePoints(payload.points)
}

func (payload DiagnosticBranchCondition) empty() bool {
	return !payload.decision.Available() && !payload.value.Available() && len(payload.points) == 0
}

func (payload DiagnosticBranchCondition) DecisionPathID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.decision
}

// ValueSpanID is the exact Program span identity substituted to one mounted
// Boundary Value at Link compile time. It is not an authored Term.
func (payload DiagnosticBranchCondition) ValueSpanID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.value
}

func (payload DiagnosticBranchCondition) EvidencePointCount() int {
	if !payload.available() {
		return 0
	}
	return len(payload.points)
}

func (payload DiagnosticBranchCondition) EvidencePoints() ([]keyspace.ContentID, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]keyspace.ContentID(nil), payload.points...), true
}

func (payload DiagnosticBranchCondition) EvidencePointAt(index int) keyspace.ContentID {
	if !payload.available() || index < 0 || index >= len(payload.points) {
		return keyspace.ContentID{}
	}
	return payload.points[index]
}

// DiagnosticUnresolvedTypeReference is the typed payload of a static
// unresolved type-reference observation. The reference and optional root are
// owner-issued identities; path is the exact lexical spelling copied while
// Source is live. The observation kind itself is the detached unresolved
// binder proof, so no redundant resolution scalar is persisted.
type DiagnosticUnresolvedTypeReference struct {
	reference keyspace.ContentID
	root      keyspace.ContentID
	path      []string
}

func (payload DiagnosticUnresolvedTypeReference) available() bool {
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

func (payload DiagnosticUnresolvedTypeReference) empty() bool {
	return !payload.reference.Available() && !payload.root.Available() && len(payload.path) == 0
}

func (payload DiagnosticUnresolvedTypeReference) StaticReferenceID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.reference
}

func (payload DiagnosticUnresolvedTypeReference) RootID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.root
}

func (payload DiagnosticUnresolvedTypeReference) Path() ([]string, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]string(nil), payload.path...), true
}

// DiagnosticUnresolvedValueReference is the typed proof for one
// binder-classified implicit global Read. ReadID and CellID are distinct
// Program-issued identities; Name is copied from the exact Source atom while
// the Program owner is live. No runtime Value observation is required to prove
// that lexical binding failed.
type DiagnosticUnresolvedValueReference struct {
	read keyspace.ContentID
	cell keyspace.ContentID
	name string
}

func (payload DiagnosticUnresolvedValueReference) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload DiagnosticUnresolvedValueReference) empty() bool {
	return !payload.read.Available() && !payload.cell.Available() && payload.name == ""
}

func (payload DiagnosticUnresolvedValueReference) ReadID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.read
}

func (payload DiagnosticUnresolvedValueReference) CellID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.cell
}

func (payload DiagnosticUnresolvedValueReference) Name() (string, bool) {
	return payload.name, payload.available()
}

// DiagnosticObservation is one exact Program-owned semantic proof. Its
// payload is a closed tagged union: exactly one typed payload is populated for
// each valid kind. Program and Source are retained only for owner validation;
// ProgramArtifact copies the detached scalars and never reopens them.
type DiagnosticObservation struct {
	input           TransformerInput
	kind            DiagnosticObservationKind
	id              keyspace.ContentID
	location        source.Span
	branch          DiagnosticBranchCondition
	unresolved      DiagnosticUnresolvedTypeReference
	value           DiagnosticUnresolvedValueReference
	route           StructuralRoute
	guard           RouteGuard
	span            Span
	staticRef       programstatic.StaticTypeRef
	staticTerm      keyspace.Term
	staticRoot      keyspace.Term
	storageRead     StorageReadOccurrence
	storageCell     Cell
	implicitOrdinal uint32
}

// DiagnosticObservation returns the exact branch observation carried by a
// structural route. Unsupported route families do not fabricate observations.
func (route StructuralRoute) DiagnosticObservation() (DiagnosticObservation, bool) {
	observation, ok := diagnosticObservationForRoute(route)
	return observation, ok && observation.Available()
}

// DiagnosticObservationKind reports whether this route issues a supported
// branch observation. Static observations are issued from Static directly.
func (route StructuralRoute) DiagnosticObservationKind() DiagnosticObservationKind {
	if !route.Available() || !route.input.OwnsStructuralRoute(route) {
		return DiagnosticObservationInvalid
	}
	decision := route.identity.Decision()
	if keyspace.TermFamily(decision) != keyspace.FamilyBranch {
		return DiagnosticObservationInvalid
	}
	_, _, whenTrue, whenFalse, branchOK := route.input.owner.Flow().Authored().Control().Branches().Get(decision)
	if branchOK && diagnosticBranchScopeRewriteSafe(route.input, whenTrue, whenFalse) {
		return DiagnosticObservationBranchCondition
	}
	return DiagnosticObservationInvalid
}

func diagnosticObservationForRoute(route StructuralRoute) (DiagnosticObservation, bool) {
	if !route.Available() || !route.input.OwnsStructuralRoute(route) {
		return DiagnosticObservation{}, false
	}
	guard, guardOK := route.Guard()
	if !guardOK || !route.input.OwnsRouteGuard(guard) {
		return DiagnosticObservation{}, false
	}
	decisionTerm := route.identity.Decision()
	if keyspace.TermFamily(decisionTerm) != keyspace.FamilyBranch {
		return DiagnosticObservation{}, false
	}
	_, condition, whenTrue, whenFalse, branchOK := route.input.owner.Flow().Authored().Control().Branches().Get(decisionTerm)
	span, spanOK := route.input.Span(condition)
	location, locationOK := route.input.owner.Source().Identity().Span(condition)
	finish, finishOK := span.Finish()
	attachments := route.input.PointAttachments(finish)
	decision, decisionOK := guard.DecisionPathID()
	if !branchOK || !diagnosticBranchScopeRewriteSafe(route.input, whenTrue, whenFalse) ||
		!spanOK || !route.input.OwnsSpan(span) || !locationOK || location.File == "" ||
		!finishOK || !route.input.OwnsSite(finish) || !attachments.Available() || attachments.Count() == 0 || !decisionOK || !decision.Available() {
		return DiagnosticObservation{}, false
	}
	value := span.ContextID()
	if !value.Available() {
		return DiagnosticObservation{}, false
	}
	points := make([]keyspace.ContentID, attachments.Count())
	seenPoints := make(map[keyspace.ContentID]struct{}, len(points))
	for index := range points {
		attachment, attachmentOK := attachments.At(index)
		if !attachmentOK || !route.input.OwnsPointAttachment(attachment) {
			return DiagnosticObservation{}, false
		}
		points[index] = attachment.PointPathID()
		if !points[index].Available() {
			return DiagnosticObservation{}, false
		}
		if _, duplicate := seenPoints[points[index]]; duplicate {
			return DiagnosticObservation{}, false
		}
		seenPoints[points[index]] = struct{}{}
	}
	branch := DiagnosticBranchCondition{decision: decision, value: value, points: points}
	id := diagnosticObservationID(route.input.programID, DiagnosticObservationBranchCondition, location, branch, DiagnosticUnresolvedTypeReference{}, DiagnosticUnresolvedValueReference{})
	if !id.Available() {
		return DiagnosticObservation{}, false
	}
	return DiagnosticObservation{
		input: route.input, route: route, guard: guard, span: span, kind: DiagnosticObservationBranchCondition,
		id: id, location: location, branch: branch,
	}, true
}

// diagnosticBranchScopeRewriteSafe is Program's exact eligibility proof for
// the source rewrite named by the always-true/false advice. Moving either arm
// into its parent scope is not semantics-preserving when that arm directly
// introduces a local value, static type, interface, or label. Nested scopes
// keep their own owners and therefore do not poison an otherwise safe arm.
//
// The proof consumes only Program-owned sealed relations. Link and Analysis
// never reopen source or reinforce the eligibility decision.
func diagnosticBranchScopeRewriteSafe(input TransformerInput, whenTrue, whenFalse keyspace.Term) bool {
	if !input.Available() || keyspace.TermFamily(whenTrue) != keyspace.FamilyBody || keyspace.TermOrdinal(whenTrue) == 0 ||
		keyspace.TermFamily(whenFalse) != keyspace.FamilyBody || keyspace.TermOrdinal(whenFalse) == 0 || whenTrue == whenFalse {
		return false
	}
	arm := func(owner keyspace.Term) bool { return owner == whenTrue || owner == whenFalse }
	authored := input.owner.Flow().Authored()

	cells := authored.Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		kind, body, key, rowOK := cells.Get(term)
		if !termOK || !rowOK {
			return false
		}
		switch kind {
		case flow.CellLocal:
			if key != 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
				return false
			}
			if arm(body) {
				return false
			}
		case flow.CellGlobal:
			if body != 0 || key == 0 {
				return false
			}
		default:
			return false
		}
	}

	labels := authored.Control().Labels()
	for index := 0; index < labels.Count(); index++ {
		term, termOK := labels.At(index)
		owner, rowOK := labels.Get(term)
		if !termOK || !rowOK {
			return false
		}
		if arm(owner) {
			return false
		}
	}

	static := input.Static().Declarations()
	aliases := static.Aliases()
	for index := 0; index < aliases.Count(); index++ {
		term, termOK := aliases.At(index)
		owner, _, _, _, rowOK := aliases.Get(term)
		if !termOK || !rowOK {
			return false
		}
		if arm(owner) {
			return false
		}
	}
	interfaces := static.Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		term, termOK := interfaces.At(index)
		owner, _, _, rowOK := interfaces.Get(term)
		if !termOK || !rowOK {
			return false
		}
		if arm(owner) {
			return false
		}
	}
	return true
}

// TypeReferenceUnresolvedObservation issues one exact unresolved Static type
// reference. It is a Program/Static proof only; no Flow point or Engine
// observation is created for this kind.
func (input TransformerInput) TypeReferenceUnresolvedObservation(term keyspace.Term) (DiagnosticObservation, bool) {
	if !input.Available() || keyspace.TermFamily(term) != keyspace.FamilyTypeRef {
		return DiagnosticObservation{}, false
	}
	static := input.Static()
	ref, refOK := static.StaticTypes().Ref(term)
	resolution, target, rootTerm, referenceOK := static.References().Get(term)
	location, locationOK := input.owner.Source().Identity().Span(term)
	if !refOK || !referenceOK || resolution != programstatic.TypeRefUnresolved || target != 0 || !locationOK || location.File == "" {
		return DiagnosticObservation{}, false
	}
	count, countOK := static.References().SourceCount(term)
	if !countOK || count == 0 {
		return DiagnosticObservation{}, false
	}
	path := make([]string, count)
	for index := range path {
		key, keyOK := static.References().SourceAt(term, index)
		component, componentOK := input.StaticKeyText(key)
		if !keyOK || !componentOK || component == "" {
			return DiagnosticObservation{}, false
		}
		path[index] = component
	}
	root := keyspace.ContentID{}
	if len(path) == 1 {
		if rootTerm != 0 {
			return DiagnosticObservation{}, false
		}
	} else {
		if rootTerm == 0 || keyspace.TermFamily(rootTerm) != keyspace.FamilyCell {
			return DiagnosticObservation{}, false
		}
		var rootOK bool
		root, rootOK = StaticScopeID(input.programID, rootTerm)
		if !rootOK {
			return DiagnosticObservation{}, false
		}
	}
	reference, referenceIDOK := StaticTypeReferenceID(input.programID, ref)
	if !referenceIDOK {
		return DiagnosticObservation{}, false
	}
	unresolved := DiagnosticUnresolvedTypeReference{reference: reference, root: root, path: path}
	id := diagnosticObservationID(input.programID, DiagnosticObservationTypeReferenceUnresolved, location, DiagnosticBranchCondition{}, unresolved, DiagnosticUnresolvedValueReference{})
	if !id.Available() {
		return DiagnosticObservation{}, false
	}
	return DiagnosticObservation{
		input: input, kind: DiagnosticObservationTypeReferenceUnresolved, id: id, location: location,
		unresolved: unresolved, staticRef: ref, staticTerm: term, staticRoot: rootTerm,
	}, true
}

// ValueReferenceUnresolvedObservationCount is the exact binder-issued sparse
// denominator of implicit global Reads. It is not the full Read denominator
// and therefore does not infer missing bindings from names or Link state.
func (input TransformerInput) ValueReferenceUnresolvedObservationCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Storage().Reads().ImplicitCount()
}

// ValueReferenceUnresolvedObservationAt issues one Program-owned static
// absence proof. The implicit Read index, exact executable read, global Cell,
// Source atom, and source span must all agree; consumers receive only detached
// semantic identities and the copied name.
func (input TransformerInput) ValueReferenceUnresolvedObservationAt(index int) (DiagnosticObservation, bool) {
	if !input.Available() || index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return DiagnosticObservation{}, false
	}
	storage := input.owner.Flow().Authored().Storage()
	term, termOK := storage.Reads().ImplicitAt(index)
	owner, sourceTerm, implicit, relationOK := storage.Reads().Get(term)
	ordinal := keyspace.TermOrdinal(term)
	if !termOK || !relationOK || !implicit || owner == 0 || ordinal == 0 {
		return DiagnosticObservation{}, false
	}
	read, readOK := input.StorageReadAt(int(ordinal - 1))
	cell, cellOK := read.Cell()
	kind, body, key, cellRelationOK := storage.Cells().Get(sourceTerm)
	literal, literalOK := input.owner.Source().Keys().Exact(key)
	location, locationOK := input.owner.Source().Identity().Span(term)
	if !readOK || !input.OwnsStorageReadOccurrence(read) || !cellOK || !input.OwnsCell(cell) ||
		!cellRelationOK || kind != flow.CellGlobal || body != 0 || key == 0 ||
		!literalOK || literal.Kind != keyspace.LiteralString || literal.String == "" ||
		!locationOK || location.File == "" {
		return DiagnosticObservation{}, false
	}
	payload := DiagnosticUnresolvedValueReference{read: read.ContextID(), cell: cell.ContextID(), name: literal.String}
	id := diagnosticObservationID(input.programID, DiagnosticObservationValueReferenceUnresolved, location, DiagnosticBranchCondition{}, DiagnosticUnresolvedTypeReference{}, payload)
	if !payload.available() || !id.Available() {
		return DiagnosticObservation{}, false
	}
	return DiagnosticObservation{
		input: input, kind: DiagnosticObservationValueReferenceUnresolved, id: id, location: location,
		value: payload, storageRead: read, storageCell: cell, implicitOrdinal: uint32(index + 1),
	}, true
}

func (observation DiagnosticObservation) Available() bool {
	if !observation.input.Available() || !observation.kind.valid() || !observation.id.Available() || observation.location.File == "" {
		return false
	}
	switch observation.kind {
	case DiagnosticObservationBranchCondition:
		if !observation.branch.available() || !observation.unresolved.empty() || !observation.value.empty() || !observation.valueProofEmpty() ||
			!observation.input.OwnsStructuralRoute(observation.route) || !observation.input.OwnsRouteGuard(observation.guard) ||
			!observation.input.OwnsSpan(observation.span) {
			return false
		}
		want, ok := diagnosticObservationForRoute(observation.route)
		return ok && want.id == observation.id && want.location == observation.location &&
			want.branch.decision == observation.branch.decision && want.branch.value == observation.branch.value &&
			equalDiagnosticEvidencePoints(want.branch.points, observation.branch.points)
	case DiagnosticObservationTypeReferenceUnresolved:
		if !observation.unresolved.available() || !observation.branch.empty() || !observation.value.empty() || !observation.valueProofEmpty() || observation.staticTerm == 0 ||
			!observation.input.Static().StaticTypes().Owns(observation.staticRef) || observation.staticRef.Term() != observation.staticTerm {
			return false
		}
		want, ok := observation.input.TypeReferenceUnresolvedObservation(observation.staticTerm)
		return ok && want.id == observation.id && want.location == observation.location &&
			want.unresolved.reference == observation.unresolved.reference && want.unresolved.root == observation.unresolved.root &&
			want.staticRoot == observation.staticRoot && equalDiagnosticStringPath(want.unresolved.path, observation.unresolved.path)
	case DiagnosticObservationValueReferenceUnresolved:
		if !observation.value.available() || !observation.branch.empty() || !observation.unresolved.empty() || observation.implicitOrdinal == 0 ||
			!observation.input.OwnsStorageReadOccurrence(observation.storageRead) || !observation.input.OwnsCell(observation.storageCell) {
			return false
		}
		want, ok := observation.input.ValueReferenceUnresolvedObservationAt(int(observation.implicitOrdinal - 1))
		return ok && want.id == observation.id && want.location == observation.location && want.implicitOrdinal == observation.implicitOrdinal &&
			want.value == observation.value && want.storageRead.ContextID() == observation.storageRead.ContextID() &&
			want.storageCell.ContextID() == observation.storageCell.ContextID()
	default:
		return false
	}
}

func (observation DiagnosticObservation) valueProofEmpty() bool {
	return observation.implicitOrdinal == 0 && !observation.storageRead.Available() && !observation.storageCell.Available()
}

func (observation DiagnosticObservation) Kind() DiagnosticObservationKind {
	if !observation.Available() {
		return DiagnosticObservationInvalid
	}
	return observation.kind
}

func (observation DiagnosticObservation) ID() keyspace.ContentID {
	if !observation.Available() {
		return keyspace.ContentID{}
	}
	return observation.id
}

func (observation DiagnosticObservation) Location() (source.Span, bool) {
	if !observation.Available() {
		return source.Span{}, false
	}
	return observation.location, true
}

func (observation DiagnosticObservation) BranchCondition() (DiagnosticBranchCondition, bool) {
	if !observation.Available() || observation.kind != DiagnosticObservationBranchCondition {
		return DiagnosticBranchCondition{}, false
	}
	return DiagnosticBranchCondition{decision: observation.branch.decision, value: observation.branch.value, points: append([]keyspace.ContentID(nil), observation.branch.points...)}, true
}

func (observation DiagnosticObservation) UnresolvedTypeReference() (DiagnosticUnresolvedTypeReference, bool) {
	if !observation.Available() || observation.kind != DiagnosticObservationTypeReferenceUnresolved {
		return DiagnosticUnresolvedTypeReference{}, false
	}
	return DiagnosticUnresolvedTypeReference{reference: observation.unresolved.reference, root: observation.unresolved.root, path: append([]string(nil), observation.unresolved.path...)}, true
}

func (observation DiagnosticObservation) UnresolvedValueReference() (DiagnosticUnresolvedValueReference, bool) {
	if !observation.Available() || observation.kind != DiagnosticObservationValueReferenceUnresolved {
		return DiagnosticUnresolvedValueReference{}, false
	}
	return observation.value, true
}

func (input TransformerInput) OwnsDiagnosticObservation(observation DiagnosticObservation) bool {
	return input.Available() && observation.input == input && observation.Available()
}

func diagnosticObservationID(owner keyspace.ContentID, kind DiagnosticObservationKind, location source.Span, branch DiagnosticBranchCondition, unresolved DiagnosticUnresolvedTypeReference, value DiagnosticUnresolvedValueReference) keyspace.ContentID {
	if !owner.Available() || !kind.valid() || location.File == "" {
		return keyspace.ContentID{}
	}
	id := transformerRoleID("program/transformer/diagnostic-observation", owner, func(writer *canonical.Writer) bool {
		if writer.Uint(uint64(kind)) != nil || writer.String(location.File) != nil || writer.Uint(uint64(location.StartLine)) != nil ||
			writer.Uint(uint64(location.StartCol)) != nil || writer.Uint(uint64(location.EndLine)) != nil || writer.Uint(uint64(location.EndCol)) != nil {
			return false
		}
		switch kind {
		case DiagnosticObservationBranchCondition:
			return branch.available() && writer.Bytes(branch.decision[:]) == nil && writer.Bytes(branch.value[:]) == nil &&
				writer.Count(uint64(len(branch.points))) == nil && writeDiagnosticEvidencePoints(writer, branch.points)
		case DiagnosticObservationTypeReferenceUnresolved:
			if !unresolved.available() || writer.Bytes(unresolved.reference[:]) != nil || writer.Bytes(unresolved.root[:]) != nil ||
				writer.Count(uint64(len(unresolved.path))) != nil {
				return false
			}
			for _, component := range unresolved.path {
				if writer.String(component) != nil {
					return false
				}
			}
			return true
		case DiagnosticObservationValueReferenceUnresolved:
			return value.available() && writer.Bytes(value.read[:]) == nil && writer.Bytes(value.cell[:]) == nil && writer.String(value.name) == nil
		default:
			return false
		}
	})
	return id
}

func writeDiagnosticEvidencePoints(writer *canonical.Writer, points []keyspace.ContentID) bool {
	for _, point := range points {
		if !point.Available() || writer.Bytes(point[:]) != nil {
			return false
		}
	}
	return true
}

func equalDiagnosticEvidencePoints(left, right []keyspace.ContentID) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Available() || left[index] != right[index] {
			return false
		}
	}
	return true
}

func validDiagnosticEvidencePoints(points []keyspace.ContentID) bool {
	if len(points) == 0 {
		return false
	}
	seen := make(map[keyspace.ContentID]struct{}, len(points))
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

func equalDiagnosticStringPath(left, right []string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] == "" || left[index] != right[index] {
			return false
		}
	}
	return true
}
