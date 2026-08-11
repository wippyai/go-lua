package core

import (
	"errors"
	"io"

	"github.com/wippyai/go-lua/program/internal/canonical"
)

const (
	artifactRecordRoot uint64 = iota + 1
	artifactRecordDependency
	artifactRecordProgram
	artifactRecordTerm
	artifactRecordImplicitReads
	artifactRecordEquationCache
	artifactRecordEquationBody
	artifactRecordEquationEdge
	artifactRecordEquationBoundary
	artifactRecordMetamethodCandidates
)

func encodeArtifact(dst io.Writer, p *Program, envelope ArtifactEnvelope) error {
	var w canonical.Writer
	if err := w.Reset(dst, artifactCodecDomain, artifactCodecVersion); err != nil {
		return err
	}
	e := artifactEncoder{p: p, w: &w, measure: canonical.StreamMeasure{
		// Reset emits fixed domain and codec-version frames. The domain is
		// raw-compared by Reader.Header, never copied into a decoded string.
		Events: 2,
	}}
	if !artifactMeasureAllowed(e.measure) {
		return ErrArtifactLimit
	}
	e.root(envelope)
	if e.err != nil {
		return e.err
	}
	return w.Finish()
}

type artifactEncoder struct {
	p       *Program
	w       *canonical.Writer
	err     error
	measure canonical.StreamMeasure
}

func (e *artifactEncoder) call(err error) {
	if e.err == nil && err != nil {
		e.err = err
	}
}

// frame accounts for the exact canonical event that is about to be written.
// It is the Encode-side half of canonical.Scan's reconstruction contract: a
// successfully encoded artifact is already eligible for Decode's first pass.
func (e *artifactEncoder) frame(stringBytes int) bool {
	if e == nil || e.err != nil {
		return false
	}
	if e.measure.Events == ^uint64(0) || uint64(stringBytes) > ^uint64(0)-e.measure.StringBytes {
		e.err = ErrArtifactLimit
		return false
	}
	next := e.measure
	next.Events++
	next.StringBytes += uint64(stringBytes)
	if !artifactMeasureAllowed(next) {
		e.err = ErrArtifactLimit
		return false
	}
	e.measure = next
	return true
}
func (e *artifactEncoder) u(v uint64) {
	if e.frame(0) {
		e.call(e.w.Uint(v))
	}
}
func (e *artifactEncoder) b(v bool) {
	if e.frame(0) {
		e.call(e.w.Bool(v))
	}
}
func (e *artifactEncoder) s(v string) {
	if e.frame(len(v)) {
		e.call(e.w.String(v))
	}
}
func (e *artifactEncoder) r(v uint64) {
	if e.frame(0) {
		e.call(e.w.Record(v))
	}
}
func (e *artifactEncoder) n(v uint64) {
	if e.frame(0) {
		e.call(e.w.Count(v))
	}
}
func (e *artifactEncoder) id(v ContentID) {
	if e.frame(0) {
		e.call(e.w.Bytes(v[:]))
	}
}

func (e *artifactEncoder) root(envelope ArtifactEnvelope) {
	e.r(artifactRecordRoot)
	e.id(envelope.Target)
	e.id(e.p.ContentID())
	e.s(envelope.Provenance)
	e.n(uint64(len(envelope.Dependencies)))
	for _, dependency := range envelope.Dependencies {
		e.r(artifactRecordDependency)
		e.s(dependency.Name)
		e.id(dependency.ID)
	}
	e.r(artifactRecordProgram)
	e.s(e.p.sourceName)
	e.term(e.p.entry)
	e.term(e.p.chunkVararg)
	for tag := uint8(1); tag < tagCount && e.err == nil; tag++ {
		count, ok := e.authoredCount(tag)
		if !ok {
			e.err = errors.New("program artifact: unknown Program family")
			return
		}
		e.u(uint64(tag))
		e.n(uint64(count))
		for index := 0; index < count && e.err == nil; index++ {
			e.row(tag, index)
		}
	}
	// Implicit global reads are sparse binder evidence carried by ordinary Read
	// terms. They are neither a Cell property nor a Seal projection, so the
	// artifact retains the occurrence relation explicitly.
	e.r(artifactRecordImplicitReads)
	e.n(uint64(len(e.p.implicitReads)))
	for _, read := range e.p.implicitReads {
		e.term(read)
	}
	e.metamethodCandidates()
	e.equationCache(envelope.Equations)
}

