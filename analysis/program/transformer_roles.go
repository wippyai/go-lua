package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

// transformerRoleID is the sole small Program-local identity codec for
// transient transformer proofs. Every role is fenced by the published Program
// identity; it intentionally commits neither Link identity nor a physical
// Flow row/ordinal.
func transformerRoleID(domain string, programID identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	if !programID.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(programID[:]) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var result identity.ContentID
	if sum := hash.Sum(result[:0]); len(sum) != len(result) {
		return identity.ContentID{}
	}
	return result
}

// transformerSemanticID is the owner-neutral companion used for descriptors
// whose parent Flow receipt is already content-stable. It deliberately omits
// ProgramID; exact hot ownership remains carried by the enclosing proof.
func transformerSemanticID(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var result identity.ContentID
	if sum := hash.Sum(result[:0]); len(sum) != len(result) {
		return identity.ContentID{}
	}
	return result
}

// ContextID is a Program-fenced identity for one existing authored Span. The
// authored coordinate stays private; equivalent replay has the same ID while
// OwnsSpan remains an exact hot-owner predicate.
func (span Span) ContextID() identity.ContentID {
	if !span.Available() {
		return identity.ContentID{}
	}
	return span.context
}

func spanContextID(span Span) identity.ContentID {
	if !span.availableGeometry() {
		return identity.ContentID{}
	}
	entryID, finishID := span.entry.ContextID(), span.finish.ContextID()
	return transformerRoleID("program/transformer/span", span.input.programID, func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(span.authored))) == nil &&
			writer.Uint(uint64(keyspace.TermOrdinal(span.authored))) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
}

func (input TransformerInput) OwnsSpan(span Span) bool {
	if !input.Available() || span.input != input || !span.Available() {
		return false
	}
	entry, entryOK := span.Entry()
	finish, finishOK := span.Finish()
	return entryOK && finishOK && input.OwnsSite(entry) && input.OwnsSite(finish)
}

// Guard is an opaque continuation Guard proof. The shared decision identity
// is independent of its admitted subject, while the proof itself remains
// exact-owner fenced by the subject Site and TransformerInput.
type Guard struct {
	input    TransformerInput
	subject  flow.Site
	decision flow.Site
	term     keyspace.Term
	path     identity.ContentID
	index    int
}

func (input TransformerInput) GuardAt(site flow.Site, index int) (Guard, bool) {
	term, ok := input.ownedSite(site)
	if !ok || index < 0 {
		return Guard{}, false
	}
	guard, guardOK := input.owner.Flow().Continuation().GuardAt(term, index)
	path, pathOK := input.owner.Flow().SemanticTermPath(guard)
	decision, _ := input.owner.Flow().Causal().Sites().ForTerm(guard)
	result := Guard{input: input, subject: site, decision: decision, term: guard, path: path, index: index}
	return result, guardOK && pathOK && result.Available()
}

func (guard Guard) Available() bool {
	if !guard.input.Available() || !guard.input.OwnsSite(guard.subject) || guard.term == 0 || !guard.path.Available() || guard.index < 0 {
		return false
	}
	subject, ok := guard.input.ownedSite(guard.subject)
	if !ok {
		return false
	}
	candidate, ok := guard.input.owner.Flow().Continuation().GuardAt(subject, guard.index)
	path, pathOK := guard.input.owner.Flow().SemanticTermPath(candidate)
	if !ok || candidate != guard.term || !pathOK || path != guard.path {
		return false
	}
	if guard.decision.Available() {
		decisionTerm, decisionOK := guard.decision.Term()
		return guard.input.OwnsSite(guard.decision) && decisionOK && decisionTerm == guard.term && guard.decision.PathID() == guard.path
	}
	return true
}

func (guard Guard) ContextID() identity.ContentID {
	if !guard.Available() {
		return identity.ContentID{}
	}
	return transformerRoleID("program/transformer/guard", guard.input.programID, func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(guard.term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(guard.term))) == nil
	})
}

// PathID is the portable semantic identity of the exact decision Site. It is
// the same identity carried by recurrence reset-member receipts.
func (guard Guard) PathID() identity.ContentID {
	if !guard.Available() {
		return identity.ContentID{}
	}
	return guard.path
}

func (input TransformerInput) OwnsGuard(guard Guard) bool {
	return input.Available() && guard.input == input && input.OwnsSite(guard.subject) && guard.Available()
}

type cellRole uint8

