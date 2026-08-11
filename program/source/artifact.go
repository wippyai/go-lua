package source

// This file is the Source-owned payload codec used by Program artifacts. It
// intentionally imports only the canonical framing primitive and keyspace:
// the Source child is responsible for decoding its own authored Input, while
// the parent artifact codec owns the surrounding envelope and seal replay.

import (
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	sourceArtifactRecordIdentity uint64 = iota + 1
	sourceArtifactRecordSpans
	sourceArtifactRecordLiterals
	sourceArtifactRecordOrder
	sourceArtifactRecordKeys
)

// WriteArtifactSection writes only the Source-authored payload. The caller
// owns the enclosing artifact stream, including its Header and Finish; this
// method therefore never resets or finishes the supplied Writer. The direct
// Source View is the only accepted owner capability; an unavailable view
// fails closed before any payload frame is emitted.
func WriteArtifactSection(writer *canonical.Writer, view View) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	authority := liveAuthority(view.authority, nil)
	if authority == nil || !authority.content.Available() {
		return canonical.ErrMalformed
	}
	return writeAuthoredPayload(writer, authority)
}

// ReadArtifactSection consumes exactly one Source-authored payload and leaves
// any following parent-artifact events unread. It returns construction Input,
// not a Component: the parent performs the ordinary sibling assembly and
// Source Commit, which is where derived Outcomes and all seal indexes belong.
// The decoder deliberately never calls Build.
func ReadArtifactSection(reader *canonical.Reader) (Input, error) {
	if reader == nil {
		return Input{}, canonical.ErrMalformed
	}

	if err := sourceRecord(reader, sourceArtifactRecordIdentity); err != nil {
		return Input{}, err
	}
	nameBytes, err := sourceStringBytes(reader)
	if err != nil {
		return Input{}, err
	}
	if len(nameBytes) == 0 {
		return Input{}, canonical.ErrMalformed
	}
	authoredTermCount, err := reader.Count()
	if err != nil {
		return Input{}, err
	}
	if authoredTermCount == 0 || authoredTermCount > uint64(^uint32(0)) {
		return Input{}, canonical.ErrLimit
	}

	// Probe the complete payload on a value copy before copying the source
	// name or reserving any authored collection. A malformed final key/fault
	// row must not leave earlier spans, literals, or order pools allocated.
	probe := *reader
	if err := sourceRecord(&probe, sourceArtifactRecordSpans); err != nil {
		return Input{}, err
	}
	var counts [keyspace.FamilyCount]uint32
	if err := preflightSourceSpans(&probe, &counts); err != nil {
		return Input{}, err
	}
	var computedTermCount uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		computedTermCount += uint64(counts[family])
	}
	if computedTermCount == 0 || computedTermCount != authoredTermCount || computedTermCount > uint64(^uint32(0)) {
		return Input{}, canonical.ErrMalformed
	}

	if err := sourceRecord(&probe, sourceArtifactRecordLiterals); err != nil {
		return Input{}, err
	}
	if err := preflightSourceLiterals(&probe, counts); err != nil {
		return Input{}, err
	}

	if err := sourceRecord(&probe, sourceArtifactRecordOrder); err != nil {
		return Input{}, err
	}
	faultOwners, err := preflightSourceOrder(&probe, counts)
	if err != nil {
		return Input{}, err
	}

	if err := sourceRecord(&probe, sourceArtifactRecordKeys); err != nil {
		return Input{}, err
	}
	if err := preflightSourceKeys(&probe, counts, faultOwners); err != nil {
		return Input{}, err
	}

	// The copied-reader proof above is complete. Only now may the real pass
	// copy the filename and allocate the Input-owned authored collections.
	name := string(nameBytes)
	if err := sourceRecord(reader, sourceArtifactRecordSpans); err != nil {
		return Input{}, err
	}
	decodedCounts, families, err := readSourceSpans(reader, name)
	if err != nil {
		return Input{}, err
	}
	if decodedCounts != counts {
		return Input{}, canonical.ErrMalformed
	}
	input := Input{Name: name, Families: families}
	if err := sourceRecord(reader, sourceArtifactRecordLiterals); err != nil {
		return Input{}, err
	}
	if err := readSourceLiterals(reader, &input, counts); err != nil {
		return Input{}, err
	}

	if err := sourceRecord(reader, sourceArtifactRecordOrder); err != nil {
		return Input{}, err
	}
	faultOwners, err = readSourceOrder(reader, &input, counts)
	if err != nil {
		return Input{}, err
	}

	if err := sourceRecord(reader, sourceArtifactRecordKeys); err != nil {
		return Input{}, err
	}
	if err := readSourceKeys(reader, &input, counts, faultOwners); err != nil {
		return Input{}, err
	}
	return input, nil
}

