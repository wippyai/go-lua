package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// TransformerInput is a zero-copy, owner-fenced view for transformers. It
// retains only the already-published Program root and its child identities;
// it creates no Program identity, row, cache, or relation.
type TransformerInput struct {
	owner                       *Program
	programID, sourceID, flowID identity.ContentID
	staticID, moduleID          identity.ContentID
	allocationReceipt           *allocationReceipt
}

// TransformerInput captures the already-published Program seal scalars once.
// Issuance fences the current live quartet and Flow provenance without
// retaining a second semantic-source transport object.
func (program *Program) TransformerInput() TransformerInput {
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil ||
		!program.id.Available() {
		return TransformerInput{}
	}
	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.Cold().ContentID()
	moduleID := program.module.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		program.flow.View().Provenance().Source != sourceID ||
		program.flow.View().Provenance().Flow != flowID ||
		program.flow.View().Provenance().Static != staticID ||
		program.flow.View().Provenance().Module != moduleID {
		return TransformerInput{}
	}
	input := TransformerInput{
		owner: program, programID: program.id,
		sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID,
		allocationReceipt: program.allocationReceipt,
	}
	if !input.valid() {
		return TransformerInput{}
	}
	return input
}

func (input TransformerInput) Available() bool { return input.valid() }

func (input TransformerInput) valid() bool {
	if input.owner == nil || !input.programID.Available() || !input.sourceID.Available() ||
		!input.flowID.Available() || !input.staticID.Available() || !input.moduleID.Available() ||
		input.owner.source == nil || input.owner.flow == nil || input.owner.static == nil || input.owner.module == nil ||
		input.owner.id != input.programID {
		return false
	}
	if !input.allocationReceipt.valid(input.owner) {
		return false
	}
	if input.owner.source.Cold().ContentID() != input.sourceID || input.owner.flow.ContentID() != input.flowID ||
		input.owner.static.Cold().ContentID() != input.staticID || input.owner.module.Cold().ContentID() != input.moduleID {
		return false
	}
	provenance := input.owner.flow.View().Provenance()
	return provenance.Source == input.sourceID && provenance.Flow == input.flowID &&
		provenance.Static == input.staticID && provenance.Module == input.moduleID
}

func (input TransformerInput) ContentID() identity.ContentID {
	if !input.Available() {
		return identity.ContentID{}
	}
	return input.programID
}

// Static exposes the already-published authored Static view to an owner
// artifact compiler. Consumers retain only the rows emitted by that compiler.
func (input TransformerInput) Static() programstatic.View {
	if !input.Available() {
		return programstatic.View{}
	}
	return input.owner.Static()
}

// Source exposes the already-published canonical Source view while a
// transformer construction proof is live. Consumers must copy only the
// scalar rows they own; the view itself is not retained by artifacts.
func (input TransformerInput) Source() source.View {
	if !input.Available() {
		return source.View{}
	}
	return input.owner.Source()
}

// Flow exposes the already-published canonical Flow view while a transformer
// construction proof is live. This is the narrow construction-time bridge
// for artifact builders that consume authored rows directly.
func (input TransformerInput) Flow() flow.View {
	if !input.Available() {
		return flow.View{}
	}
	return input.owner.Flow()
}

// StaticKeyText resolves an authored Static key through Source while an
// artifact row is being issued.
func (input TransformerInput) StaticKeyText(key keyspace.Key) (string, bool) {
	if !input.Available() || key == 0 {
		return "", false
	}
	literal, ok := input.owner.Source().Keys().Exact(key)
	return literal.String, ok && literal.Kind == keyspace.LiteralString
}

func (input TransformerInput) StaticKeyLiteral(key keyspace.Key) (keyspace.LiteralValue, bool) {
	if !input.Available() || key == 0 {
		return keyspace.LiteralValue{}, false
	}
	return input.owner.Source().Keys().Exact(key)
}

func StaticOccurrenceID(owner identity.ContentID, family uint8, term keyspace.Term) (id identity.ContentID, ok bool) {
	if !owner.Available() || term == 0 || family == 0 {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("program/static-occurrence/v1"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(owner[:])
	_, _ = h.Write([]byte{family})
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(term))
	_, _ = h.Write(word[:])
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

func (input TransformerInput) StaticTypeOfCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Static().Operators().TypeOfs().Count()
}
func (input TransformerInput) StaticTypeOfAt(index int) (keyspace.Term, keyspace.Term, bool) {
	if !input.Available() {
		return 0, 0, false
	}
	term, ok := input.owner.Static().Operators().TypeOfs().At(index)
	if !ok {
		return 0, 0, false
	}
	_, operand, ok := input.owner.Static().Operators().TypeOfs().Get(term)
	return term, operand, ok
}

func (input TransformerInput) StaticAnnotationCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Static().Operands().Annotations().Count()
}
func (input TransformerInput) StaticAnnotationAt(index int) (keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	if !input.Available() {
		return 0, 0, 0, false
	}
	sourceTerm, ok := input.owner.Static().Operands().Annotations().At(index)
	if !ok {
		return 0, 0, 0, false
	}
	annotation, ok := input.owner.Static().Operands().Annotations().Get(sourceTerm)
	if !ok {
		return 0, 0, 0, false
	}
	return sourceTerm, annotation.Target, annotation.Values, true
}
func (input TransformerInput) StaticAnnotationValue(values keyspace.Term, index int) (keyspace.Term, bool) {
	if !input.Available() {
		return 0, false
	}
	return input.owner.Flow().Authored().Values().Member(values, index)
}
func (input TransformerInput) StaticAnnotationValueCount(values keyspace.Term) (int, bool) {
	if !input.Available() {
		return 0, false
	}
	return input.owner.Flow().Authored().Values().Len(values)
}
func (input TransformerInput) StaticFrontier(term keyspace.Term) (identity.ContentID, uint32, bool) {
	if !input.Available() || term == 0 {
		return identity.ContentID{}, 0, false
	}
	bodyTerm, cursor, ok := input.owner.Source().Index().Frontier(term)
	if !ok || cursor < 0 || uint64(cursor) > uint64(^uint32(0)) {
		return identity.ContentID{}, 0, false
	}
	body, bodyOK := input.Body(bodyTerm)
	if !bodyOK {
		return identity.ContentID{}, 0, false
	}
	return body.PathID(), uint32(cursor), body.PathID().Available()
}