const (
	cellRoleInvalid cellRole = iota
	cellRoleContinuation
	cellRoleFormal
	cellRoleVararg
	cellRoleCaptureInner
	cellRoleCaptureOuter
	cellRoleStorage
)

// Cell is an opaque existing lexical Cell proof. Its role and parent proof
// make a raw Cell term insufficient to substitute a foreign or replay handle.
type Cell struct {
	input    TransformerInput
	role     cellRole
	term     keyspace.Term
	subject  flow.Site
	function Function
	body     Body
	index    int
}

func (input TransformerInput) CellAt(site flow.Site, index int) (Cell, bool) {
	subject, ok := input.ownedSite(site)
	if !ok || index < 0 {
		return Cell{}, false
	}
	term, termOK := input.owner.Flow().Continuation().CellAt(subject, index)
	body, bodyOK := input.ContainingBody(term)
	cell := Cell{input: input, role: cellRoleContinuation, term: term, subject: site, body: body, index: index}
	return cell, termOK && bodyOK && cell.Available()
}

func (cell Cell) Available() bool {
	if !cell.input.Available() || cell.term == 0 || cell.index < 0 {
		return false
	}
	// Function formals are boundary-owned Cells. They are not required to
	// have a direct Source position (the body boundary is their authoritative
	// containment), so fence them through the exact transformer Function/body
	// pair instead of reconstructing containment from the source index.
	if cell.role == cellRoleFormal || cell.role == cellRoleVararg || cell.role == cellRoleCaptureInner {
		if !cell.function.Available() || cell.function.body != cell.body || !cell.input.OwnsBody(cell.body) {
			return false
		}
	} else if cell.role == cellRoleCaptureOuter {
		// The outer side is deliberately owned by a different lexical Body.
		// The role-specific switch below reauthenticates that Body against the
		// exact capture pair issued by this Function boundary.
		if !cell.function.Available() || !cell.input.OwnsBody(cell.body) || cell.function.body.Equal(cell.body) {
			return false
		}
	} else if cell.role != cellRoleStorage {
		issuedBody, bodyOK := cell.input.ContainingBody(cell.term)
		if !bodyOK || !cell.input.OwnsBody(cell.body) || !cell.body.Equal(issuedBody) {
			return false
		}
	}
	switch cell.role {
	case cellRoleContinuation:
		subject, ok := cell.input.ownedSite(cell.subject)
		if !ok {
			return false
		}
		term, ok := cell.input.owner.Flow().Continuation().CellAt(subject, cell.index)
		return ok && term == cell.term
	case cellRoleFormal:
		term, ok := cell.function.boundary.FormalAt(cell.index)
		return cell.function.Available() && ok && term == cell.term
	case cellRoleVararg:
		term, ok := cell.function.boundary.Vararg()
		return cell.function.Available() && cell.index == 0 && ok && term == cell.term
	case cellRoleCaptureInner, cellRoleCaptureOuter:
		capture, ok := cell.function.boundary.CaptureAt(cell.index)
		if !cell.function.Available() || !ok {
			return false
		}
		if cell.role == cellRoleCaptureInner {
			body, bodyOK := cell.body.boundary.Body()
			return bodyOK && body == capture.InnerBody && capture.Inner == cell.term
		}
		body, bodyOK := cell.body.boundary.Body()
		return bodyOK && body == capture.OuterBody && capture.Outer == cell.term
	case cellRoleStorage:
		_, _, _, ok := cell.input.owner.Flow().Authored().Storage().Cells().Get(cell.term)
		return ok && cell.subject == (flow.Site{}) && !cell.body.Available() && !cell.function.Available() && cell.index == 0
	default:
		return false
	}
}