func sourceRecord(reader *canonical.Reader, want uint64) error {
	got, err := reader.Record()
	if err != nil {
		return err
	}
	if got != want {
		return canonical.ErrMalformed
	}
	return nil
}

// sourceCount checks an untrusted Count before any Go slice capacity is
// derived from it. minimumBytes is a conservative lower bound for one row's
// remaining canonical frames; using Remaining catches impossible arities even
// when a caller gives the Reader a very large external limit.
func sourceCount(reader *canonical.Reader, maximum uint64, minimumBytes int) (int, error) {
	value, err := reader.Count()
	if err != nil {
		return 0, err
	}
	if value > maximum || value > sourceMaxInt() {
		return 0, canonical.ErrLimit
	}
	if minimumBytes > 0 && value > uint64(reader.Remaining()/minimumBytes) {
		return 0, canonical.ErrLimit
	}
	return int(value), nil
}

func sourceMaxInt() uint64 { return uint64(^uint(0) >> 1) }

func sourceString(reader *canonical.Reader) (string, error) {
	value, err := sourceStringBytes(reader)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func sourceStringBytes(reader *canonical.Reader) ([]byte, error) {
	limit := reader.Remaining()
	if uint64(limit) > sourceMaxInt() {
		limit = int(sourceMaxInt())
	}
	return reader.StringBytes(limit)
}

func readSourceSpans(reader *canonical.Reader, name string) ([keyspace.FamilyCount]uint32, []FamilySpans, error) {
	var counts [keyspace.FamilyCount]uint32
	families := make([]FamilySpans, 0, int(keyspace.FamilyCount-1))
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			// Outcome is the one derived family. Its empty authored row is
			// restored in Input at its canonical family position below.
			families = append(families, FamilySpans{Family: family})
			continue
		}
		tag, err := reader.Uint()
		if err != nil {
			return counts, nil, err
		}
		if tag != uint64(family) {
			return counts, nil, canonical.ErrMalformed
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
		if err != nil {
			return counts, nil, err
		}
		counts[family] = uint32(count)
		spans := make([]Span, count)
		for index := range spans {
			startLine, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			startCol, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			endLine, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			endCol, err := sourceUint32(reader)
			if err != nil {
				return counts, nil, err
			}
			if !validSourceCoordinate(startLine, startCol, endLine, endCol) {
				return counts, nil, canonical.ErrMalformed
			}
			spans[index] = Span{
				File: name, StartLine: startLine, StartCol: startCol,
				EndLine: endLine, EndCol: endCol,
			}
		}
		families = append(families, FamilySpans{Family: family, Spans: spans})
	}
	return counts, families, nil
}

func sourceUint32(reader *canonical.Reader) (uint32, error) {
	value, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, canonical.ErrMalformed
	}
	return uint32(value), nil
}

func validSourceCoordinate(startLine, startCol, endLine, endCol uint32) bool {
	if startLine == 0 && startCol == 0 && endLine == 0 && endCol == 0 {
		return true
	}
	if startLine == 0 || startCol == 0 || (endLine == 0) != (endCol == 0) {
		return false
	}
	return endLine == 0 || endLine > startLine || endLine == startLine && endCol >= startCol
}

