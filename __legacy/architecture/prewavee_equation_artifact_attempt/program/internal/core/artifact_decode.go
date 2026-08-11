package core

import (
	"fmt"
	"math"

	"github.com/wippyai/go-lua/program/internal/canonical"
)

func decodeArtifact(data []byte, target ContentID) (*Program, ArtifactEnvelope, error) {
	measure, err := canonical.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	if !artifactMeasureAllowed(measure) {
		return nil, ArtifactEnvelope{}, ErrArtifactLimit
	}
	r, err := canonical.NewReader(data, artifactMaxBytes)
	if err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	if err := r.Header(artifactCodecDomain, artifactCodecVersion); err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	d := artifactDecoder{r: r, b: &Builder{}, stringBytes: measure.StringBytes}
	envelope, claimed, err := d.root(target)
	if err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	if err := r.Finish(); err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	if err := d.finish(); err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	p, err := d.b.Seal()
	if err != nil {
		return nil, ArtifactEnvelope{}, fmt.Errorf("program artifact: authored replay: %w", err)
	}
	if envelope.Equations != nil && !validateArtifactEquationCache(p, *envelope.Equations) {
		return nil, ArtifactEnvelope{}, ErrArtifactCanonical
	}
	if p.ContentID() != claimed {
		return nil, ArtifactEnvelope{}, fmt.Errorf("%w: replay ContentID differs", ErrArtifactCanonical)
	}
	return p, envelope, nil
}

type artifactDecoder struct {
	r              *canonical.Reader
	b              *Builder
	counts         [tagCount]uint32
	stringBytes    uint64
	equationBudget artifactEquationBudget
}

func (d *artifactDecoder) string(limit int) (string, error) {
	payload, err := d.r.StringBytes(limit)
	if err != nil {
		return "", err
	}
	if uint64(len(payload)) > d.stringBytes {
		return "", ErrArtifactLimit
	}
	d.stringBytes -= uint64(len(payload))
	return string(payload), nil
}

func (d *artifactDecoder) root(target ContentID) (ArtifactEnvelope, ContentID, error) {
	if err := d.record(artifactRecordRoot); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	storedTarget, err := d.id()
	if err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	if storedTarget != target {
		return ArtifactEnvelope{}, ContentID{}, ErrArtifactTarget
	}
	claimed, err := d.id()
	if err != nil || !claimed.Available() {
		return ArtifactEnvelope{}, ContentID{}, ErrArtifactCanonical
	}
	provenance, err := d.string(artifactMaxBytes)
	if err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	count, err := d.countAtLeast(artifactDependencyWireMin)
	if err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	envelope := ArtifactEnvelope{Target: storedTarget, Provenance: provenance}
	for index := uint64(0); index < count; index++ {
		if err := d.record(artifactRecordDependency); err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		name, err := d.string(artifactMaxBytes)
		if err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		id, err := d.id()
		if err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		envelope.Dependencies = append(envelope.Dependencies, ArtifactDependency{Name: name, ID: id})
	}
	if err := validateArtifactEnvelope(envelope); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	if err := d.record(artifactRecordProgram); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	name, err := d.string(artifactMaxBytes)
	if err != nil || name == "" {
		return ArtifactEnvelope{}, ContentID{}, ErrArtifactCanonical
	}
	d.b.sourceName = name
	if d.b.entry, err = d.term(); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	if d.b.chunkVararg, err = d.term(); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	for tag := uint8(1); tag < tagCount; tag++ {
		got, err := d.r.Uint()
		if err != nil || got != uint64(tag) {
			return ArtifactEnvelope{}, ContentID{}, ErrArtifactCanonical
		}
		count, err := d.countAtLeast(artifactRowWireMin)
		if err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		if tag == tagOutcome && count != 0 {
			return ArtifactEnvelope{}, ContentID{}, ErrArtifactCanonical
		}
		d.counts[tag] = uint32(count)
		for index := uint32(1); index <= uint32(count); index++ {
			if err := d.row(tag, index); err != nil {
				return ArtifactEnvelope{}, ContentID{}, err
			}
		}
	}
	if err := d.record(artifactRecordImplicitReads); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	implicit, err := d.countAtLeast(artifactTermWireMin)
	if err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	for index := uint64(0); index < implicit; index++ {
		read, err := d.term()
		if err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		if read.tag() != tagRead {
			return ArtifactEnvelope{}, ContentID{}, ErrArtifactCanonical
		}
		d.b.implicitReads = append(d.b.implicitReads, read)
	}
	if err := d.metamethodCandidates(); err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	present, err := d.r.Bool()
	if err != nil {
		return ArtifactEnvelope{}, ContentID{}, err
	}
	if present {
		cache, err := d.equationCache()
		if err != nil {
			return ArtifactEnvelope{}, ContentID{}, err
		}
		envelope.Equations = &cache
	}
	return envelope, claimed, nil
}