// metamethodCandidates is one fixed-order typed schema section.  It carries
// only source Terms; normal/outcome projections and direct indexes are rebuilt
// by Seal, and no runtime handler/name registry is serialized.
func (e *artifactEncoder) metamethodCandidates() {
	if e.err != nil {
		return
	}
	e.r(artifactRecordMetamethodCandidates)
	e.candidateSources(e.p.unaryNumeric)
	e.candidateSources(e.p.lengths)
	e.candidateSources(e.p.arithmetic)
	e.candidateSources(e.p.bitwise)
	e.candidateSources(e.p.concat)
	e.candidateSources(e.p.equality)
	e.candidateSources(e.p.order)
	e.candidateSources(e.p.indexGet)
	e.candidateSources(e.p.indexSet)
	e.candidateSources(e.p.callable)
}

func (e *artifactEncoder) candidateSources(rows []candidateSourceRow) {
	e.n(uint64(len(rows)))
	for _, row := range rows {
		e.term(row.source)
	}
}

func (e *artifactEncoder) semantic(key ArtifactSemanticKey) {
	e.id(key.ID)
	e.u(key.Version)
}

func (e *artifactEncoder) equationCache(cache *ArtifactEquationCache) {
	e.b(cache != nil)
	if cache == nil || e.err != nil {
		return
	}
	e.r(artifactRecordEquationCache)
	e.id(cache.Program)
	e.id(cache.Module)
	e.semantic(cache.Engine)
	e.n(uint64(len(cache.Factors)))
	for _, factor := range cache.Factors {
		e.semantic(factor)
	}
	e.n(uint64(len(cache.Rules)))
	for _, rule := range cache.Rules {
		e.semantic(rule)
	}
	e.n(uint64(len(cache.Bodies)))
	for _, body := range cache.Bodies {
		e.r(artifactRecordEquationBody)
		e.term(body.Body)
		e.n(uint64(len(body.Terms)))
		for _, term := range body.Terms {
			e.term(term)
		}
		e.n(uint64(len(body.Edges)))
		for _, edge := range body.Edges {
			e.r(artifactRecordEquationEdge)
			e.term(edge.From)
			e.term(edge.To)
			e.term(edge.Decision)
			e.b(edge.Truthy)
			e.term(edge.Mu)
			e.n(uint64(len(edge.MuDecisions)))
			for _, decision := range edge.MuDecisions {
				e.term(decision)
			}
		}
	}
	e.n(uint64(len(cache.Boundary)))
	for _, boundary := range cache.Boundary {
		e.r(artifactRecordEquationBoundary)
		e.semantic(boundary.Rule)
		e.semantic(boundary.Output)
		e.term(boundary.Activation)
		e.term(boundary.At)
		e.u(uint64(boundary.InputArity))
		e.n(uint64(len(boundary.Reads)))
		for _, read := range boundary.Reads {
			e.u(uint64(read.Position))
			e.semantic(read.Factor)
			e.b(read.Exact)
			if read.Exact {
				e.u(read.Key)
			}
		}
		e.n(uint64(len(boundary.Writes)))
		for _, key := range boundary.Writes {
			e.u(key)
		}
	}
}

func (e *artifactEncoder) authoredCount(tag uint8) (int, bool) {
	if tag == tagOutcome {
		return 0, true // Outcomes are Seal-derived and never persisted.
	}
	return (&programEncoder{p: e.p}).familyCount(tag)
}