// preflightSourceSpans consumes one copied Reader without creating any span
// slice. The second pass in readSourceSpans is allowed to allocate only after
// this complete count, coordinate, and family-order proof succeeds.
func preflightSourceSpans(reader *canonical.Reader, counts *[keyspace.FamilyCount]uint32) error {
	if reader == nil || counts == nil {
		return canonical.ErrMalformed
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		tag, err := reader.Uint()
		if err != nil {
			return err
		}
		if tag != uint64(family) {
			return canonical.ErrMalformed
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
		if err != nil {
			return err
		}
		counts[family] = uint32(count)
		for index := 0; index < count; index++ {
			startLine, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			startCol, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			endLine, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			endCol, err := sourceUint32(reader)
			if err != nil {
				return err
			}
			if !validSourceCoordinate(startLine, startCol, endLine, endCol) {
				return canonical.ErrMalformed
			}
		}
	}
	return nil
}

// preflightSourceLiterals validates every literal row from a Reader copy.
// String payloads are inspected through StringBytes, so no Go string is copied
// until the allocation/fill pass begins.
func preflightSourceLiterals(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32) error {
	if reader == nil {
		return canonical.ErrMalformed
	}
	for _, family := range []keyspace.Family{keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString} {
		if err := readSourceLiteralTag(reader, family); err != nil {
			return err
		}
		count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
		if err != nil {
			return err
		}
		if uint32(count) != counts[family] {
			return canonical.ErrMalformed
		}
		for index := 0; index < count; index++ {
			if _, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false); err != nil {
				return err
			}
			switch family {
			case keyspace.FamilyNil:
			case keyspace.FamilyBool:
				if _, err := reader.Bool(); err != nil {
					return err
				}
			case keyspace.FamilyInteger, keyspace.FamilyFloat:
				if _, err := reader.Uint(); err != nil {
					return err
				}
			case keyspace.FamilyString:
				if _, err := sourceStringBytes(reader); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func readSourceLiterals(reader *canonical.Reader, input *Input, counts [keyspace.FamilyCount]uint32) error {
	if input == nil {
		return canonical.ErrMalformed
	}
	if err := readSourceLiteralTag(reader, keyspace.FamilyNil); err != nil {
		return err
	}
	nilCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(nilCount) != counts[keyspace.FamilyNil] {
		return canonical.ErrMalformed
	}
	input.Nil = make([]NilLiteral, nilCount)
	for index := range input.Nil {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		input.Nil[index] = NilLiteral{Owner: owner}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyBool); err != nil {
		return err
	}
	boolCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(boolCount) != counts[keyspace.FamilyBool] {
		return canonical.ErrMalformed
	}
	input.Bool = make([]BoolLiteral, boolCount)
	for index := range input.Bool {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := reader.Bool()
		if err != nil {
			return err
		}
		input.Bool[index] = BoolLiteral{Owner: owner, Value: value}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyInteger); err != nil {
		return err
	}
	integerCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(integerCount) != counts[keyspace.FamilyInteger] {
		return canonical.ErrMalformed
	}
	input.Integer = make([]IntegerLiteral, integerCount)
	for index := range input.Integer {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := reader.Uint()
		if err != nil {
			return err
		}
		input.Integer[index] = IntegerLiteral{Owner: owner, Value: int64(value)}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyFloat); err != nil {
		return err
	}
	floatCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(floatCount) != counts[keyspace.FamilyFloat] {
		return canonical.ErrMalformed
	}
	input.Float = make([]FloatLiteral, floatCount)
	for index := range input.Float {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		bits, err := reader.Uint()
		if err != nil {
			return err
		}
		input.Float[index] = FloatLiteral{Owner: owner, Bits: bits}
	}

	if err := readSourceLiteralTag(reader, keyspace.FamilyString); err != nil {
		return err
	}
	stringCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	if uint32(stringCount) != counts[keyspace.FamilyString] {
		return canonical.ErrMalformed
	}
	input.String = make([]StringLiteral, stringCount)
	for index := range input.String {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		value, err := sourceString(reader)
		if err != nil {
			return err
		}
		input.String[index] = StringLiteral{Owner: owner, Value: value}
	}
	return nil
}

func readSourceLiteralTag(reader *canonical.Reader, family keyspace.Family) error {
	tag, err := reader.Uint()
	if err != nil {
		return err
	}
	if tag != uint64(family) {
		return canonical.ErrMalformed
	}
	return nil
}

// sourceTermBits is a bounded dense membership plane. It is allocated only
// after the first order pass has proved every Count and row shape, and it is
// discarded when this preflight returns. A bitset keeps the hostile case
// linear without allowing one source Term to reserve a Go object of its own.
type sourceTermBits []uint64

func newSourceTermBits(count uint32) sourceTermBits {
	words := (uint64(count) + 63) / 64
	return make(sourceTermBits, int(words))
}

func (bits sourceTermBits) mark(ordinal uint32) bool {
	if ordinal == 0 {
		return true
	}
	index := uint64(ordinal - 1)
	word := index >> 6
	if word >= uint64(len(bits)) {
		return true
	}
	mask := uint64(1) << (index & 63)
	if bits[word]&mask != 0 {
		return true
	}
	bits[word] |= mask
	return false
}

type sourceOrderScratch struct {
	direct      [keyspace.FamilyCount]sourceTermBits
	cells       sourceTermBits
	faultOwners []keyspace.Term
}

func newSourceOrderScratch(counts [keyspace.FamilyCount]uint32) sourceOrderScratch {
	var scratch sourceOrderScratch
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if sourceDirectFamily(family) {
			scratch.direct[family] = newSourceTermBits(counts[family])
		}
	}
	scratch.cells = newSourceTermBits(counts[keyspace.FamilyCell])
	scratch.faultOwners = make([]keyspace.Term, int(counts[keyspace.FamilyControlFault]))
	return scratch
}

// preflightSourceOrder first proves all Count arities and row shapes on a
// copied Reader. Only after that allocation-free pass does it allocate the
// bounded dense scratch used by the second pass for all uniqueness and owner
// relations. Both passes are linear in the order section.
func preflightSourceOrder(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32) ([]keyspace.Term, error) {
	if reader == nil {
		return nil, canonical.ErrMalformed
	}
	probe := *reader
	if err := walkSourceOrder(&probe, counts, nil); err != nil {
		return nil, err
	}
	scratch := newSourceOrderScratch(counts)
	if err := walkSourceOrder(reader, counts, &scratch); err != nil {
		return nil, err
	}
	for _, owner := range scratch.faultOwners {
		if owner == 0 {
			return nil, canonical.ErrMalformed
		}
	}
	return scratch.faultOwners, nil
}

func walkSourceOrder(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32, scratch *sourceOrderScratch) error {
	if reader == nil {
		return canonical.ErrMalformed
	}
	if err := readSourceRangeTag(reader, keyspace.FamilyBody); err != nil {
		return err
	}
	bodyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(bodyCount) != counts[keyspace.FamilyBody] {
		return canonical.ErrMalformed
	}
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyIndex+1))
		for termIndex := 0; termIndex < rowCount; termIndex++ {
			term, err := readBoundTerm(reader, counts, keyspace.FamilyInvalid, false)
			if err != nil {
				return err
			}
			family := keyspace.TermFamily(term)
			if !sourceDirectFamily(family) {
				return canonical.ErrMalformed
			}
			if scratch == nil {
				continue
			}
			if scratch.direct[family].mark(keyspace.TermOrdinal(term)) {
				return canonical.ErrMalformed
			}
			if family == keyspace.FamilyControlFault {
				ordinal := keyspace.TermOrdinal(term)
				if ordinal == 0 || uint64(ordinal) > uint64(len(scratch.faultOwners)) || scratch.faultOwners[ordinal-1] != 0 {
					return canonical.ErrMalformed
				}
				scratch.faultOwners[ordinal-1] = body
			}
		}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyBind); err != nil {
		return err
	}
	bindCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(bindCount) != counts[keyspace.FamilyBind] {
		return canonical.ErrMalformed
	}
	for bindIndex := 0; bindIndex < bindCount; bindIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		for cellIndex := 0; cellIndex < rowCount; cellIndex++ {
			cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return err
			}
			if scratch != nil && scratch.cells.mark(keyspace.TermOrdinal(cell)) {
				return canonical.ErrMalformed
			}
		}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyFunction); err != nil {
		return err
	}
	functionCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return err
	}
	if uint32(functionCount) != counts[keyspace.FamilyFunction] {
		return canonical.ErrMalformed
	}
	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return err
		}
		for formalIndex := 0; formalIndex < rowCount; formalIndex++ {
			formal, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return err
			}
			if scratch != nil && scratch.cells.mark(keyspace.TermOrdinal(formal)) {
				return canonical.ErrMalformed
			}
		}
	}
	return nil
}