// StaticOperandKind is the closed disposition vocabulary copied into a
// ProgramArtifact static-input row. It is deliberately separate from Link's
// Boundary value algebra: Link later supplies the mounted value for a
// RuntimeSubject through its scalar identity.
type StaticOperandKind uint8

const (
	StaticOperandInvalid StaticOperandKind = iota
	StaticOperandKnown
	StaticOperandRuntimeSubject
	StaticOperandTypeValue
)

// StaticOperand is a seal-time proof of the exact scalar operand behind a
// TypeOf/annotation input. No authored term crosses this API.
type StaticOperand struct {
	kind      StaticOperandKind
	id        identity.ContentID
	literal   keyspace.LiteralValue
	reference identity.ContentID
	subject   identity.ContentID
	body      identity.ContentID
}

func (operand StaticOperand) Kind() StaticOperandKind         { return operand.kind }
func (operand StaticOperand) ID() identity.ContentID          { return operand.id }
func (operand StaticOperand) Literal() keyspace.LiteralValue  { return operand.literal }
func (operand StaticOperand) ReferenceID() identity.ContentID { return operand.reference }
func (operand StaticOperand) SubjectID() identity.ContentID   { return operand.subject }
func (operand StaticOperand) BodyPathID() identity.ContentID  { return operand.body }

// StaticOperandAt resolves one exact authored operand while the Program proof
// is live. Claims are transparent (matching the existing Static evaluator),
// TypeValues retain their static target reference, literals retain their exact
// payload, and fixed-cell reads retain the parent-issued Cell identity.
func (input TransformerInput) StaticOperandAt(term keyspace.Term) (StaticOperand, bool) {
	if !input.Available() || term == 0 {
		return StaticOperand{}, false
	}
	return input.staticOperandAt(term, make(map[keyspace.Term]struct{}))
}

func (input TransformerInput) staticOperandAt(term keyspace.Term, seen map[keyspace.Term]struct{}) (StaticOperand, bool) {
	if _, duplicate := seen[term]; duplicate {
		return StaticOperand{}, false
	}
	seen[term] = struct{}{}
	if ordinal := keyspace.TermOrdinal(term); ordinal != 0 {
		literals := input.owner.Source().Literals()
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyNil:
			if value, _, ok := literals.Nils().At(int(ordinal - 1)); ok && value == term {
				source, sourceOK := input.NilSourceAt(int(ordinal - 1))
				return StaticOperand{kind: StaticOperandKnown, id: source.ContextID(), literal: keyspace.LiteralValue{}}, sourceOK
			}
		case keyspace.FamilyBool:
			if value, _, payload, ok := literals.Bools().At(int(ordinal - 1)); ok && value == term {
				source, sourceOK := input.BoolSourceAt(int(ordinal - 1))
				return StaticOperand{kind: StaticOperandKnown, id: source.ContextID(), literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: payload}}, sourceOK
			}
		case keyspace.FamilyInteger:
			if value, _, payload, ok := literals.Integers().At(int(ordinal - 1)); ok && value == term {
				source, sourceOK := input.IntegerSourceAt(int(ordinal - 1))
				return StaticOperand{kind: StaticOperandKnown, id: source.ContextID(), literal: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: payload}}, sourceOK
			}
		case keyspace.FamilyFloat:
			if value, _, payload, ok := literals.Floats().At(int(ordinal - 1)); ok && value == term {
				source, sourceOK := input.FloatSourceAt(int(ordinal - 1))
				return StaticOperand{kind: StaticOperandKnown, id: source.ContextID(), literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: payload}}, sourceOK
			}
		case keyspace.FamilyString:
			if value, _, payload, ok := literals.Strings().At(int(ordinal - 1)); ok && value == term {
				source, sourceOK := input.StringSourceAt(int(ordinal - 1))
				return StaticOperand{kind: StaticOperandKnown, id: source.ContextID(), literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: payload}}, sourceOK
			}
		}
	}
	claims := input.owner.Flow().Authored().Claims()
	if owner, operand, _, ok := claims.Get(term); ok && owner != 0 && operand != 0 {
		return input.staticOperandAt(operand, seen)
	}
	typeValues := input.owner.Flow().Authored().TypeValues()
	if owner, ok := typeValues.Get(term); ok && owner != 0 && input.owner.Flow().Executable().Contains(term) {
		target, targetOK := input.owner.Static().Operands().TypeValues().Target(term)
		ref, refOK := input.owner.Static().StaticTypes().Ref(target)
		id, idOK := StaticTypeReferenceID(input.programID, ref)
		if targetOK && refOK && ref.Term() == target && idOK {
			body, bodyOK := input.Body(owner)
			if bodyOK {
				source, sourceOK := input.TypeValueSourceAt(int(keyspace.TermOrdinal(term) - 1))
				return StaticOperand{kind: StaticOperandTypeValue, id: source.ContextID(), reference: id, body: body.PathID()}, sourceOK
			}
		}
		return StaticOperand{}, false
	}
	reads := input.owner.Flow().Authored().Storage().Reads()
	if owner, source, _, ok := reads.Get(term); ok && owner != 0 && source != 0 && input.owner.Flow().Executable().Contains(term) {
		read, readOK := input.StorageReadAt(int(keyspace.TermOrdinal(term) - 1))
		if readOK {
			cell, cellOK := read.Cell()
			body, bodyOK := read.Body()
			if cellOK && bodyOK {
				return StaticOperand{kind: StaticOperandRuntimeSubject, id: read.ContextID(), subject: cell.ContextID(), body: body.PathID()}, true
			}
		}
	}
	return StaticOperand{}, false
}