func (e *artifactEncoder) row(tag uint8, index int) {
	p := e.p
	term := makeTerm(tag, uint32(index+1))
	if !p.Valid(term) || index >= len(p.spans[tag]) {
		e.err = errors.New("program artifact: invalid authored row")
		return
	}
	e.r(artifactRecordTerm)
	e.u(uint64(tag))
	e.u(uint64(index + 1))
	e.span(p.spans[tag][index])
	switch tag {
	case tagNil:
		e.term(p.nils[index])
	case tagBool:
		x := p.bools[index]
		e.term(x.owner)
		e.b(x.value)
	case tagInteger:
		x := p.integers[index]
		e.term(x.owner)
		e.i(x.value)
	case tagFloat:
		x := p.floats[index]
		e.term(x.owner)
		e.u(x.bits)
	case tagString:
		x := p.strings[index]
		e.term(x.owner)
		e.s(x.value)
	case tagValues:
		x := p.values[index]
		e.term(x.owner)
		e.terms(p.valueTerms, x.fixed)
		e.term(x.tail)
	case tagLensExact:
		x := p.lensExact[index]
		e.term(x.owner)
		e.u(uint64(x.kind))
		e.term(x.base)
		e.term(x.source)
	case tagLensKey:
		x := p.lensKeys[index]
		e.term(x.owner)
		e.term(x.base)
		e.term(x.key)
	case tagReturn:
		x := p.returns[index]
		e.term(x.owner)
		e.term(x.values)
	case tagBreak:
		e.term(p.breaks[index].owner)
	case tagLabel:
		e.term(p.labelOwners[index])
	case tagGoto:
		e.term(p.gotoOwners[index])
		e.term(p.gotoTargets[index])
	case tagBody:
		e.terms(p.sourceTerms, p.bodies[index].source)
	case tagCell:
		e.cell(index)
	case tagRead:
		x := p.reads[index]
		e.term(x.owner)
		e.term(x.source)
	case tagVararg:
		x := p.varargs[index]
		e.term(x.owner)
		e.term(x.cell)
	case tagUnary:
		x := p.unaries[index]
		e.term(x.owner)
		e.u(uint64(x.op))
		e.term(x.operand)
	case tagBinary:
		x := p.binaries[index]
		e.term(x.owner)
		e.u(uint64(x.op))
		e.term(x.left)
		e.term(x.right)
	case tagSelect:
		x := p.selects[index]
		e.term(x.owner)
		e.u(uint64(x.op))
		e.term(x.left)
		e.term(x.right)
	case tagBind:
		x := p.binds[index]
		e.term(x.owner)
		e.terms(p.bindTerms, x.cells)
		e.term(x.values)
	case tagAssign:
		x := p.assigns[index]
		e.term(x.owner)
		e.termsByTerm(tagWrite, x.writes)
		e.term(x.values)
	case tagFunction:
		e.function(index)
	case tagCall:
		x := p.calls[index]
		e.term(x.owner)
		e.term(x.callee)
		e.term(x.receiver)
		e.term(x.actuals)
		e.terms(p.callTypeArgs, x.typeArgs)
	case tagBranch:
		x := p.branches[index]
		e.term(x.owner)
		e.term(x.condition)
		e.term(x.whenTrue)
		e.term(x.whenFalse)
	case tagLoop:
		e.loop(index)
	case tagTable:
		x := p.tables[index]
		e.term(x.owner)
		e.terms(p.tableFieldTerms, x.fields)
	case tagKey:
		x := p.keys[index]
		e.term(x.owner)
		e.u(uint64(x.kind))
		e.key(x.exact)
	case tagTypeAlias:
		x := p.typeAliases[index]
		e.term(x.owner)
		e.term(x.target)
		e.key(x.name)
		e.span(x.nameSpan)
		e.terms(p.typeParamTerms, x.params)
		e.b(x.paramsSet)
		e.b(x.filled)
	case tagTypeInterface:
		e.iface(index)
	case tagTypeParam:
		x := p.typeParams[index]
		e.term(x.owner)
		e.key(x.name)
		e.term(x.constraint)
		e.b(x.constraintFilled)
	case tagTypePrimitive:
		e.u(uint64(p.primitiveTypes[index].kind))
	case tagTypeLiteral:
		x := p.literalTypes[index]
		e.u(uint64(x.kind))
		e.key(x.exact)
		e.u(x.bits)
	case tagTypeOptional:
		e.term(p.optionalTypes[index].inner)
	case tagTypeUnion:
		e.terms(p.staticTypeTerms, p.unionTypes[index].terms)
	case tagTypeIntersection:
		e.terms(p.staticTypeTerms, p.intersectionTypes[index].terms)
	case tagTypeRef:
		e.typeRef(index)
	case tagTypeGeneric:
		x := p.genericTypes[index]
		e.term(x.base)
		e.terms(p.staticTypeTerms, x.args)
	case tagTypeArray:
		x := p.arrayTypes[index]
		e.term(x.element)
		e.b(x.readonly)
	case tagTypeMap:
		x := p.mapTypes[index]
		e.term(x.key)
		e.term(x.value)
		e.b(x.readonly)
	case tagTypeRecord:
		x := p.recordTypes[index]
		e.b(x.readonly)
		e.terms(p.typeFieldTerms, x.fields)
	case tagTypeField:
		x := p.typeFields[index]
		e.term(x.owner)
		e.key(x.key)
		e.term(x.typ)
		e.b(x.optional)
	case tagTypeFunction:
		e.signature(index)
	case tagTypeAsserts:
		x := p.assertions[index]
		e.key(x.name)
		e.i(int64(x.param))
		e.span(x.paramSpan)
		e.term(x.narrow)
	case tagDeclaredType:
		x := p.declaredTypes[index]
		e.term(x.host)
		e.term(x.target)
	case tagTypePublication:
		x := p.typePublications[index]
		e.term(x.assign)
		e.u(uint64(x.pair))
		e.term(x.target)
	case tagTypeValue:
		x := p.typeValues[index]
		e.term(x.owner)
		e.term(x.target)
	case tagValueClaim:
		x := p.valueClaims[index]
		e.term(x.owner)
		e.term(x.operand)
		e.term(x.target)
		e.u(uint64(x.kind))
	case tagAnnotation:
		x := p.annotations[index]
		e.term(x.scope)
		e.term(x.target)
		e.key(x.name)
		e.term(x.values)
	case tagTypeOf:
		x := p.typeOfs[index]
		e.term(x.scope)
		e.term(x.operand)
	case tagTypeKeyOf:
		e.term(p.keyOfTypes[index].inner)
	case tagTypeIndexAccess:
		x := p.indexAccessTypes[index]
		e.term(x.object)
		e.term(x.index)
	case tagTypeConditional:
		x := p.conditionalTypes[index]
		e.term(x.check)
		e.term(x.extends)
		e.term(x.then)
		e.term(x.otherwise)
	case tagWrite:
		x := p.writes[index]
		e.term(x.assign)
		e.term(x.target)
	case tagTableField:
		x := p.tableFields[index]
		e.term(x.table)
		e.term(x.key)
		e.term(x.values)
		e.u(uint64(x.kind))
	case tagControlFault:
		x := p.controlFaults[index]
		e.term(x.owner)
		e.u(uint64(x.kind))
		e.term(x.label)
		e.term(x.blocker)
	case tagImport:
		x := p.imports[index]
		e.term(x.call)
		e.term(x.alias)
	default:
		e.err = errors.New("program artifact: missing authored codec")
	}
}