// readSourceOrder returns the direct Body owner of each authored ControlFault
// ordinal. Keeping this as a dense slice (rather than a map) mirrors the
// Input's canonical ordinal storage and lets the fault section cross-check
// owner provenance without importing or invoking Build.
func readSourceOrder(reader *canonical.Reader, input *Input, counts [keyspace.FamilyCount]uint32) ([]keyspace.Term, error) {
	if input == nil {
		return nil, canonical.ErrMalformed
	}
	faultOwners := make([]keyspace.Term, int(counts[keyspace.FamilyControlFault]))
	var seen [keyspace.FamilyCount][]bool
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if counts[family] != 0 {
			seen[family] = make([]bool, int(counts[family]))
		}
	}
	seenCells := make([]bool, int(counts[keyspace.FamilyCell]))

	if err := readSourceRangeTag(reader, keyspace.FamilyBody); err != nil {
		return nil, err
	}
	bodyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(bodyCount) != counts[keyspace.FamilyBody] {
		return nil, canonical.ErrMalformed
	}
	input.Bodies = make([]BodySource, bodyCount)
	for index := range input.Bodies {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
		terms, err := readDirectTerms(reader, counts, seen, faultOwners, body)
		if err != nil {
			return nil, err
		}
		input.Bodies[index] = BodySource{Body: body, Terms: terms}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyBind); err != nil {
		return nil, err
	}
	bindCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(bindCount) != counts[keyspace.FamilyBind] {
		return nil, canonical.ErrMalformed
	}
	input.Binds = make([]BindCells, bindCount)
	for index := range input.Binds {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return nil, err
		}
		cells := make([]keyspace.Term, rowCount)
		for at := range cells {
			cell, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return nil, err
			}
			ordinal := keyspace.TermOrdinal(cell) - 1
			if seenCells[ordinal] {
				return nil, canonical.ErrMalformed
			}
			seenCells[ordinal] = true
			cells[at] = cell
		}
		input.Binds[index] = BindCells{Bind: bind, Cells: cells}
	}

	if err := readSourceRangeTag(reader, keyspace.FamilyFunction); err != nil {
		return nil, err
	}
	functionCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	if uint32(functionCount) != counts[keyspace.FamilyFunction] {
		return nil, canonical.ErrMalformed
	}
	input.Functions = make([]FunctionFormals, functionCount)
	for index := range input.Functions {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		rowCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
		if err != nil {
			return nil, err
		}
		formals := make([]keyspace.Term, rowCount)
		for at := range formals {
			formal, err := readBoundTerm(reader, counts, keyspace.FamilyCell, false)
			if err != nil {
				return nil, err
			}
			ordinal := keyspace.TermOrdinal(formal) - 1
			if seenCells[ordinal] {
				return nil, canonical.ErrMalformed
			}
			seenCells[ordinal] = true
			formals[at] = formal
		}
		input.Functions[index] = FunctionFormals{Function: function, Formals: formals}
	}
	return faultOwners, nil
}