// StaticExpressionID is the Program-issued identity of one authored static
// expression occurrence. It is intentionally distinct from the type-node
// identity: Link may join several qualified occurrences onto one type node.
func StaticExpressionID(owner identity.ContentID, ref programstatic.StaticTypeRef) (identity.ContentID, bool) {
	if !owner.Available() || ref.Term() == 0 {
		return identity.ContentID{}, false
	}
	return staticInputDigest("program/static-expression/v1", owner, ref.Term(), 0), true
}

// StaticInputID issues a dense, index-bearing row identity without narrowing
// the index into the uint8 occurrence-family namespace.
func StaticInputID(owner identity.ContentID, family uint8, source keyspace.Term, index uint32) (identity.ContentID, bool) {
	if !owner.Available() || source == 0 {
		return identity.ContentID{}, false
	}
	id := staticInputDigest("program/static-input/v1", owner, source, uint64(family)<<32|uint64(index))
	return id, id.Available()
}

func StaticScopeID(owner identity.ContentID, scope keyspace.Term) (identity.ContentID, bool) {
	if !owner.Available() || scope == 0 {
		return identity.ContentID{}, false
	}
	id := staticInputDigest("program/static-scope/v1", owner, scope, 0)
	return id, id.Available()
}

func staticInputDigest(domain string, owner identity.ContentID, term keyspace.Term, index uint64) identity.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(owner[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(term))
	_, _ = hash.Write(word[:])
	binary.BigEndian.PutUint64(word[:], index)
	_, _ = hash.Write(word[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// Span is an opaque owner-fenced join of one authored occurrence to its
// existing Entry and Finish Sites. It is transient, not a generic Port or a
// retained projection.
type Span struct {
	input    TransformerInput
	authored keyspace.Term
	entry    flow.Site
	finish   flow.Site
	context  identity.ContentID
}

func (input TransformerInput) Span(term keyspace.Term) (Span, bool) {
	if !input.Available() {
		return Span{}, false
	}
	ports, sites := input.owner.Flow().Ports(), input.owner.Flow().Causal().Sites()
	entry, entryOK := ports.Entry(term)
	finish, finishOK := ports.Finish(term)
	if !entryOK || !finishOK {
		return Span{}, false
	}
	entrySite, entrySiteOK := sites.ForTerm(entry)
	finishSite, finishSiteOK := sites.ForTerm(finish)
	if !entrySiteOK || !finishSiteOK {
		return Span{}, false
	}
	span := Span{input: input, authored: term, entry: entrySite, finish: finishSite}
	if !span.availableGeometry() {
		return Span{}, false
	}
	span.context = spanContextID(span)
	return span, span.Available()
}

// Available proves that this is still the exact published Program join.
// Equivalent artifact replay follows flow.Site.Equal semantics: matching
// sealed-quartet Sites remain valid; foreign/mutated handles fail closed.
func (span Span) Available() bool {
	return span.context.Available() && span.availableGeometry()
}

func (span Span) availableGeometry() bool {
	if !span.input.Available() || span.authored == 0 || !span.entry.Available() || !span.finish.Available() {
		return false
	}
	ports, sites := span.input.owner.Flow().Ports(), span.input.owner.Flow().Causal().Sites()
	entry, entryOK := ports.Entry(span.authored)
	finish, finishOK := ports.Finish(span.authored)
	wantEntry, wantEntryOK := sites.ForTerm(entry)
	wantFinish, wantFinishOK := sites.ForTerm(finish)
	return entryOK && finishOK && wantEntryOK && wantFinishOK &&
		span.entry.Equal(wantEntry) && span.finish.Equal(wantFinish)
}

func (span Span) Entry() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.entry, true
}

// Authored is a temporary compatibility escape hatch for Pack's unmigrated
// construction path. Transformer/compiler consumers must use ContextID and
// role proofs instead; this accessor will disappear with that flash cut.
func (span Span) Authored() (keyspace.Term, bool) {
	if !span.Available() {
		return 0, false
	}
	return span.authored, true
}

func (span Span) Finish() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.finish, true
}

// TailReturn returns the exact terminal Outcome owned by this Call span's
// already-sealed causal boundary. It never exposes the Flow Outcome term or
// scans authored Return rows; the boundary and Body outcome range are the
// sole proof chain.
func (span Span) TailReturn() (Outcome, bool) {
	if !span.Available() {
		return Outcome{}, false
	}
	boundary, boundaryOK := span.input.owner.Flow().Causal().Boundaries().For(span.authored)
	if !boundaryOK || boundary.Call != span.authored || boundary.TailReturn == 0 {
		return Outcome{}, false
	}
	outcome, ok := span.input.Outcome(boundary.TailReturn)
	body, bodyOK := span.input.ContainingBody(span.authored)
	outcomeKind, kindOK := outcome.Kind()
	target, targetOK := outcome.Target()
	return outcome, ok && bodyOK && span.input.OwnsBody(body) && span.input.OwnsOutcome(outcome) &&
		outcome.BelongsTo(body) && kindOK && outcomeKind == kind.OutcomeReturn && targetOK && target == 0
}

// Equal follows the published Site replay policy: equivalent sealed Programs
// compare by their exact-quartet Site identities, not by a Program pointer.
func (span Span) Equal(other Span) bool {
	return span.Available() && other.Available() && span.authored == other.authored &&
		span.context == other.context && span.entry.Equal(other.entry) && span.finish.Equal(other.finish)
}

// RootSpan first uses Source's sealed root projection, then returns that
// root's existing Flow span. It does not infer lexical ownership itself.
func (input TransformerInput) RootSpan(term keyspace.Term) (Span, bool) {
	if !input.Available() {
		return Span{}, false
	}
	root, ok := input.owner.Source().Index().Root(term)
	if !ok {
		return Span{}, false
	}
	return input.Span(root)
}

func (input TransformerInput) FinishSite(term keyspace.Term) (flow.Site, bool) {
	span, ok := input.Span(term)
	if !ok {
		return flow.Site{}, false
	}
	return span.Finish()
}

// GuardCount and GuardAt accept only a Site issued by this exact Program's
// Causal owner. The Site is re-resolved by term and contextual identity before
// forwarding to Continuation, rejecting same-shaped foreign handles.
func (input TransformerInput) GuardCount(site flow.Site) (int, bool) {
	term, ok := input.ownedSite(site)
	if !ok {
		return 0, false
	}
	return input.owner.Flow().Continuation().GuardCount(term)
}

func (input TransformerInput) ownedSite(site flow.Site) (keyspace.Term, bool) {
	if !input.Available() || !site.Available() {
		return 0, false
	}
	term, ok := site.Term()
	if !ok {
		return 0, false
	}
	sites := input.owner.Flow().Causal().Sites()
	issued, ok := sites.ForTerm(term)
	if !ok || !sites.Owns(site) || !issued.Equal(site) {
		return 0, false
	}
	return term, true
}

// Body is an opaque transformer view over one existing Flow Body boundary.
// It retains no body/outcome rows and exposes only existing Causal Sites plus
// the optional Function boundary needed for formal/capture access.
type Body struct {
	input    TransformerInput
	boundary flow.BodyBoundary
	function flow.FunctionBoundary
}

// BodyCount forwards Source's sole canonical Body denominator. It does not
// retain an input-owned index or promote the Function-boundary join to one.
func (input TransformerInput) BodyCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Source().Identity().FamilyCount(keyspace.FamilyBody)
}