func (d *artifactDecoder) metamethodCandidates() error {
	if err := d.record(artifactRecordMetamethodCandidates); err != nil {
		return err
	}
	for _, family := range []struct {
		destination *[]candidateSourceRow
		tag         uint8
		maximum     int
	}{
		{&d.b.unaryNumeric, tagUnary, len(d.b.unaries)},
		{&d.b.lengths, tagUnary, len(d.b.unaries)},
		{&d.b.arithmetic, tagBinary, len(d.b.binaries)},
		{&d.b.bitwise, tagBinary, len(d.b.binaries)},
		{&d.b.concat, tagBinary, len(d.b.binaries)},
		{&d.b.equality, tagBinary, len(d.b.binaries)},
		{&d.b.order, tagBinary, len(d.b.binaries)},
		{&d.b.indexGet, tagRead, len(d.b.reads)},
		{&d.b.indexSet, tagWrite, len(d.b.writes)},
		{&d.b.callable, tagCall, len(d.b.calls)},
	} {
		if err := d.candidateSources(family.destination, family.tag, family.maximum); err != nil {
			return err
		}
	}
	return nil
}

func (d *artifactDecoder) candidateSources(destination *[]candidateSourceRow, tag uint8, maximum int) error {
	count, err := d.countAtLeast(artifactTermWireMin)
	if err != nil {
		return err
	}
	if maximum < 0 || count > uint64(maximum) || count > uint64(math.MaxInt-len(*destination)) {
		return ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		source, err := d.term()
		if err != nil {
			return err
		}
		if source.tag() != tag {
			return ErrArtifactCanonical
		}
		*destination = append(*destination, candidateSourceRow{source: source})
	}
	return nil
}

func (d *artifactDecoder) semantic() (ArtifactSemanticKey, error) {
	id, err := d.id()
	if err != nil {
		return ArtifactSemanticKey{}, err
	}
	version, err := d.r.Uint()
	if err != nil {
		return ArtifactSemanticKey{}, err
	}
	key := ArtifactSemanticKey{ID: id, Version: version}
	if !validArtifactSemanticKey(key) {
		return ArtifactSemanticKey{}, ErrArtifactCanonical
	}
	return key, nil
}