func readSourceRangeTag(reader *canonical.Reader, family keyspace.Family) error {
	tag, err := reader.Uint()
	if err != nil {
		return err
	}
	if tag != uint64(family) {
		return canonical.ErrMalformed
	}
	return nil
}

// readSourceLiteralRef returns a zero-copy semantic view used only by
// preflight. A String payload points into the immutable Reader input; the
// conversion to a Go string is deferred until the allocation/fill pass.
func readSourceLiteralRef(reader *canonical.Reader) (exactKeyRef, error) {
	kind, err := reader.Uint()
	if err != nil {
		return exactKeyRef{}, err
	}
	if kind < uint64(keyspace.LiteralBool) || kind > uint64(keyspace.LiteralString) {
		return exactKeyRef{}, canonical.ErrMalformed
	}
	ref := exactKeyRef{kind: keyspace.LiteralKind(kind)}
	switch ref.kind {
	case keyspace.LiteralBool:
		ref.boolean, err = reader.Bool()
	case keyspace.LiteralInteger:
		var value uint64
		value, err = reader.Uint()
		ref.integer = int64(value)
	case keyspace.LiteralFloat:
		ref.float, err = reader.Uint()
	case keyspace.LiteralString:
		ref.bytes, err = sourceStringBytes(reader)
	}
	if err != nil {
		return exactKeyRef{}, err
	}
	return ref, nil
}