// BodyAt returns one opaque existing Body view in canonical Body order.
func (input TransformerInput) BodyAt(index int) (Body, bool) {
	if !input.Available() || index < 0 || index >= input.BodyCount() {
		return Body{}, false
	}
	term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	return input.Body(term)
}

// Body joins an authored Body to its published boundary. Root/non-Function
// Bodies retain an unavailable Function boundary rather than a fabricated one.
func (input TransformerInput) Body(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	boundaries := input.owner.Flow().FunctionBoundaries()
	boundary, ok := boundaries.ForBody(term)
	if !ok {
		return Body{}, false
	}
	function, _ := boundaries.ForFunctionBody(term)
	body := Body{input: input, boundary: boundary, function: function}
	return body, body.Available()
}

// Function resolves one authored Function directly to its exact transformer
// proof. It is an inverse lookup into the sealed FunctionBoundary relation,
// not a scan of authored Functions or a caller-minted coordinate.
func (input TransformerInput) Function(term keyspace.Term) (Function, bool) {
	if !input.Available() {
		return Function{}, false
	}
	boundary, boundaryOK := input.owner.Flow().FunctionBoundaries().For(term)
	bodyTerm, bodyOK := boundary.Body()
	body, issuedBodyOK := input.Body(bodyTerm)
	function := Function{body: body, boundary: boundary}
	return function, boundaryOK && bodyOK && issuedBodyOK && function.Available()
}

// OwnsBody authenticates a Body issued by this exact hot Program facade.
// Equivalent replay Bodies deliberately do not pass: mount-local consumers
// must retain and thread their own issued proof rather than substitute one.
func (input TransformerInput) OwnsBody(body Body) bool {
	if !input.Available() || body.input != input || !body.Available() {
		return false
	}
	boundaries := input.owner.Flow().FunctionBoundaries()
	if !boundaries.OwnsBody(body.boundary) {
		return false
	}
	if body.function.Available() {
		return boundaries.OwnsFunction(body.function)
	}
	return true
}