func (d *artifactDecoder) semantics() ([]ArtifactSemanticKey, error) {
	count, err := d.countAtLeast(34)
	if err != nil {
		return nil, err
	}
	capacity, err := d.equationCapacity(count, artifactEquationSemanticBytes)
	if err != nil {
		return nil, err
	}
	result := make([]ArtifactSemanticKey, 0, capacity)
	for index := uint64(0); index < count; index++ {
		key, err := d.semantic()
		if err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	if !orderedArtifactSemanticKeys(result) {
		return nil, ErrArtifactCanonical
	}
	return result, nil
}

func (d *artifactDecoder) semanticSchema() ([]ArtifactSemanticKey, error) {
	count, err := d.countAtLeast(34)
	if err != nil {
		return nil, err
	}
	capacity, err := d.equationCapacity(count, artifactEquationSemanticBytes)
	if err != nil {
		return nil, err
	}
	result := make([]ArtifactSemanticKey, 0, capacity)
	for index := uint64(0); index < count; index++ {
		key, err := d.semantic()
		if err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	return result, nil
}

func (d *artifactDecoder) equationCache() (ArtifactEquationCache, error) {
	if err := d.record(artifactRecordEquationCache); err != nil {
		return ArtifactEquationCache{}, err
	}
	programID, err := d.id()
	if err != nil || !programID.Available() {
		return ArtifactEquationCache{}, ErrArtifactCanonical
	}
	moduleID, err := d.id()
	if err != nil || !moduleID.Available() {
		return ArtifactEquationCache{}, ErrArtifactCanonical
	}
	engine, err := d.semantic()
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	factors, err := d.semanticSchema()
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	rules, err := d.semantics()
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	if _, err := d.equationCapacity(uint64(len(factors)), artifactEquationSetEntryBytes); err != nil {
		return ArtifactEquationCache{}, err
	}
	if _, err := d.equationCapacity(uint64(len(rules)), artifactEquationSetEntryBytes); err != nil {
		return ArtifactEquationCache{}, err
	}
	bodyCount, err := d.countAtLeast(13)
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	bodyCapacity, err := d.equationCapacity(bodyCount, artifactEquationBodyBytes)
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	cache := ArtifactEquationCache{Program: programID, Module: moduleID, Engine: engine, Factors: factors, Rules: rules, Bodies: make([]ArtifactEquationBody, 0, bodyCapacity)}
	for index := uint64(0); index < bodyCount; index++ {
		if err := d.record(artifactRecordEquationBody); err != nil {
			return ArtifactEquationCache{}, err
		}
		body, err := d.term()
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		termCount, err := d.countAtLeast(artifactTermWireMin)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		termCapacity, err := d.equationCapacity(termCount, artifactEquationTermBytes)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		row := ArtifactEquationBody{Body: body, Terms: make([]Term, 0, termCapacity)}
		for termIndex := uint64(0); termIndex < termCount; termIndex++ {
			term, err := d.term()
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			row.Terms = append(row.Terms, term)
		}
		edgeCount, err := d.countAtLeast(15)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		edgeCapacity, err := d.equationCapacity(edgeCount, artifactEquationEdgeBytes)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		row.Edges = make([]ArtifactEquationEdge, 0, edgeCapacity)
		for edgeIndex := uint64(0); edgeIndex < edgeCount; edgeIndex++ {
			if err := d.record(artifactRecordEquationEdge); err != nil {
				return ArtifactEquationCache{}, err
			}
			edge := ArtifactEquationEdge{}
			if edge.From, err = d.term(); err != nil {
				return ArtifactEquationCache{}, err
			}
			if edge.To, err = d.term(); err != nil {
				return ArtifactEquationCache{}, err
			}
			if edge.Decision, err = d.term(); err != nil {
				return ArtifactEquationCache{}, err
			}
			if edge.Truthy, err = d.r.Bool(); err != nil {
				return ArtifactEquationCache{}, err
			}
			if edge.Mu, err = d.term(); err != nil {
				return ArtifactEquationCache{}, err
			}
			decisionCount, err := d.countAtLeast(artifactTermWireMin)
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			decisionCapacity, err := d.equationCapacity(decisionCount, artifactEquationTermBytes)
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			edge.MuDecisions = make([]Term, 0, decisionCapacity)
			for decisionIndex := uint64(0); decisionIndex < decisionCount; decisionIndex++ {
				decision, err := d.term()
				if err != nil {
					return ArtifactEquationCache{}, err
				}
				edge.MuDecisions = append(edge.MuDecisions, decision)
			}
			row.Edges = append(row.Edges, edge)
		}
		cache.Bodies = append(cache.Bodies, row)
	}
	boundaryCount, err := d.countAtLeast(105)
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	boundaryCapacity, err := d.equationCapacity(boundaryCount, artifactEquationBoundaryBytes)
	if err != nil {
		return ArtifactEquationCache{}, err
	}
	cache.Boundary = make([]ArtifactEquationBoundary, 0, boundaryCapacity)
	for index := uint64(0); index < boundaryCount; index++ {
		if err := d.record(artifactRecordEquationBoundary); err != nil {
			return ArtifactEquationCache{}, err
		}
		boundary := ArtifactEquationBoundary{}
		if boundary.Rule, err = d.semantic(); err != nil {
			return ArtifactEquationCache{}, err
		}
		if boundary.Output, err = d.semantic(); err != nil {
			return ArtifactEquationCache{}, err
		}
		if boundary.Activation, err = d.term(); err != nil {
			return ArtifactEquationCache{}, err
		}
		if boundary.At, err = d.term(); err != nil {
			return ArtifactEquationCache{}, err
		}
		inputArity, err := d.r.Uint()
		if err != nil || inputArity > uint64(math.MaxInt) || inputArity == 0 {
			return ArtifactEquationCache{}, ErrArtifactCanonical
		}
		boundary.InputArity = int(inputArity)
		readCount, err := d.countAtLeast(artifactEquationReadWireMin)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		readCapacity, err := d.equationCapacity(readCount, artifactEquationReadBytes)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		boundary.Reads = make([]ArtifactEquationRead, 0, readCapacity)
		for readIndex := uint64(0); readIndex < readCount; readIndex++ {
			position, err := d.r.Uint()
			if err != nil || position > uint64(math.MaxInt) {
				return ArtifactEquationCache{}, ErrArtifactCanonical
			}
			factor, err := d.semantic()
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			exact, err := d.r.Bool()
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			var key uint64
			if exact {
				key, err = d.r.Uint()
				if err != nil {
					return ArtifactEquationCache{}, err
				}
			}
			boundary.Reads = append(boundary.Reads, ArtifactEquationRead{Position: int(position), Factor: factor, Exact: exact, Key: key})
		}
		writeCount, err := d.countAtLeast(1)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		writeCapacity, err := d.equationCapacity(writeCount, artifactEquationWriteBytes)
		if err != nil {
			return ArtifactEquationCache{}, err
		}
		boundary.Writes = make([]uint64, 0, writeCapacity)
		for writeIndex := uint64(0); writeIndex < writeCount; writeIndex++ {
			key, err := d.r.Uint()
			if err != nil {
				return ArtifactEquationCache{}, err
			}
			boundary.Writes = append(boundary.Writes, key)
		}
		cache.Boundary = append(cache.Boundary, boundary)
	}
	return cache, nil
}

func (d *artifactDecoder) row(tag uint8, index uint32) error {
	if err := d.record(artifactRecordTerm); err != nil {
		return err
	}
	storedTag, err := d.r.Uint()
	if err != nil || storedTag != uint64(tag) {
		return ErrArtifactCanonical
	}
	storedIndex, err := d.r.Uint()
	if err != nil || storedIndex != uint64(index) {
		return ErrArtifactCanonical
	}
	span, err := d.span()
	if err != nil {
		return err
	}
	d.b.spans[tag] = append(d.b.spans[tag], span)
	var term Term
	switch tag {
	case tagNil:
		if term, err = d.term(); err == nil {
			d.b.nils = append(d.b.nils, term)
		}
	case tagBool:
		var owner Term
		var value bool
		owner, err = d.term()
		if err == nil {
			value, err = d.r.Bool()
		}
		if err == nil {
			d.b.bools = append(d.b.bools, boolRow{owner: owner, value: value})
		}
	case tagInteger:
		var owner Term
		var value int64
		owner, err = d.term()
		if err == nil {
			value, err = d.integer()
		}
		if err == nil {
			d.b.integers = append(d.b.integers, integerRow{owner: owner, value: value})
		}
	case tagFloat:
		var owner Term
		var bits uint64
		owner, err = d.term()
		if err == nil {
			bits, err = d.r.Uint()
		}
		if err == nil {
			d.b.floats = append(d.b.floats, floatRow{owner: owner, bits: bits})
		}
	case tagString:
		var owner Term
		var value string
		owner, err = d.term()
		if err == nil {
			value, err = d.string(artifactMaxBytes)
		}
		if err == nil {
			d.b.strings = append(d.b.strings, stringRow{owner: owner, value: value})
		}
	case tagValues:
		var owner, tail Term
		var fixed termRange
		owner, err = d.term()
		if err == nil {
			fixed, err = d.terms(&d.b.valueTerms)
		}
		if err == nil {
			tail, err = d.term()
		}
		if err == nil {
			d.b.values = append(d.b.values, valuesRow{owner: owner, fixed: fixed, tail: tail})
		}
	case tagLensExact:
		var owner, base, source Term
		var kind uint64
		owner, err = d.term()
		if err == nil {
			kind, err = d.r.Uint()
		}
		if err == nil {
			base, err = d.term()
		}
		if err == nil {
			source, err = d.term()
		}
		if err == nil {
			d.b.lensExact = append(d.b.lensExact, exactLensRow{owner: owner, kind: FieldKind(kind), base: base, source: source})
		}
	case tagLensKey:
		var owner, base, key Term
		owner, err = d.term()
		if err == nil {
			base, err = d.term()
		}
		if err == nil {
			key, err = d.term()
		}
		if err == nil {
			d.b.lensKeys = append(d.b.lensKeys, keyLensRow{owner: owner, base: base, key: key})
		}
	case tagReturn:
		var owner, values Term
		owner, err = d.term()
		if err == nil {
			values, err = d.term()
		}
		if err == nil {
			d.b.returns = append(d.b.returns, returnRow{owner: owner, values: values})
		}
	case tagBreak:
		if term, err = d.term(); err == nil {
			d.b.breaks = append(d.b.breaks, breakRow{owner: term})
		}
	case tagLabel:
		if term, err = d.term(); err == nil {
			d.b.labelOwners = append(d.b.labelOwners, term)
			d.b.labelResumes = append(d.b.labelResumes, 0)
		}
	case tagGoto:
		var owner, target Term
		owner, err = d.term()
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			d.b.gotoOwners = append(d.b.gotoOwners, owner)
			d.b.gotoTargets = append(d.b.gotoTargets, target)
			d.b.gotoExits = append(d.b.gotoExits, 0)
		}
	case tagBody:
		var source termRange
		source, err = d.terms(&d.b.sourceTerms)
		if err == nil {
			d.b.bodies = append(d.b.bodies, bodyRow{source: source, filled: true})
		}
	case tagCell:
		err = d.cell(index)
	case tagRead:
		var owner, source Term
		owner, err = d.term()
		if err == nil {
			source, err = d.term()
		}
		if err == nil {
			d.b.reads = append(d.b.reads, readRow{owner: owner, source: source})
		}
	case tagVararg:
		var owner, cell Term
		owner, err = d.term()
		if err == nil {
			cell, err = d.term()
		}
		if err == nil {
			d.b.varargs = append(d.b.varargs, varargRow{owner: owner, cell: cell})
		}
	case tagUnary:
		var owner, operand Term
		var op uint64
		owner, err = d.term()
		if err == nil {
			op, err = d.r.Uint()
		}
		if err == nil {
			operand, err = d.term()
		}
		if err == nil {
			d.b.unaries = append(d.b.unaries, unaryRow{owner: owner, op: UnaryOp(op), operand: operand})
		}
	case tagBinary:
		var owner, left, right Term
		var op uint64
		owner, err = d.term()
		if err == nil {
			op, err = d.r.Uint()
		}
		if err == nil {
			left, err = d.term()
		}
		if err == nil {
			right, err = d.term()
		}
		if err == nil {
			d.b.binaries = append(d.b.binaries, binaryRow{owner: owner, op: BinaryOp(op), left: left, right: right})
		}
	case tagSelect:
		var owner, left, right Term
		var op uint64
		owner, err = d.term()
		if err == nil {
			op, err = d.r.Uint()
		}
		if err == nil {
			left, err = d.term()
		}
		if err == nil {
			right, err = d.term()
		}
		if err == nil {
			d.b.selects = append(d.b.selects, selectRow{owner: owner, op: SelectOp(op), left: left, right: right})
		}
	case tagBind:
		var owner, values Term
		var cells termRange
		owner, err = d.term()
		if err == nil {
			cells, err = d.terms(&d.b.bindTerms)
		}
		if err == nil {
			values, err = d.term()
		}
		if err == nil {
			d.b.binds = append(d.b.binds, bindRow{owner: owner, cells: cells, values: values})
		}
	case tagAssign:
		var owner, values Term
		var writes termRange
		owner, err = d.term()
		if err == nil {
			writes, err = d.termsByTag(tagWrite)
		}
		if err == nil {
			values, err = d.term()
		}
		if err == nil {
			d.b.assigns = append(d.b.assigns, assignRow{owner: owner, writes: writes, values: values})
		}
	case tagFunction:
		err = d.function()
	case tagCall:
		var owner, callee, receiver, actuals Term
		var args termRange
		owner, err = d.term()
		if err == nil {
			callee, err = d.term()
		}
		if err == nil {
			receiver, err = d.term()
		}
		if err == nil {
			actuals, err = d.term()
		}
		if err == nil {
			args, err = d.terms(&d.b.callTypeArgs)
		}
		if err == nil {
			d.b.calls = append(d.b.calls, callRow{owner: owner, callee: callee, receiver: receiver, actuals: actuals, typeArgs: args})
		}
	case tagBranch:
		var owner, condition, whenTrue, whenFalse Term
		owner, err = d.term()
		if err == nil {
			condition, err = d.term()
		}
		if err == nil {
			whenTrue, err = d.term()
		}
		if err == nil {
			whenFalse, err = d.term()
		}
		if err == nil {
			d.b.branches = append(d.b.branches, branchRow{owner: owner, condition: condition, whenTrue: whenTrue, whenFalse: whenFalse})
		}
	case tagLoop:
		err = d.loop()
	case tagTable:
		var owner Term
		var fields termRange
		owner, err = d.term()
		if err == nil {
			fields, err = d.terms(&d.b.tableFieldTerms)
		}
		if err == nil {
			d.b.tables = append(d.b.tables, tableRow{owner: owner, fields: fields})
		}
	case tagKey:
		var owner Term
		var kind uint64
		var key Key
		owner, err = d.term()
		if err == nil {
			kind, err = d.r.Uint()
		}
		if err == nil {
			key, err = d.key()
		}
		if err == nil {
			d.b.keys = append(d.b.keys, keyRow{owner: owner, kind: FieldKind(kind), exact: key})
		}
	case tagTypeAlias:
		err = d.alias()
	case tagTypeInterface:
		err = d.iface()
	case tagTypeParam:
		var owner, constraint Term
		var name Key
		var filled bool
		owner, err = d.term()
		if err == nil {
			name, err = d.key()
		}
		if err == nil {
			constraint, err = d.term()
		}
		if err == nil {
			filled, err = d.r.Bool()
		}
		if err == nil {
			d.b.typeParams = append(d.b.typeParams, typeParamRow{owner: owner, name: name, constraint: constraint, constraintFilled: filled})
		}
	case tagTypePrimitive:
		var kind uint64
		kind, err = d.r.Uint()
		if err == nil {
			d.b.primitiveTypes = append(d.b.primitiveTypes, primitiveTypeRow{kind: PrimitiveKind(kind)})
		}
	case tagTypeLiteral:
		var kind, bits uint64
		var key Key
		kind, err = d.r.Uint()
		if err == nil {
			key, err = d.key()
		}
		if err == nil {
			bits, err = d.r.Uint()
		}
		if err == nil {
			d.b.literalTypes = append(d.b.literalTypes, literalTypeRow{kind: LiteralKind(kind), exact: key, bits: bits})
		}
	case tagTypeOptional:
		if term, err = d.term(); err == nil {
			d.b.optionalTypes = append(d.b.optionalTypes, unaryTypeRow{inner: term})
		}
	case tagTypeUnion:
		var terms termRange
		terms, err = d.terms(&d.b.staticTypeTerms)
		if err == nil {
			d.b.unionTypes = append(d.b.unionTypes, termsTypeRow{terms: terms})
		}
	case tagTypeIntersection:
		var terms termRange
		terms, err = d.terms(&d.b.staticTypeTerms)
		if err == nil {
			d.b.intersectionTypes = append(d.b.intersectionTypes, termsTypeRow{terms: terms})
		}
	case tagTypeRef:
		err = d.typeRef()
	case tagTypeGeneric:
		var base Term
		var args termRange
		base, err = d.term()
		if err == nil {
			args, err = d.terms(&d.b.staticTypeTerms)
		}
		if err == nil {
			d.b.genericTypes = append(d.b.genericTypes, genericTypeRow{base: base, args: args})
		}
	case tagTypeArray:
		var element Term
		var readonly bool
		element, err = d.term()
		if err == nil {
			readonly, err = d.r.Bool()
		}
		if err == nil {
			d.b.arrayTypes = append(d.b.arrayTypes, arrayTypeRow{element: element, readonly: readonly})
		}
	case tagTypeMap:
		var key, value Term
		var readonly bool
		key, err = d.term()
		if err == nil {
			value, err = d.term()
		}
		if err == nil {
			readonly, err = d.r.Bool()
		}
		if err == nil {
			d.b.mapTypes = append(d.b.mapTypes, mapTypeRow{key: key, value: value, readonly: readonly})
		}
	case tagTypeRecord:
		var readonly bool
		var fields termRange
		readonly, err = d.r.Bool()
		if err == nil {
			fields, err = d.terms(&d.b.typeFieldTerms)
		}
		if err == nil {
			d.b.recordTypes = append(d.b.recordTypes, recordTypeRow{readonly: readonly, fields: fields})
		}
	case tagTypeField:
		var owner, typ Term
		var key Key
		var optional bool
		owner, err = d.term()
		if err == nil {
			key, err = d.key()
		}
		if err == nil {
			typ, err = d.term()
		}
		if err == nil {
			optional, err = d.r.Bool()
		}
		if err == nil {
			d.b.typeFields = append(d.b.typeFields, typeFieldRow{owner: owner, key: key, typ: typ, optional: optional})
		}
	case tagTypeFunction:
		err = d.signature()
	case tagTypeAsserts:
		var name Key
		var param int64
		var parameter storedSpan
		var narrow Term
		name, err = d.key()
		if err == nil {
			param, err = d.integer()
		}
		if err == nil {
			parameter, err = d.span()
		}
		if err == nil {
			narrow, err = d.term()
		}
		if err == nil && param >= math.MinInt32 && param <= math.MaxInt32 {
			d.b.assertions = append(d.b.assertions, assertionRow{name: name, param: int32(param), paramSpan: parameter, narrow: narrow})
		} else if err == nil {
			err = ErrArtifactCanonical
		}
	case tagDeclaredType:
		var host, target Term
		host, err = d.term()
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			d.b.declaredTypes = append(d.b.declaredTypes, declaredTypeRow{host: host, target: target})
		}
	case tagTypePublication:
		var assign, target Term
		var pair uint64
		assign, err = d.term()
		if err == nil {
			pair, err = d.r.Uint()
		}
		if err == nil {
			target, err = d.term()
		}
		if err == nil && pair <= math.MaxUint32 {
			d.b.typePublications = append(d.b.typePublications, typePublicationRow{assign: assign, pair: uint32(pair), target: target})
		} else if err == nil {
			err = ErrArtifactCanonical
		}
	case tagTypeValue:
		var owner, target Term
		owner, err = d.term()
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			d.b.typeValues = append(d.b.typeValues, typeValueRow{owner: owner, target: target})
		}
	case tagValueClaim:
		var owner, operand, target Term
		var kind uint64
		owner, err = d.term()
		if err == nil {
			operand, err = d.term()
		}
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			kind, err = d.r.Uint()
		}
		if err == nil {
			d.b.valueClaims = append(d.b.valueClaims, valueClaimRow{owner: owner, operand: operand, target: target, kind: ValueClaimKind(kind)})
		}
	case tagAnnotation:
		var scope, target, values Term
		var name Key
		scope, err = d.term()
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			name, err = d.key()
		}
		if err == nil {
			values, err = d.term()
		}
		if err == nil {
			d.b.annotations = append(d.b.annotations, annotationRow{scope: scope, target: target, name: name, values: values, filled: true})
		}
	case tagTypeOf:
		var scope, operand Term
		scope, err = d.term()
		if err == nil {
			operand, err = d.term()
		}
		if err == nil {
			d.b.typeOfs = append(d.b.typeOfs, typeOfRow{scope: scope, operand: operand})
		}
	case tagTypeKeyOf:
		if term, err = d.term(); err == nil {
			d.b.keyOfTypes = append(d.b.keyOfTypes, keyOfTypeRow{inner: term})
		}
	case tagTypeIndexAccess:
		var object, indexTerm Term
		object, err = d.term()
		if err == nil {
			indexTerm, err = d.term()
		}
		if err == nil {
			d.b.indexAccessTypes = append(d.b.indexAccessTypes, indexAccessTypeRow{object: object, index: indexTerm})
		}
	case tagTypeConditional:
		var check, extends, then, otherwise Term
		check, err = d.term()
		if err == nil {
			extends, err = d.term()
		}
		if err == nil {
			then, err = d.term()
		}
		if err == nil {
			otherwise, err = d.term()
		}
		if err == nil {
			d.b.conditionalTypes = append(d.b.conditionalTypes, conditionalTypeRow{check: check, extends: extends, then: then, otherwise: otherwise})
		}
	case tagWrite:
		var assign, target Term
		assign, err = d.term()
		if err == nil {
			target, err = d.term()
		}
		if err == nil {
			d.b.writes = append(d.b.writes, writeRow{assign: assign, target: target})
		}
	case tagTableField:
		var table, key, values Term
		var kind uint64
		table, err = d.term()
		if err == nil {
			key, err = d.term()
		}
		if err == nil {
			values, err = d.term()
		}
		if err == nil {
			kind, err = d.r.Uint()
		}
		if err == nil {
			d.b.tableFields = append(d.b.tableFields, tableFieldRow{table: table, key: key, values: values, kind: FieldKind(kind)})
		}
	case tagControlFault:
		var owner, label, blocker Term
		var kind uint64
		owner, err = d.term()
		if err == nil {
			kind, err = d.r.Uint()
		}
		if err == nil {
			label, err = d.term()
		}
		if err == nil {
			blocker, err = d.term()
		}
		if err == nil {
			d.b.controlFaults = append(d.b.controlFaults, controlFaultRow{owner: owner, kind: ControlFaultKind(kind), label: label, blocker: blocker})
		}
	case tagImport:
		var call, alias Term
		call, err = d.term()
		if err == nil {
			alias, err = d.term()
		}
		if err == nil {
			d.b.imports = append(d.b.imports, importRow{call: call, alias: alias})
		}
	default:
		return ErrArtifactCanonical
	}
	if err != nil {
		return err
	}
	return nil
}