// preflightSourceKeys validates all three key/fault pools after their exact
// arities have been bounded. Exact atoms are emitted first in canonical dense
// order, so each Key row can carry and validate its nonzero exact ordinal in
// O(1), without a map, sort, replay, or retained lookup state.
func preflightSourceKeys(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32, faultOwners []keyspace.Term) error {
	if reader == nil {
		return canonical.ErrMalformed
	}
	exactCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	keyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(keyCount) != counts[keyspace.FamilyKey] {
		return canonical.ErrMalformed
	}
	faultCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(faultCount) != counts[keyspace.FamilyControlFault] {
		return canonical.ErrMalformed
	}

	exactRows := make([]exactKeyRef, exactCount)
	var previous exactKeyRef
	for index := 0; index < exactCount; index++ {
		value, err := readSourceLiteralRef(reader)
		if err != nil {
			return err
		}
		normalized, ok := normalizeExactKeyRef(value)
		if !ok || !exactKeyRefEqual(value, normalized) {
			return canonical.ErrMalformed
		}
		if index > 0 && compareCanonicalExactKey(previous, value) >= 0 {
			return canonical.ErrMalformed
		}
		exactRows[index] = value
		previous = value
	}
	for index := 0; index < keyCount; index++ {
		if _, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false); err != nil {
			return err
		}
		form, err := reader.Uint()
		if err != nil {
			return err
		}
		if form < uint64(keyFormName) || form > uint64(keyFormList) {
			return canonical.ErrMalformed
		}
		exactOrdinal, err := reader.Uint()
		if err != nil {
			return err
		}
		if exactOrdinal == 0 || exactOrdinal > uint64(exactCount) {
			return canonical.ErrMalformed
		}
		exact := exactRows[exactOrdinal-1]
		if !validSourceKey(keyForm(form), exactKeyRefValue(exact, false)) {
			return canonical.ErrMalformed
		}
	}
	if len(faultOwners) != faultCount {
		return canonical.ErrMalformed
	}
	for index := 0; index < faultCount; index++ {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		kind, err := reader.Uint()
		if err != nil {
			return err
		}
		if kind < uint64(ControlFaultDuplicateLabel) || kind > uint64(ControlFaultBreakOutsideLoop) {
			return canonical.ErrMalformed
		}
		label, err := readBoundTerm(reader, counts, keyspace.FamilyLabel, true)
		if err != nil {
			return err
		}
		blocker, err := readBoundTerm(reader, counts, keyspace.FamilyCell, true)
		if err != nil {
			return err
		}
		fault := ControlFault{Owner: owner, Kind: ControlFaultKind(kind), Label: label, Blocker: blocker}
		if faultOwners[index] == 0 || faultOwners[index] != owner || !validDecodedFault(fault) {
			return canonical.ErrMalformed
		}
	}
	return nil
}

func readDirectTerms(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32, seen [keyspace.FamilyCount][]bool, faultOwners []keyspace.Term, body keyspace.Term) ([]keyspace.Term, error) {
	count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	terms := make([]keyspace.Term, count)
	for index := range terms {
		term, err := readBoundTerm(reader, counts, keyspace.FamilyInvalid, false)
		if err != nil {
			return nil, err
		}
		if !sourceDirectFamily(keyspace.TermFamily(term)) {
			return nil, canonical.ErrMalformed
		}
		family := keyspace.TermFamily(term)
		ordinal := keyspace.TermOrdinal(term)
		if seen[family][ordinal-1] {
			return nil, canonical.ErrMalformed
		}
		seen[family][ordinal-1] = true
		if family == keyspace.FamilyControlFault {
			if faultOwners[ordinal-1] != 0 {
				return nil, canonical.ErrMalformed
			}
			faultOwners[ordinal-1] = body
		}
		terms[index] = term
	}
	return terms, nil
}

func readBoundTerm(reader *canonical.Reader, counts [keyspace.FamilyCount]uint32, family keyspace.Family, allowZero bool) (keyspace.Term, error) {
	raw, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if raw == 0 {
		if allowZero {
			return 0, nil
		}
		return 0, canonical.ErrMalformed
	}
	if raw > uint64(^uint32(0)) {
		return 0, canonical.ErrMalformed
	}
	term := keyspace.Term(uint32(raw))
	termFamily, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if termFamily == keyspace.FamilyInvalid || ordinal == 0 || ordinal > keyspace.MaxTermOrdinal || keyspace.MakeTerm(termFamily, ordinal) != term {
		return 0, canonical.ErrMalformed
	}
	if family != keyspace.FamilyInvalid && termFamily != family {
		return 0, canonical.ErrMalformed
	}
	if uint64(ordinal) > uint64(counts[termFamily]) {
		return 0, canonical.ErrMalformed
	}
	return term, nil
}