// OwnsSite authenticates an exact hot Causal Site issued by this Program.
// Equivalent replay Sites are intentionally rejected at mount-local joins.
func (input TransformerInput) OwnsSite(site flow.Site) bool {
	if !input.Available() || !site.Available() {
		return false
	}
	term, ok := site.Term()
	if !ok {
		return false
	}
	sites := input.owner.Flow().Causal().Sites()
	issued, ok := sites.ForTerm(term)
	return ok && sites.Owns(site) && issued.Equal(site) && issued.ContextID() == site.ContextID()
}

// OwnsOutcome authenticates a terminal Outcome issued by this exact hot
// Program facade. Equivalent replay outcomes intentionally fail this fence.
func (input TransformerInput) OwnsOutcome(outcome Outcome) bool {
	if !input.Available() || outcome.body.input != input || !input.OwnsBody(outcome.body) || !outcome.Available() {
		return false
	}
	site, ok := outcome.Site()
	return ok && input.OwnsSite(site)
}

// Outcome resolves one existing Body Outcome through FunctionBoundary's sole
// dense inverse and the existing Causal Site table. It does not scan a Body's
// Outcome range or create another outcome index.
func (input TransformerInput) Outcome(term keyspace.Term) (Outcome, bool) {
	if !input.Available() || term == 0 {
		return Outcome{}, false
	}
	boundary, boundaryOK := input.owner.Flow().FunctionBoundaries().ForOutcome(term)
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := input.Body(bodyTerm)
	exit, ordinal, exitOK := boundary.OutcomeForTerm(term)
	if !boundaryOK || !bodyTermOK || !bodyOK || !body.boundary.Equal(boundary) || !exitOK || exit.Outcome != term {
		return Outcome{}, false
	}
	site, _ := input.owner.Flow().Causal().Sites().ForTerm(term)
	outcome := Outcome{ordinal: ordinal, body: body, site: site, kind: exit.Kind, target: exit.Target}
	return outcome, outcome.Available()
}

// ContainingBody resolves Source's existing lexical containment internally
// and returns only the opaque Body proof. It does not expose the containing
// raw Body coordinate for callers to rejoin.
func (input TransformerInput) ContainingBody(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	body, _, _, ok := input.owner.Source().Index().Position(term)
	if !ok {
		return Body{}, false
	}
	return input.Body(body)
}

func (body Body) Available() bool {
	if !body.input.Available() || !body.boundary.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return false
	}
	want, wantOK := body.input.owner.Flow().FunctionBoundaries().ForBody(term)
	if !wantOK || !body.boundary.Equal(want) {
		return false
	}
	function, functionOK := body.input.owner.Flow().FunctionBoundaries().ForFunctionBody(term)
	if functionOK {
		return body.function.Available() && body.function.Equal(function)
	}
	return !body.function.Available()
}

// Equal compares the existing exact-quartet Body boundary proof. It never
// compares or exposes an authored Body term.
func (body Body) Equal(other Body) bool {
	return body.Available() && other.Available() && body.boundary.Equal(other.boundary)
}

// ContextID is Flow's stable exact-quartet identity for this Body boundary.
func (body Body) ContextID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.boundary.ContextID()
}

// ProgramID returns the already-published Program owner of this exact Body
// proof. It is a scalar provenance fence for reusable transformer consumers;
// it neither reopens Program state nor exposes the authored Body term.
func (body Body) ProgramID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.input.ContentID()
}

// PathID returns Flow's owner-local lexical Body path. Unlike ContextID it
// carries no quartet identity and is therefore suitable for semantic
// descriptors that must replay across equivalent owner publication.
func (body Body) PathID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	term, ok := body.boundary.Body()
	if !ok {
		return identity.ContentID{}
	}
	path, ok := body.input.owner.Flow().BodyPath(term)
	if !ok {
		return identity.ContentID{}
	}
	return path
}

// Executable reports the exact sealed Flow executable membership for this
// Body. It is a scalar proof copied for artifact boundary filtering.
func (body Body) Executable() bool {
	if !body.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	return ok && body.input.owner.Flow().Executable().Contains(term)
}

// RootCount returns Source's existing root denominator for this Body. The
// Body term remains internal to this proof-native join.
func (body Body) RootCount() (int, bool) {
	if !body.Available() {
		return 0, false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return 0, false
	}
	return body.input.owner.Source().Index().BodyRootLen(term)
}

// Root is one Source-owned Body root proof. Its Flow span is optional because
// Source roots include non-executable authored structure; callers never
// receive the containing raw Body coordinate to rejoin themselves.
type Root struct {
	body     Body
	ordinal  int
	authored keyspace.Term
	span     Span
}

func (root Root) Available() bool {
	if !root.body.Available() || root.authored == 0 {
		return false
	}
	term, ok := root.body.boundary.Body()
	if !ok {
		return false
	}
	candidate, ok := root.body.input.owner.Source().Index().BodyRootAt(term, root.ordinal)
	if !ok || candidate != root.authored {
		return false
	}
	issued, issuedOK := root.body.input.Span(root.authored)
	if issuedOK != root.span.Available() {
		return false
	}
	return !issuedOK || (root.body.input.OwnsSpan(root.span) && root.span == issued)
}