func (cell Cell) ContextID() identity.ContentID {
	if !cell.Available() {
		return identity.ContentID{}
	}
	if cell.role == cellRoleStorage {
		return transformerRoleID("program/transformer/storage-cell", cell.input.programID, func(writer *framing.Writer) bool {
			return writeTransformerTerm(writer, cell.term)
		})
	}
	if !cell.body.Available() {
		return identity.ContentID{}
	}
	bodyID := cell.body.ContextID()
	if cell.role == cellRoleFormal || cell.role == cellRoleVararg || cell.role == cellRoleCaptureInner || cell.role == cellRoleCaptureOuter {
		pathID := cell.body.PathID()
		return transformerSemanticID("program/transformer/cell-semantic", func(writer *framing.Writer) bool {
			return writer.Bytes(pathID[:]) == nil && writer.Uint(uint64(cell.role)) == nil && writer.Uint(uint64(cell.index)) == nil
		})
	}
	return transformerRoleID("program/transformer/cell", cell.input.programID, func(writer *framing.Writer) bool {
		return writer.Bytes(bodyID[:]) == nil && writer.Uint(uint64(keyspace.TermFamily(cell.term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(cell.term))) == nil
	})
}

func (input TransformerInput) OwnsCell(cell Cell) bool {
	if !input.Available() || cell.input != input || !cell.Available() {
		return false
	}
	switch cell.role {
	case cellRoleContinuation:
		return input.OwnsBody(cell.body) && input.OwnsSite(cell.subject)
	case cellRoleFormal, cellRoleVararg, cellRoleCaptureInner, cellRoleCaptureOuter:
		return input.OwnsBody(cell.body) && input.OwnsFunction(cell.function)
	case cellRoleStorage:
		return !cell.body.Available() && !cell.function.Available()
	default:
		return false
	}
}

// storageCell is the private Program-issued projection for one already-sealed
// storage Cell. The raw coordinate stays inside the proof; consumers receive
// only the exact Cell receipt and ContextID.
func (input TransformerInput) storageCell(term keyspace.Term) (Cell, bool) {
	if !input.Available() || term == 0 {
		return Cell{}, false
	}
	cell := Cell{input: input, role: cellRoleStorage, term: term}
	return cell, cell.Available() && input.OwnsCell(cell)
}

// StorageCellCount and StorageCellAt expose the exact sealed storage-cell
// denominator as opaque Cell proofs.  They are the construction-time bridge
// for artifact and Link receipt compilers; neither method leaks the authored
// Cell term.
func (input TransformerInput) StorageCellCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Storage().Cells().Count()
}

func (input TransformerInput) StorageCellAt(index int) (Cell, bool) {
	if !input.Available() || index < 0 {
		return Cell{}, false
	}
	term, ok := input.owner.Flow().Authored().Storage().Cells().At(index)
	if !ok {
		return Cell{}, false
	}
	cell, cellOK := input.storageCell(term)
	return cell, cellOK && input.OwnsCell(cell)
}

func (input TransformerInput) OwnsFunction(function Function) bool {
	return input.Available() && function.body.input == input && input.OwnsBody(function.body) &&
		input.owner.Flow().FunctionBoundaries().OwnsFunction(function.boundary) && function.Available()
}

// Function is the transformer-owned wrapper for a FunctionBoundary. It
// exposes only Cell proofs and stable role IDs, not Function/Body coordinates.
type Function struct {
	body     Body
	boundary flow.FunctionBoundary
}

func (body Body) TransformerFunction() (Function, bool) {
	if !body.Available() || !body.function.Available() {
		return Function{}, false
	}
	function := Function{body: body, boundary: body.function}
	return function, function.Available()
}

func (function Function) Available() bool {
	return function.body.Available() && function.boundary.Available() && function.body.function.Equal(function.boundary)
}

func (function Function) Body() (Body, bool) { return function.body, function.Available() }

func (function Function) ContextID() identity.ContentID {
	if !function.Available() {
		return identity.ContentID{}
	}
	context := function.boundary.ContextID()
	return transformerRoleID("program/transformer/function", function.body.input.programID, func(writer *framing.Writer) bool {
		return writer.Bytes(context[:]) == nil
	})
}

func (function Function) FormalCount() int {
	if !function.Available() {
		return 0
	}
	return function.boundary.FormalCount()
}

func (function Function) FormalAt(index int) (Formal, bool) {
	term, ok := function.boundary.FormalAt(index)
	cell := Cell{input: function.body.input, role: cellRoleFormal, term: term, function: function, body: function.body, index: index}
	formal := Formal{function: function, cell: cell}
	return formal, ok && formal.Available()
}

func (function Function) Vararg() (Vararg, bool) {
	term, ok := function.boundary.Vararg()
	cell := Cell{input: function.body.input, role: cellRoleVararg, term: term, function: function, body: function.body}
	vararg := Vararg{function: function, cell: cell}
	return vararg, ok && vararg.Available()
}

func (function Function) CaptureCount() int {
	if !function.Available() {
		return 0
	}
	return function.boundary.CaptureCount()
}