func (d *artifactDecoder) record(want uint64) error {
	got, err := d.r.Record()
	if err != nil || got != want {
		return ErrArtifactCanonical
	}
	return nil
}
func (d *artifactDecoder) id() (ContentID, error) {
	payload, err := d.r.Bytes(len(ContentID{}))
	var id ContentID
	if err != nil || len(payload) != len(id) {
		return id, ErrArtifactCanonical
	}
	copy(id[:], payload)
	return id, nil
}

const (
	artifactTermWireMin         = 3
	artifactDependencyWireMin   = 40
	artifactEquationReadWireMin = 38
	artifactRowWireMin          = 21
)

func (d *artifactDecoder) count() (uint64, error) { return d.countAtLeast(1) }

func (d *artifactDecoder) countAtLeast(bytesPerItem uint64) (uint64, error) {
	if bytesPerItem == 0 {
		return 0, ErrArtifactCanonical
	}
	value, err := d.r.Count()
	if err != nil || value > uint64(indexMax) || value > uint64(d.r.Remaining())/bytesPerItem {
		return 0, ErrArtifactCanonical
	}
	return value, nil
}

// equationCapacity reserves one bounded reconstruction-object budget before
// every cache slice allocation.  Canonical framing makes each count finite,
// but a finite hostile count must still not turn into an enormous pre-make
// allocation.  This is deliberately independent of wire-byte accounting:
// nested term/edge/dependency vectors are separate heap objects.
func (d *artifactDecoder) equationCapacity(count, width uint64) (int, error) {
	if count > uint64(math.MaxInt) || !d.equationBudget.reserve(count, width) {
		return 0, ErrArtifactLimit
	}
	return int(count), nil
}