func (root Root) Authored() (keyspace.Term, bool) {
	if !root.Available() {
		return 0, false
	}
	return root.authored, true
}

// Executable forwards Flow's exact executable membership for this authored
// Source root. Span availability is deliberately not used as a classifier.
func (root Root) Executable() bool {
	return root.Available() && root.body.input.owner.Flow().Executable().Contains(root.authored)
}

// Span returns the existing Flow Span if Ports/Causal publish one. Source-root
// executability and Span availability are independent sealed relations.
func (root Root) Span() (Span, bool) {
	if !root.Available() || !root.span.Available() {
		return Span{}, false
	}
	return root.span, true
}

// RootAt returns one existing Source root proof. It attempts the existing
// Flow join once but does not reject non-executable Source structure.
func (body Body) RootAt(index int) (Root, bool) {
	if !body.Available() || index < 0 {
		return Root{}, false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return Root{}, false
	}
	root, ok := body.input.owner.Source().Index().BodyRootAt(term, index)
	if !ok {
		return Root{}, false
	}
	span, _ := body.input.Span(root)
	result := Root{body: body, ordinal: index, authored: root, span: span}
	return result, result.Available()
}

// ExecutableRoot is an artifact-safe proof of one direct executable Body
// root. It carries only Flow's sealed semantic identity, family, and dense
// executable ordinal: neither the authored Term nor its Span may cross the
// ProgramArtifact boundary.
type ExecutableRoot struct {
	catalog *executableRootCatalog
	ordinal int
	id      identity.ContentID
	family  keyspace.Family
}

// ExecutableRoots is Program's complete dense artifact-safe root catalog for
// one Body. Construction fails closed when any Source denominator row cannot
// be joined to Flow; consumers never infer a denominator by silently skipping
// malformed source rows.
type ExecutableRoots struct {
	catalog *executableRootCatalog
}

type executableRootCatalog struct {
	body   Body
	rows   []ExecutableRoot
	sealed bool
}

func (roots ExecutableRoots) Available() bool {
	return roots.catalog != nil && roots.catalog.sealed && roots.catalog.body.Available()
}
func (roots ExecutableRoots) Count() int {
	if !roots.Available() {
		return 0
	}
	return len(roots.catalog.rows)
}
func (roots ExecutableRoots) At(index int) (ExecutableRoot, bool) {
	if !roots.Available() || index < 0 || index >= len(roots.catalog.rows) {
		return ExecutableRoot{}, false
	}
	root := roots.catalog.rows[index]
	return root, root.Available()
}

func (root ExecutableRoot) Available() bool {
	if root.catalog == nil || !root.catalog.sealed || !root.catalog.body.Available() || root.ordinal < 0 || !root.id.Available() || root.family == keyspace.FamilyInvalid || root.ordinal >= len(root.catalog.rows) {
		return false
	}
	issued := root.catalog.rows[root.ordinal]
	return issued.catalog == root.catalog && issued.ordinal == root.ordinal && issued.id == root.id && issued.family == root.family
}
func (root ExecutableRoot) ID() identity.ContentID {
	if !root.Available() {
		return identity.ContentID{}
	}
	return root.id
}
func (root ExecutableRoot) Family() keyspace.Family {
	if !root.Available() {
		return keyspace.FamilyInvalid
	}
	return root.family
}

// ExecutableRootCount is the dense denominator after filtering Source roots
// through Flow's sealed executable and semantic-path proofs.
func (body Body) ExecutableRootCount() int {
	roots, ok := body.ExecutableRoots()
	if !ok {
		return 0
	}
	return roots.Count()
}

// ExecutableRootAt issues one dense artifact-safe root proof.
func (body Body) ExecutableRootAt(index int) (ExecutableRoot, bool) {
	roots, ok := body.ExecutableRoots()
	if !ok {
		return ExecutableRoot{}, false
	}
	return roots.At(index)
}