func (function Function) CaptureAt(index int) (Capture, bool) {
	pair, ok := function.boundary.CaptureAt(index)
	innerBody, innerBodyOK := function.body.input.Body(pair.InnerBody)
	outerBody, outerBodyOK := function.body.input.Body(pair.OuterBody)
	inner := Cell{input: function.body.input, role: cellRoleCaptureInner, term: pair.Inner, function: function, body: innerBody, index: index}
	outer := Cell{input: function.body.input, role: cellRoleCaptureOuter, term: pair.Outer, function: function, body: outerBody, index: index}
	capture := Capture{function: function, inner: inner, outer: outer, index: index}
	return capture, ok && innerBodyOK && outerBodyOK && innerBody.Equal(function.body) && !outerBody.Equal(innerBody) && capture.Available()
}

type Formal struct {
	function Function
	cell     Cell
}

func (formal Formal) Available() bool {
	return formal.function.Available() && formal.cell.role == cellRoleFormal && formal.cell.function.Available() &&
		formal.cell.function == formal.function && formal.function.body == formal.cell.function.body && formal.cell.body == formal.function.body && formal.cell.Available()
}
func (formal Formal) Cell() (Cell, bool) {
	return formal.cell, formal.Available()
}

// StorageCell is the exact storage-role projection of this formal's lexical
// Cell. Formal and storage ContextIDs intentionally differ; callers that need
// a Boundary Value must use this parent-issued bridge rather than equating the
// two semantic namespaces.
func (formal Formal) StorageCell() (Cell, bool) {
	if !formal.Available() {
		return Cell{}, false
	}
	cell, ok := formal.function.body.input.storageCell(formal.cell.term)
	return cell, ok && formal.function.body.input.OwnsCell(cell)
}
func (formal Formal) Position() (int, bool) {
	if !formal.Available() {
		return 0, false
	}
	return formal.cell.index, true
}

// DeclaredStaticTypeReferenceID returns the exact Program-owned static type
// attached to this formal Cell. A false result means the formal is genuinely
// unannotated; callers must not infer a type from its name or position.
func (formal Formal) DeclaredStaticTypeReferenceID() (identity.ContentID, bool) {
	if !formal.Available() {
		return identity.ContentID{}, false
	}
	static := formal.function.body.input.owner.Static()
	declaration, declarationOK := static.Declarations().DeclaredTypes().ForCell(formal.cell.term)
	cell, target, rowOK := static.Declarations().DeclaredTypes().Get(declaration)
	ref, refOK := static.StaticTypes().Ref(target)
	id, idOK := StaticTypeReferenceID(formal.function.body.input.programID, ref)
	if !declarationOK || !rowOK || cell != formal.cell.term || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, false
	}
	return id, true
}
func (input TransformerInput) OwnsFormal(formal Formal) bool {
	return input.OwnsFunction(formal.function) && formal.cell.input == input && input.OwnsCell(formal.cell) && formal.Available()
}
func (formal Formal) ContextID() identity.ContentID {
	if !formal.Available() {
		return identity.ContentID{}
	}
	bodyPath, cellID := formal.function.body.PathID(), formal.cell.ContextID()
	return transformerSemanticID("program/transformer/formal", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Uint(uint64(formal.cell.index)) == nil && writer.Bytes(cellID[:]) == nil
	})
}

type Vararg struct {
	function Function
	cell     Cell
}

func (vararg Vararg) Available() bool {
	return vararg.function.Available() && vararg.cell.role == cellRoleVararg && vararg.cell.function.Available() &&
		vararg.cell.function == vararg.function && vararg.function.body == vararg.cell.function.body && vararg.cell.body == vararg.function.body && vararg.cell.Available()
}
func (vararg Vararg) Cell() (Cell, bool) {
	return vararg.cell, vararg.Available()
}
func (input TransformerInput) OwnsVararg(vararg Vararg) bool {
	return input.OwnsFunction(vararg.function) && vararg.cell.input == input && input.OwnsCell(vararg.cell) && vararg.Available()
}
func (vararg Vararg) ContextID() identity.ContentID {
	if !vararg.Available() {
		return identity.ContentID{}
	}
	bodyPath, cellID := vararg.function.body.PathID(), vararg.cell.ContextID()
	return transformerSemanticID("program/transformer/vararg", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(cellID[:]) == nil
	})
}

type Capture struct {
	function     Function
	inner, outer Cell
	index        int
}