func (e *artifactEncoder) cell(index int) {
	x := e.p.cells[index]
	if ordinal, global := decodeGlobalCellOrdinal(x.storage); global {
		if int(ordinal) >= len(e.p.globalKeys) {
			e.err = errors.New("program artifact: invalid global Cell")
			return
		}
		e.b(true)
		e.key(e.p.globalKeys[ordinal])
		return
	}
	e.b(false)
	e.term(x.storage)
}

func (e *artifactEncoder) function(index int) {
	x := e.p.functions[index]
	e.term(x.owner)
	e.term(x.body)
	e.term(x.vararg)
	e.terms(e.p.formalTerms, x.formals)
	e.captures(x.captures)
	e.terms(e.p.typeParamTerms, x.typeParams)
	e.b(x.typeParamsSet)
	e.b(x.returnsKnown)
	e.terms(e.p.staticTypeTerms, x.returns)
	e.b(x.returnsSet)
}

func (e *artifactEncoder) loop(index int) {
	p := e.p
	if index >= len(p.loopBodies) || index >= len(p.loopControls) || index >= len(p.loopKinds) || index >= len(p.loopCellRanges) {
		e.err = errors.New("program artifact: malformed Loop")
		return
	}
	e.term(p.loopOwners[index])
	e.term(p.loopBodies[index])
	e.term(p.loopControls[index])
	e.u(uint64(p.loopKinds[index]))
	e.terms(p.loopCells, p.loopCellRanges[index])
}