func readSourceKeys(reader *canonical.Reader, input *Input, counts [keyspace.FamilyCount]uint32, faultOwners []keyspace.Term) error {
	if input == nil {
		return canonical.ErrMalformed
	}
	// The three key-section arities are consecutive, before any row payload.
	// Read and bound all of them first so no untrusted arity controls an
	// allocation while another section count remains unread.
	exactCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	keyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(keyCount) != counts[keyspace.FamilyKey] {
		return canonical.ErrMalformed
	}
	faultCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(faultCount) != counts[keyspace.FamilyControlFault] {
		return canonical.ErrMalformed
	}
	input.ExactAtoms = make([]keyspace.LiteralValue, exactCount)
	for index := range input.ExactAtoms {
		value, err := readExactValue(reader)
		if err != nil {
			return err
		}
		normalized, ok := NormalizeExactKey(value)
		if !ok || !equalLiteral(value, normalized) {
			return canonical.ErrMalformed
		}
		if index > 0 && compareCanonicalExactKey(exactKeyRefFromLiteral(input.ExactAtoms[index-1]), exactKeyRefFromLiteral(value)) >= 0 {
			return canonical.ErrMalformed
		}
		input.ExactAtoms[index] = value
	}

	input.Keys = make([]KeyInput, keyCount)
	for index := range input.Keys {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		formRaw, err := reader.Uint()
		if err != nil {
			return err
		}
		if formRaw < uint64(keyFormName) || formRaw > uint64(keyFormList) {
			return canonical.ErrMalformed
		}
		exactOrdinal, err := reader.Uint()
		if err != nil {
			return err
		}
		if exactOrdinal == 0 || exactOrdinal > uint64(len(input.ExactAtoms)) {
			return canonical.ErrMalformed
		}
		// The exact atom denominator was fully materialized immediately above;
		// the ordinal is the canonical dense Key handle and needs no literal
		// replay or lookup.
		exact := input.ExactAtoms[exactOrdinal-1]
		if !validSourceKey(keyForm(formRaw), exact) {
			return canonical.ErrMalformed
		}
		input.Keys[index] = KeyInput{owner: owner, exact: exact, form: keyForm(formRaw)}
	}

	input.Faults = make([]ControlFault, faultCount)
	for index := range input.Faults {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		kindRaw, err := reader.Uint()
		if err != nil {
			return err
		}
		if kindRaw < uint64(ControlFaultDuplicateLabel) || kindRaw > uint64(ControlFaultBreakOutsideLoop) {
			return canonical.ErrMalformed
		}
		label, err := readBoundTerm(reader, counts, keyspace.FamilyLabel, true)
		if err != nil {
			return err
		}
		blocker, err := readBoundTerm(reader, counts, keyspace.FamilyCell, true)
		if err != nil {
			return err
		}
		fault := ControlFault{Owner: owner, Kind: ControlFaultKind(kindRaw), Label: label, Blocker: blocker}
		if !validDecodedFault(fault) || index >= len(faultOwners) || faultOwners[index] != owner {
			return canonical.ErrMalformed
		}
		input.Faults[index] = fault
	}
	return nil
}

func readExactValue(reader *canonical.Reader) (keyspace.LiteralValue, error) {
	ref, err := readSourceLiteralRef(reader)
	if err != nil {
		return keyspace.LiteralValue{}, err
	}
	return exactKeyRefValue(ref, true), nil
}

func equalLiteral(left, right keyspace.LiteralValue) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case keyspace.LiteralBool:
		return left.Bool == right.Bool
	case keyspace.LiteralInteger:
		return left.Integer == right.Integer
	case keyspace.LiteralFloat:
		return left.FloatBits == right.FloatBits
	case keyspace.LiteralString:
		return left.String == right.String
	default:
		return false
	}
}

func validDecodedFault(fault ControlFault) bool {
	if !fault.Kind.valid() {
		return false
	}
	label := fault.Label != 0
	blocker := fault.Blocker != 0
	switch fault.Kind {
	case ControlFaultDuplicateLabel:
		return label && !blocker
	case ControlFaultUndefinedGoto, ControlFaultBreakOutsideLoop:
		return !label && !blocker
	case ControlFaultGotoEntersLocal:
		return label && blocker
	default:
		return false
	}
}