func (capture Capture) Available() bool {
	return capture.function.Available() && capture.index >= 0 && capture.inner.role == cellRoleCaptureInner && capture.outer.role == cellRoleCaptureOuter &&
		capture.inner.function.Available() && capture.outer.function.Available() &&
		capture.inner.function == capture.function && capture.outer.function == capture.function &&
		capture.inner.body == capture.function.body && capture.index == capture.inner.index && capture.index == capture.outer.index && capture.inner.Available() && capture.outer.Available()
}
func (capture Capture) Inner() (Cell, bool) { return capture.inner, capture.Available() }
func (capture Capture) Outer() (Cell, bool) { return capture.outer, capture.Available() }
func (capture Capture) Position() (int, bool) {
	if !capture.Available() {
		return 0, false
	}
	return capture.index, true
}
func (input TransformerInput) OwnsCapture(capture Capture) bool {
	return input.OwnsFunction(capture.function) && capture.inner.input == input && capture.outer.input == input && input.OwnsCell(capture.inner) && input.OwnsCell(capture.outer) && capture.Available()
}
func (capture Capture) ContextID() identity.ContentID {
	if !capture.Available() {
		return identity.ContentID{}
	}
	innerPath, outerPath := capture.inner.body.PathID(), capture.outer.body.PathID()
	innerID, outerID := capture.inner.ContextID(), capture.outer.ContextID()
	return transformerSemanticID("program/transformer/capture", func(writer *framing.Writer) bool {
		return writer.Bytes(innerPath[:]) == nil && writer.Bytes(outerPath[:]) == nil && writer.Uint(uint64(capture.index)) == nil && writer.Bytes(innerID[:]) == nil && writer.Bytes(outerID[:]) == nil
	})
}

// InnerBodyPathID and OuterBodyPathID expose the portable Program-local Body
// identities joined by this exact capture. They deliberately do not expose
// authored Body or Cell coordinates: ProgramArtifact needs only this closed
// boundary relation when it freezes a reusable transformer interface.
func (capture Capture) InnerBodyPathID() identity.ContentID {
	if !capture.Available() {
		return identity.ContentID{}
	}
	return capture.inner.body.PathID()
}

func (capture Capture) OuterBodyPathID() identity.ContentID {
	if !capture.Available() {
		return identity.ContentID{}
	}
	return capture.outer.body.PathID()
}

// CallBoundary and CallArm wrap the already-final Flow causal boundary and
// successor proofs. They retain no arm table; enumeration borrows the sole
// Causal successor denominator in its published order.
type CallBoundary struct {
	span      Span
	boundary  flow.CallBoundary
	arms      [7]callArmProof
	armCount  int
	contextID identity.ContentID
	validated bool
}

type callArmProof struct {
	successor   flow.Successor
	target      flow.Site
	routeDigest identity.ContentID
	targetID    identity.ContentID
	contextID   identity.ContentID
	validated   bool
}

func (input TransformerInput) CallBoundary(span Span) (CallBoundary, bool) {
	if !input.OwnsSpan(span) {
		return CallBoundary{}, false
	}
	boundary, ok := input.owner.Flow().Causal().Boundaries().For(span.authored)
	result := CallBoundary{span: span, boundary: boundary}
	if !ok || !result.validBasePayload() {
		return CallBoundary{}, false
	}
	for _, kind := range transformerBoundaryArmOrder {
		proof, proofOK := result.issueArm(kind)
		if !proofOK {
			continue
		}
		result.arms[result.armCount] = proof
		result.armCount++
	}
	if result.armCount == 0 {
		return CallBoundary{}, false
	}
	spanID := result.span.ContextID()
	result.contextID = transformerRoleID("program/transformer/call-boundary", result.span.input.programID, func(writer *framing.Writer) bool {
		if writer.Bytes(spanID[:]) != nil || writer.Count(uint64(result.armCount)) != nil {
			return false
		}
		for index := 0; index < result.armCount; index++ {
			proof := result.arms[index]
			if writer.Uint(uint64(proof.successor.Arm)) != nil || writer.Bytes(proof.routeDigest[:]) != nil || writer.Bytes(proof.targetID[:]) != nil {
				return false
			}
		}
		return true
	})
	if !result.contextID.Available() {
		return CallBoundary{}, false
	}
	for index := 0; index < result.armCount; index++ {
		proof := &result.arms[index]
		proof.contextID = transformerRoleID("program/transformer/call-arm", result.span.input.programID, func(writer *framing.Writer) bool {
			return writer.Bytes(result.contextID[:]) == nil && writer.Uint(uint64(proof.successor.Arm)) == nil && writer.Bytes(proof.routeDigest[:]) == nil && writer.Bytes(proof.targetID[:]) == nil
		})
		if !proof.contextID.Available() {
			return CallBoundary{}, false
		}
	}
	result.validated = true
	return result, true
}