func (e *artifactEncoder) iface(index int) {
	p, x := e.p, e.p.interfaces[index]
	e.term(x.owner)
	e.key(x.name)
	e.span(x.nameSpan)
	e.terms(p.staticTypeTerms, x.extends)
	if x.members.start > x.members.end || int(x.members.end) > len(p.interfaceMembers) {
		e.err = errors.New("program artifact: malformed interface")
		return
	}
	e.n(uint64(x.members.end - x.members.start))
	for _, member := range p.interfaceMembers[x.members.start:x.members.end] {
		e.u(uint64(member.kind))
		e.term(member.field)
		e.key(member.name)
		e.span(member.nameSpan)
		e.term(member.signature)
	}
}

func (e *artifactEncoder) typeRef(index int) {
	x := e.p.typeRefs[index]
	e.u(uint64(x.resolution))
	e.term(x.target)
	e.term(x.root)
	e.keys(e.p.typeRefSourceKeys, x.source)
	e.keys(e.p.typeRefResolutionKeys, x.canonical)
}

func (e *artifactEncoder) signature(index int) {
	p, x := e.p, e.p.signatures[index]
	e.term(x.scope)
	e.terms(p.typeParamTerms, x.typeParams)
	e.b(x.typeParamsSet)
	if x.params.start > x.params.end || int(x.params.end) > len(p.signatureParams) {
		e.err = errors.New("program artifact: malformed signature")
		return
	}
	e.n(uint64(x.params.end - x.params.start))
	for _, parameter := range p.signatureParams[x.params.start:x.params.end] {
		e.key(parameter.name)
		e.span(parameter.nameSpan)
		e.term(parameter.typ)
	}
	e.term(x.variadic)
	e.span(x.variadicSpan)
	e.b(x.returnsKnown)
	e.terms(p.staticTypeTerms, x.returns)
	e.b(x.filled)
}

func (e *artifactEncoder) span(x storedSpan) {
	e.u(uint64(x.startLine))
	e.u(uint64(x.startCol))
	e.u(uint64(x.endLine))
	e.u(uint64(x.endCol))
}
func (e *artifactEncoder) i(v int64) { e.u(uint64(v<<1) ^ uint64(v>>63)) }
func (e *artifactEncoder) term(v Term) {
	if v == 0 {
		e.u(0)
		return
	}
	if !e.p.Valid(v) {
		e.err = errors.New("program artifact: invalid Term reference")
		return
	}
	e.u(uint64(v.tag()))
	e.u(uint64(v.index()))
}
func (e *artifactEncoder) terms(pool []Term, r termRange) {
	if r.start > r.end || int(r.end) > len(pool) {
		e.err = errors.New("program artifact: invalid Term range")
		return
	}
	e.n(uint64(r.end - r.start))
	for _, term := range pool[r.start:r.end] {
		e.term(term)
	}
}
func (e *artifactEncoder) termsByTerm(tag uint8, r termRange) {
	if r.start > r.end {
		e.err = errors.New("program artifact: invalid derived-free row range")
		return
	}
	e.n(uint64(r.end - r.start))
	for index := r.start; index < r.end; index++ {
		e.term(makeTerm(tag, index))
	}
}
func (e *artifactEncoder) captures(r captureRange) {
	if r.start > r.end || int(r.end) > len(e.p.captures) {
		e.err = errors.New("program artifact: invalid captures")
		return
	}
	e.n(uint64(r.end - r.start))
	for _, capture := range e.p.captures[r.start:r.end] {
		e.term(capture.inner)
		e.term(capture.outer)
	}
}
func (e *artifactEncoder) keys(pool []Key, r keyRange) {
	if r.start > r.end || int(r.end) > len(pool) {
		e.err = errors.New("program artifact: invalid Key range")
		return
	}
	e.n(uint64(r.end - r.start))
	for _, key := range pool[r.start:r.end] {
		e.key(key)
	}
}
func (e *artifactEncoder) key(key Key) {
	if key == 0 {
		e.u(0)
		return
	}
	if uint64(key) > uint64(len(e.p.exactKeys)) {
		e.err = errors.New("program artifact: invalid Key")
		return
	}
	x := e.p.exactKeys[key-1]
	e.u(uint64(x.kind))
	switch x.kind {
	case exactBool:
		e.b(x.bool)
	case exactInteger:
		e.i(x.int)
	case exactFloat:
		e.u(x.bits)
	case exactString:
		e.s(x.text)
	default:
		e.err = errors.New("program artifact: invalid exact Key")
	}
}