func (d *artifactDecoder) integer() (int64, error) {
	value, err := d.r.Uint()
	if err != nil {
		return 0, err
	}
	return int64(value>>1) ^ -int64(value&1), nil
}
func (d *artifactDecoder) term() (Term, error) {
	tag, err := d.r.Uint()
	if err != nil {
		return 0, err
	}
	if tag == 0 {
		return 0, nil
	}
	index, err := d.r.Uint()
	if err != nil || tag >= uint64(tagCount) || index == 0 || index > uint64(indexMax) {
		return 0, ErrArtifactCanonical
	}
	return makeTerm(uint8(tag), uint32(index)), nil
}
func (d *artifactDecoder) span() (storedSpan, error) {
	a, err := d.r.Uint()
	if err != nil {
		return storedSpan{}, err
	}
	b, err := d.r.Uint()
	if err != nil {
		return storedSpan{}, err
	}
	c, err := d.r.Uint()
	if err != nil {
		return storedSpan{}, err
	}
	e, err := d.r.Uint()
	if err != nil {
		return storedSpan{}, err
	}
	if a > math.MaxUint32 || b > math.MaxUint32 || c > math.MaxUint32 || e > math.MaxUint32 {
		return storedSpan{}, ErrArtifactCanonical
	}
	span := storedSpan{startLine: uint32(a), startCol: uint32(b), endLine: uint32(c), endCol: uint32(e)}
	if !validSpan(Span{StartLine: int(span.startLine), StartCol: int(span.startCol), EndLine: int(span.endLine), EndCol: int(span.endCol)}) {
		return storedSpan{}, ErrArtifactCanonical
	}
	return span, nil
}
func (d *artifactDecoder) key() (Key, error) {
	kind, err := d.r.Uint()
	if err != nil {
		return 0, err
	}
	if kind == 0 {
		return 0, nil
	}
	var exact exactKey
	exact.kind = uint8(kind)
	switch exact.kind {
	case exactBool:
		exact.bool, err = d.r.Bool()
	case exactInteger:
		exact.int, err = d.integer()
	case exactFloat:
		exact.bits, err = d.r.Uint()
	case exactString:
		exact.text, err = d.string(artifactMaxBytes)
	default:
		return 0, ErrArtifactCanonical
	}
	if err != nil {
		return 0, err
	}
	key := d.b.internExact(exact)
	if key == 0 {
		return 0, ErrArtifactCanonical
	}
	return key, nil
}
func (d *artifactDecoder) terms(pool *[]Term) (termRange, error) {
	count, err := d.countAtLeast(artifactTermWireMin)
	if err != nil {
		return termRange{}, err
	}
	start, end, ok := boundedRange(len(*pool), int(count))
	if !ok {
		return termRange{}, ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		term, err := d.term()
		if err != nil {
			return termRange{}, err
		}
		*pool = append(*pool, term)
	}
	return termRange{start: start, end: end}, nil
}
func (d *artifactDecoder) termsByTag(tag uint8) (termRange, error) {
	count, err := d.countAtLeast(artifactTermWireMin * 2)
	if err != nil || count == 0 || count > uint64(indexMax) {
		return termRange{}, ErrArtifactCanonical
	}
	first, err := d.term()
	if err != nil || first.tag() != tag || first.index() == 0 || uint64(first.index())+count > uint64(indexMax)+1 {
		return termRange{}, ErrArtifactCanonical
	}
	for offset := uint64(1); offset < count; offset++ {
		term, err := d.term()
		if err != nil || term != makeTerm(tag, first.index()+uint32(offset)) {
			return termRange{}, ErrArtifactCanonical
		}
	}
	return termRange{start: first.index(), end: first.index() + uint32(count)}, nil
}