func (boundary CallBoundary) Available() bool {
	return boundary.validated && boundary.span.input.Available() && boundary.armCount > 0 && boundary.armCount <= len(boundary.arms) && boundary.contextID.Available()
}

func (boundary CallBoundary) validBasePayload() bool {
	if !boundary.span.Available() || boundary.boundary.Call != boundary.span.authored {
		return false
	}
	issued, ok := boundary.span.input.owner.Flow().Causal().Boundaries().For(boundary.span.authored)
	return ok && issued == boundary.boundary
}

func (boundary CallBoundary) closedArmCount() int {
	if !boundary.Available() {
		return 0
	}
	return boundary.armCount
}
func (boundary CallBoundary) ContextID() identity.ContentID {
	if !boundary.Available() {
		return identity.ContentID{}
	}
	return boundary.contextID
}
func (boundary CallBoundary) Span() (Span, bool) { return boundary.span, boundary.Available() }
func (input TransformerInput) OwnsCallBoundary(boundary CallBoundary) bool {
	return input.Available() && boundary.span.input == input && input.OwnsSpan(boundary.span) && boundary.Available()
}
func (boundary CallBoundary) ArmCount() int {
	if !boundary.Available() {
		return 0
	}
	return boundary.closedArmCount()
}
func (boundary CallBoundary) ArmAt(index int) (CallArm, bool) {
	if !boundary.Available() || index < 0 || index >= boundary.armCount {
		return CallArm{}, false
	}
	arm := CallArm{boundary: boundary, proof: boundary.arms[index]}
	return arm, arm.Available()
}

var transformerBoundaryArmOrder = [...]flow.BoundaryArmKind{
	flow.BoundaryResume, flow.BoundarySelectTrue, flow.BoundarySelectFalse,
	flow.BoundaryTail, flow.BoundaryThrow, flow.BoundaryYield, flow.BoundaryCancel,
}

func (boundary CallBoundary) issueArm(want flow.BoundaryArmKind) (callArmProof, bool) {
	if !boundary.validBasePayload() {
		return callArmProof{}, false
	}
	successor, ok := boundary.span.input.owner.Flow().Causal().Boundaries().Arm(boundary.span.authored, want)
	if !ok {
		return callArmProof{}, false
	}
	identity, identityOK := successor.Identity()
	target, targetOK := boundary.span.input.owner.Flow().Causal().Sites().ForTerm(successor.To)
	targetID := target.ContextID()
	proof := callArmProof{
		successor: successor, target: target, routeDigest: identity.Digest(), targetID: targetID, validated: true,
	}
	return proof, identityOK && identity.Available() && identity.Provenance() == boundary.span.input.owner.Flow().Provenance() && successor.From == boundary.span.authored && successor.Arm == want && targetOK && boundary.span.input.OwnsSite(target) && proof.routeDigest.Available() && targetID.Available()
}

type CallArm struct {
	boundary CallBoundary
	proof    callArmProof
}

func (arm CallArm) Available() bool {
	return arm.boundary.Available() && arm.proof.validated && arm.proof.successor.From == arm.boundary.span.authored && arm.proof.routeDigest.Available() && arm.proof.targetID.Available() && arm.proof.contextID.Available()
}
func (arm CallArm) ContextID() identity.ContentID {
	if !arm.Available() {
		return identity.ContentID{}
	}
	return arm.proof.contextID
}

// Kind is the closed causal arm disposition already issued by Flow. It does
// not reveal an endpoint coordinate or let callers synthesize an arm.
func (arm CallArm) Kind() (flow.BoundaryArmKind, bool) {
	if !arm.Available() {
		return 0, false
	}
	return arm.proof.successor.Arm, true
}

func (arm CallArm) RouteDigest() (identity.ContentID, bool) {
	if !arm.Available() {
		return identity.ContentID{}, false
	}
	return arm.proof.routeDigest, true
}

func (arm CallArm) Target() (flow.Site, bool) {
	if !arm.Available() {
		return flow.Site{}, false
	}
	return arm.proof.target, true
}

func (input TransformerInput) OwnsCallArm(arm CallArm) bool {
	return input.OwnsCallBoundary(arm.boundary) && arm.boundary.span.input == input && arm.Available()
}