func (body Body) ExecutableRoots() (ExecutableRoots, bool) {
	if !body.Available() {
		return ExecutableRoots{}, false
	}
	count, countOK := body.RootCount()
	if !countOK {
		return ExecutableRoots{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	if !bodyOK {
		return ExecutableRoots{}, false
	}
	catalog := &executableRootCatalog{body: body, rows: make([]ExecutableRoot, 0, count)}
	for sourceIndex := 0; sourceIndex < count; sourceIndex++ {
		authored, rootOK := body.input.owner.Source().Index().BodyRootAt(bodyTerm, sourceIndex)
		if !rootOK {
			return ExecutableRoots{}, false
		}
		if !body.input.owner.Flow().Executable().Contains(authored) {
			continue
		}
		id, idOK := body.input.owner.Flow().SemanticTermPath(authored)
		root := ExecutableRoot{catalog: catalog, ordinal: len(catalog.rows), id: id, family: keyspace.TermFamily(authored)}
		if !idOK || !root.id.Available() || root.family == keyspace.FamilyInvalid {
			return ExecutableRoots{}, false
		}
		catalog.rows = append(catalog.rows, root)
	}
	catalog.sealed = true
	return ExecutableRoots{catalog: catalog}, true
}

func (input TransformerInput) OwnsExecutableRoot(root ExecutableRoot) bool {
	return input.Available() && root.catalog != nil && root.catalog.body.input == input && input.OwnsBody(root.catalog.body) && root.Available()
}
func (input TransformerInput) OwnsExecutableRoots(roots ExecutableRoots) bool {
	return input.Available() && roots.catalog != nil && roots.catalog.body.input == input && input.OwnsBody(roots.catalog.body) && roots.Available()
}

// Function is a temporary raw Flow escape hatch for unmigrated compiler
// paths. New transformer/compiler consumers must use TransformerFunction,
// Formal, Vararg, and Capture proofs below instead.
func (body Body) Function() (flow.FunctionBoundary, bool) {
	if !body.Available() || !body.function.Available() {
		return flow.FunctionBoundary{}, false
	}
	return body.function, true
}

// EntrySite returns the existing Causal Site at this Body's boundary Entry.
func (body Body) EntrySite() (flow.Site, bool) {
	if !body.Available() {
		return flow.Site{}, false
	}
	entry, ok := body.boundary.Entry()
	if !ok {
		return flow.Site{}, false
	}
	return body.input.owner.Flow().Causal().Sites().ForTerm(entry)
}

// Outcome is one existing terminal Body Outcome joined to its Causal Site.
// Kind and Target are the typed Flow metadata; no raw Outcome term is exposed.
type Outcome struct {
	ordinal  int
	body     Body
	site     flow.Site
	kind     kind.OutcomeKind
	target   keyspace.Term
	returned flow.BodyReturn
}

func (body Body) OutcomeCount() int {
	if !body.Available() {
		return 0
	}
	return body.boundary.OutcomeCount()
}

// OutcomeAt returns one Body-bound typed Outcome proof. A Site is optional:
// non-terminal Break/Goto Outcomes remain meaningful Body facts even though
// Causal intentionally has no terminal Site for them.
func (body Body) OutcomeAt(index int) (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	exit, ok := body.boundary.OutcomeAt(index)
	if !ok {
		return Outcome{}, false
	}
	site, _ := body.input.owner.Flow().Causal().Sites().ForTerm(exit.Outcome)
	outcome := Outcome{ordinal: index, body: body, site: site, kind: exit.Kind, target: exit.Target}
	return outcome, outcome.Available()
}

func (outcome Outcome) Available() bool {
	if !outcome.body.Available() {
		return false
	}
	if outcome.ordinal < 0 || outcome.ordinal >= outcome.body.boundary.OutcomeCount() {
		return false
	}
	exit, ok := outcome.body.boundary.OutcomeAt(outcome.ordinal)
	if !ok || exit.Kind != outcome.kind || exit.Target != outcome.target {
		return false
	}
	issuedSite, issuedOK := outcome.body.input.owner.Flow().Causal().Sites().ForTerm(exit.Outcome)
	if issuedOK != outcome.site.Available() {
		return false
	}
	if issuedOK && (!outcome.body.input.OwnsSite(outcome.site) || !outcome.site.Equal(issuedSite)) {
		return false
	}
	if outcome.returned.Available() {
		returnSite, returnOK := outcome.returned.Outcome()
		return returnOK && outcome.kind == kind.OutcomeReturn && outcome.target == 0 && outcome.site.Equal(returnSite)
	}
	return true
}

func (outcome Outcome) Site() (flow.Site, bool) {
	if !outcome.Available() || !outcome.site.Available() || !outcome.body.input.OwnsSite(outcome.site) {
		return flow.Site{}, false
	}
	return outcome.site, true
}

// ContextID is the exact Causal Site identity of this terminal Outcome.
func (outcome Outcome) ContextID() identity.ContentID {
	site, ok := outcome.Site()
	if !ok {
		return identity.ContentID{}
	}
	return site.ContextID()
}

func (outcome Outcome) Equal(other Outcome) bool {
	return outcome.Available() && other.Available() && outcome.ordinal == other.ordinal && outcome.body.Equal(other.body) &&
		outcome.kind == other.kind && outcome.target == other.target && outcome.ContextID() == other.ContextID()
}

// BelongsTo proves the existing Body/Outcome ownership join without exposing
// either raw Flow coordinate. Exact mount-local consumers must additionally
// use TransformerInput.OwnsBody and OwnsOutcome.
func (outcome Outcome) BelongsTo(body Body) bool {
	return outcome.Available() && body.Available() && outcome.body.Equal(body)
}

func (outcome Outcome) Kind() (kind.OutcomeKind, bool) {
	if !outcome.Available() {
		return 0, false
	}
	return outcome.kind, true
}

func (outcome Outcome) Target() (keyspace.Term, bool) {
	if !outcome.Available() {
		return 0, false
	}
	return outcome.target, true
}

// Return returns this Body's sole targetless executable OutcomeReturn proof.
// The sealed Flow projection has already validated ReturnExit, Propagation,
// activation, and executable Values ownership; this facade only threads it.
func (body Body) Return() (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	returned, returnedOK := body.input.owner.Flow().BodyReturns().ForBody(body.boundary)
	returnSite, siteOK := returned.Outcome()
	returnTerm, termOK := returnSite.Term()
	if !returnedOK || !siteOK || !termOK {
		return Outcome{}, false
	}
	outcome, outcomeOK := body.input.Outcome(returnTerm)
	if !outcomeOK || !outcome.BelongsTo(body) {
		return Outcome{}, false
	}
	outcome.returned = returned
	return outcome, outcome.Available()
}

// Normal returns this Body's canonical OutcomeNormal proof through Flow's
// existing Body-exit projection and FunctionBoundary's direct Outcome inverse.
// The authored coordinates remain inside the Program-owned facade.
func (body Body) Normal() (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	normalTerm, normalOK := body.input.owner.Flow().Outcomes().BodyExit(bodyTerm, kind.OutcomeNormal)
	if !bodyOK || !normalOK {
		return Outcome{}, false
	}
	outcome, outcomeOK := body.input.Outcome(normalTerm)
	outcomeKind, kindOK := outcome.Kind()
	target, targetOK := outcome.Target()
	return outcome, outcomeOK && outcome.BelongsTo(body) && kindOK && outcomeKind == kind.OutcomeNormal && targetOK && target == 0
}

// ReturnValuesCount exposes the owner-issued ordered executable Values range
// only for an Outcome returned by Body.Return.
func (outcome Outcome) ReturnValuesCount() int {
	if !outcome.Available() || !outcome.returned.Available() {
		return 0
	}
	return outcome.returned.ValuesCount()
}

// ReturnValueAt returns an existing Span for one ordered executable Values
// alternative. The raw Values coordinate remains internal to this facade.
func (outcome Outcome) ReturnValueAt(index int) (Span, bool) {
	if !outcome.Available() || !outcome.returned.Available() {
		return Span{}, false
	}
	site, siteOK := outcome.returned.ValueAt(index)
	term, termOK := site.Term()
	if !siteOK || !termOK {
		return Span{}, false
	}
	span, spanOK := outcome.body.input.Span(term)
	return span, spanOK
}

// OutcomeSiteAt is the compact Site-only OutcomeAt form.
func (body Body) OutcomeSiteAt(index int) (flow.Site, bool) {
	outcome, ok := body.OutcomeAt(index)
	if !ok {
		return flow.Site{}, false
	}
	return outcome.Site()
}

// Regions forwards the Program-local recurrence projection already owned by
// Flow.Causal; it performs no route grouping or SCC reconstruction.
func (input TransformerInput) Regions() flow.Regions {
	if !input.Available() {
		return flow.Regions{}
	}
	return input.owner.Flow().Local().Regions()
}

// LocalWTO forwards Flow's one parent-issued local schedule certificate.
// It exposes no source-control graph or raw schedule coordinate, so an
// analyzer may only project it into its artifact rather than rediscovering
// components or nesting.
func (input TransformerInput) LocalWTO() flow.LocalWTO {
	if !input.Available() {
		return flow.LocalWTO{}
	}
	return input.owner.Flow().Local().WTO()
}

// Provenance returns the exact four-owner fence already committed by Flow.
// It is a scalar proof copy: callers cannot obtain the underlying Flow owner
// or use this method to construct a second topology authority.
func (input TransformerInput) Provenance() (flow.Provenance, bool) {
	if !input.Available() {
		return flow.Provenance{}, false
	}
	provenance := input.owner.Flow().Provenance()
	if provenance.Source != input.sourceID || provenance.Flow != input.flowID ||
		provenance.Static != input.staticID || provenance.Module != input.moduleID {
		return flow.Provenance{}, false
	}
	return provenance, true
}

// CausalSiteCount and CausalSiteAt forward the one sealed Causal owner. They
// are intentionally typed rows rather than a generic graph/endpoint view.
func (input TransformerInput) CausalSiteCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Causal().Sites().Count()
}

func (input TransformerInput) CausalSiteAt(index int) (flow.Site, bool) {
	if !input.Available() || index < 0 {
		return flow.Site{}, false
	}
	return input.owner.Flow().Causal().Sites().At(index)
}

// CausalSuccessorCount and CausalSuccessorAt forward existing final routes.
// No endpoint pair or SCC membership is inferred by these methods.
func (input TransformerInput) CausalSuccessorCount(from keyspace.Term) int {
	if !input.Available() || keyspace.TermFamily(from) == keyspace.FamilyInvalid {
		return 0
	}
	return input.owner.Flow().Causal().Successors().Count(from)
}

func (input TransformerInput) CausalSuccessorAt(from keyspace.Term, index int) (flow.Successor, bool) {
	if !input.Available() || index < 0 || keyspace.TermFamily(from) == keyspace.FamilyInvalid {
		return flow.Successor{}, false
	}
	return input.owner.Flow().Causal().Successors().At(from, index)
}

// CausalBoundaryCount and CausalBoundaryAt forward the sealed dynamic-call
// boundary plane, including all normal/tail/exception arms.
func (input TransformerInput) CausalBoundaryCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Causal().Boundaries().Count()
}

func (input TransformerInput) CausalBoundaryAt(index int) (flow.CallBoundary, bool) {
	if !input.Available() || index < 0 {
		return flow.CallBoundary{}, false
	}
	return input.owner.Flow().Causal().Boundaries().At(index)
}

// AssignmentPredecessor returns Flow's existing reverse commit successor for
// one Write. It does not inspect authored storage order or reconstruct an
// endpoint route.
func (input TransformerInput) AssignmentPredecessor(write keyspace.Term) (flow.Successor, bool) {
	if !input.Available() || keyspace.TermFamily(write) != keyspace.FamilyWrite {
		return flow.Successor{}, false
	}
	return input.owner.Flow().Causal().Successors().AssignmentPredecessor(write)
}
