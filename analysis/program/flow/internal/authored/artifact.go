package authored

import (
	"math"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Every canonical event has a two-byte minimum frame. These exact minima are
// used below for each concrete row shape so an untrusted Count can never
// reserve a large slice unless the bounded reader contains enough bytes for
// at least one complete row per element.
const (
	artifactUintWireMin = uint64(3)
	artifactBoolWireMin = uint64(3)
	artifactTermWireMin = artifactUintWireMin * 2

	artifactValuesTermWireMin = artifactTermWireMin
	artifactValuesRowWireMin  = artifactTermWireMin*2 + artifactUintWireMin*2

	artifactExactWireMin   = artifactTermWireMin*3 + artifactUintWireMin
	artifactDynamicWireMin = artifactTermWireMin * 3

	artifactCellWireMin   = artifactUintWireMin + artifactTermWireMin + artifactUintWireMin
	artifactReadWireMin   = artifactTermWireMin*2 + artifactBoolWireMin
	artifactPairWireMin   = artifactTermWireMin * 2
	artifactWriteWireMin  = artifactTermWireMin * 2
	artifactTableOrderMin = artifactTermWireMin
	artifactTableRowMin   = artifactTermWireMin + artifactUintWireMin*2
	artifactFieldWireMin  = artifactTermWireMin*3 + artifactUintWireMin

	artifactCaptureWireMin  = artifactTermWireMin * 2
	artifactFunctionWireMin = artifactTermWireMin*3 + artifactUintWireMin*2

	artifactUnaryWireMin  = artifactTermWireMin*2 + artifactUintWireMin
	artifactBinaryWireMin = artifactTermWireMin*3 + artifactUintWireMin
	artifactCallWireMin   = artifactTermWireMin * 4

	artifactReturnWireMin = artifactTermWireMin * 2
	artifactOwnerWireMin  = artifactTermWireMin
	artifactBreakWireMin  = artifactTermWireMin * 2
	artifactGotoWireMin   = artifactTermWireMin * 2
	artifactBranchWireMin = artifactTermWireMin * 4
	artifactLoopWireMin   = artifactTermWireMin*3 + artifactUintWireMin*3
	artifactClaimWireMin  = artifactTermWireMin + artifactUintWireMin
)

type artifactDecoder struct {
	reader *framing.Reader
}

func (decoder *artifactDecoder) record(expected uint64) error {
	got, err := decoder.reader.Record()
	if err != nil {
		return err
	}
	if got != expected {
		return errInvalidArtifactSection
	}
	return nil
}

func (decoder *artifactDecoder) count(rowMinimum uint64) (int, error) {
	value, err := decoder.reader.Count()
	if err != nil {
		return 0, err
	}
	if value > uint64(keyspace.MaxTermOrdinal) || value > maxIntValue() ||
		(rowMinimum == 0 || value > uint64(decoder.reader.Remaining())/rowMinimum) {
		return 0, errInvalidArtifactSection
	}
	return int(value), nil
}

func (decoder *artifactDecoder) term() (keyspace.Term, error) {
	family, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	ordinal, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if family == 0 && ordinal == 0 {
		return 0, nil
	}
	if family == 0 || family >= uint64(keyspace.FamilyCount) || ordinal == 0 || ordinal > uint64(keyspace.MaxTermOrdinal) {
		return 0, errInvalidArtifactSection
	}
	return keyspace.MakeTerm(keyspace.Family(family), uint32(ordinal)), nil
}

func (decoder *artifactDecoder) value(maximum uint64) (uint64, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value == 0 || value > maximum {
		return 0, errInvalidArtifactSection
	}
	return value, nil
}

func (decoder *artifactDecoder) ordinal() (uint32, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(keyspace.MaxTermOrdinal) {
		return 0, errInvalidArtifactSection
	}
	return uint32(value), nil
}

func (decoder *artifactDecoder) rangeFor(length int) (Range, error) {
	start, err := decoder.ordinal()
	if err != nil {
		return Range{}, err
	}
	end, err := decoder.ordinal()
	if err != nil {
		return Range{}, err
	}
	if start > end || length < 0 || uint64(end) > uint64(length) {
		return Range{}, errInvalidArtifactSection
	}
	return Range{Start: start, End: end}, nil
}

func (decoder *artifactDecoder) key() (keyspace.Key, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, errInvalidArtifactSection
	}
	return keyspace.Key(value), nil
}

func (decoder *artifactDecoder) values() (ValuesInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanValues(); err != nil {
		return ValuesInput{}, err
	}
	return decoder.decodeValues()
}

func (decoder *artifactDecoder) scanValues() error {
	termCount, err := decoder.count(artifactValuesTermWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < termCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	rowCount, err := decoder.count(artifactValuesRowWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < rowCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.rangeFor(termCount); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeValues() (ValuesInput, error) {
	termCount, err := decoder.count(artifactValuesTermWireMin)
	if err != nil {
		return ValuesInput{}, err
	}
	terms := make([]keyspace.Term, termCount)
	for index := range terms {
		terms[index], err = decoder.term()
		if err != nil {
			return ValuesInput{}, err
		}
	}
	rowCount, err := decoder.count(artifactValuesRowWireMin)
	if err != nil {
		return ValuesInput{}, err
	}
	rows := make([]Value, rowCount)
	for index := range rows {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ValuesInput{}, readErr
		}
		tail, readErr := decoder.term()
		if readErr != nil {
			return ValuesInput{}, readErr
		}
		fixed, readErr := decoder.rangeFor(len(terms))
		if readErr != nil {
			return ValuesInput{}, readErr
		}
		rows[index] = Value{Owner: owner, Fixed: fixed, Tail: tail}
	}
	return ValuesInput{Rows: rows, Terms: terms}, nil
}

func (decoder *artifactDecoder) access() (AccessInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanAccess(); err != nil {
		return AccessInput{}, err
	}
	return decoder.decodeAccess()
}

func (decoder *artifactDecoder) scanAccess() error {
	exactCount, err := decoder.count(artifactExactWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < exactCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.FieldKey)); err != nil {
			return err
		}
	}
	dynamicCount, err := decoder.count(artifactDynamicWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < dynamicCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeAccess() (AccessInput, error) {
	exactCount, err := decoder.count(artifactExactWireMin)
	if err != nil {
		return AccessInput{}, err
	}
	exact := make([]ExactLens, exactCount)
	for index := range exact {
		owner, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		base, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		source, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		fieldKind, readErr := decoder.value(uint64(kind.FieldKey))
		if readErr != nil {
			return AccessInput{}, readErr
		}
		exact[index] = ExactLens{
			Owner:  owner,
			Base:   base,
			Source: source,
			Kind:   kind.FieldKind(fieldKind),
		}
	}
	dynamicCount, err := decoder.count(artifactDynamicWireMin)
	if err != nil {
		return AccessInput{}, err
	}
	dynamic := make([]DynamicLens, dynamicCount)
	for index := range dynamic {
		owner, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		base, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		key, readErr := decoder.term()
		if readErr != nil {
			return AccessInput{}, readErr
		}
		dynamic[index] = DynamicLens{Owner: owner, Base: base, Key: key}
	}
	return AccessInput{Exact: exact, Dynamic: dynamic}, nil
}

func (decoder *artifactDecoder) storage() (StorageInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanStorage(); err != nil {
		return StorageInput{}, err
	}
	return decoder.decodeStorage()
}

func (decoder *artifactDecoder) scanStorage() error {
	cellCount, err := decoder.count(artifactCellWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < cellCount; index++ {
		if _, err := decoder.value(uint64(CellGlobal)); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.key(); err != nil {
			return err
		}
	}
	readCount, err := decoder.count(artifactReadWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < readCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.reader.Bool(); err != nil {
			return err
		}
	}
	varargCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < varargCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	bindCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < bindCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	assignCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < assignCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	writeCount, err := decoder.count(artifactWriteWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < writeCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeStorage() (StorageInput, error) {
	cellCount, err := decoder.count(artifactCellWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	cells := make([]Cell, cellCount)
	for index := range cells {
		cellKind, readErr := decoder.value(uint64(CellGlobal))
		if readErr != nil {
			return StorageInput{}, readErr
		}
		body, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		key, readErr := decoder.key()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		cells[index] = Cell{Kind: CellKind(cellKind), Body: body, Key: key}
	}

	readCount, err := decoder.count(artifactReadWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	reads := make([]Read, readCount)
	for index := range reads {
		owner, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		source, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		implicit, readErr := decoder.reader.Bool()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		reads[index] = Read{Owner: owner, Source: source, Implicit: implicit}
	}

	varargCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	varargs := make([]Vararg, varargCount)
	for index := range varargs {
		owner, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		cell, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		varargs[index] = Vararg{Owner: owner, Cell: cell}
	}

	bindCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	binds := make([]Bind, bindCount)
	for index := range binds {
		owner, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		values, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		binds[index] = Bind{Owner: owner, Values: values}
	}

	assignCount, err := decoder.count(artifactPairWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	assigns := make([]Assign, assignCount)
	for index := range assigns {
		owner, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		values, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		assigns[index] = Assign{Owner: owner, Values: values}
	}

	writeCount, err := decoder.count(artifactWriteWireMin)
	if err != nil {
		return StorageInput{}, err
	}
	writes := make([]Write, writeCount)
	for index := range writes {
		assign, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		target, readErr := decoder.term()
		if readErr != nil {
			return StorageInput{}, readErr
		}
		writes[index] = Write{Assign: assign, Target: target}
	}
	return StorageInput{Cells: cells, Reads: reads, Varargs: varargs, Binds: binds, Assigns: assigns, Writes: writes}, nil
}

func (decoder *artifactDecoder) tables() (TablesInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanTables(); err != nil {
		return TablesInput{}, err
	}
	return decoder.decodeTables()
}

func (decoder *artifactDecoder) scanTables() error {
	orderCount, err := decoder.count(artifactTableOrderMin)
	if err != nil {
		return err
	}
	for index := 0; index < orderCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	rowCount, err := decoder.count(artifactTableRowMin)
	if err != nil {
		return err
	}
	for index := 0; index < rowCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.rangeFor(orderCount); err != nil {
			return err
		}
	}
	fieldCount, err := decoder.count(artifactFieldWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < fieldCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.FieldKey)); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeTables() (TablesInput, error) {
	orderCount, err := decoder.count(artifactTableOrderMin)
	if err != nil {
		return TablesInput{}, err
	}
	order := make([]keyspace.Term, orderCount)
	for index := range order {
		order[index], err = decoder.term()
		if err != nil {
			return TablesInput{}, err
		}
	}
	rowCount, err := decoder.count(artifactTableRowMin)
	if err != nil {
		return TablesInput{}, err
	}
	rows := make([]Table, rowCount)
	for index := range rows {
		owner, readErr := decoder.term()
		if readErr != nil {
			return TablesInput{}, readErr
		}
		fields, readErr := decoder.rangeFor(len(order))
		if readErr != nil {
			return TablesInput{}, readErr
		}
		rows[index] = Table{Owner: owner, Fields: fields}
	}
	fieldCount, err := decoder.count(artifactFieldWireMin)
	if err != nil {
		return TablesInput{}, err
	}
	fields := make([]Field, fieldCount)
	for index := range fields {
		table, readErr := decoder.term()
		if readErr != nil {
			return TablesInput{}, readErr
		}
		key, readErr := decoder.term()
		if readErr != nil {
			return TablesInput{}, readErr
		}
		values, readErr := decoder.term()
		if readErr != nil {
			return TablesInput{}, readErr
		}
		fieldKind, readErr := decoder.value(uint64(kind.FieldKey))
		if readErr != nil {
			return TablesInput{}, readErr
		}
		fields[index] = Field{Table: table, Key: key, Values: values, Kind: kind.FieldKind(fieldKind)}
	}
	return TablesInput{Rows: rows, Fields: fields, Order: order}, nil
}

func (decoder *artifactDecoder) functions() (FunctionsInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanFunctions(); err != nil {
		return FunctionsInput{}, err
	}
	return decoder.decodeFunctions()
}

func (decoder *artifactDecoder) scanFunctions() error {
	captureCount, err := decoder.count(artifactCaptureWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < captureCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	rowCount, err := decoder.count(artifactFunctionWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < rowCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.rangeFor(captureCount); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeFunctions() (FunctionsInput, error) {
	captureCount, err := decoder.count(artifactCaptureWireMin)
	if err != nil {
		return FunctionsInput{}, err
	}
	captures := make([]Capture, captureCount)
	for index := range captures {
		inner, readErr := decoder.term()
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		outer, readErr := decoder.term()
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		captures[index] = Capture{Inner: inner, Outer: outer}
	}
	rowCount, err := decoder.count(artifactFunctionWireMin)
	if err != nil {
		return FunctionsInput{}, err
	}
	rows := make([]Function, rowCount)
	for index := range rows {
		owner, readErr := decoder.term()
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		body, readErr := decoder.term()
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		vararg, readErr := decoder.term()
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		captureRange, readErr := decoder.rangeFor(len(captures))
		if readErr != nil {
			return FunctionsInput{}, readErr
		}
		rows[index] = Function{Owner: owner, Body: body, Vararg: vararg, Captures: captureRange}
	}
	return FunctionsInput{Rows: rows, Captures: captures}, nil
}

func (decoder *artifactDecoder) operators() (OperatorsInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanOperators(); err != nil {
		return OperatorsInput{}, err
	}
	return decoder.decodeOperators()
}

func (decoder *artifactDecoder) scanOperators() error {
	unaryCount, err := decoder.count(artifactUnaryWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < unaryCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.UnaryBitNot)); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	binaryCount, err := decoder.count(artifactBinaryWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < binaryCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.BinaryGreaterEqual)); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	selectCount, err := decoder.count(artifactBinaryWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < selectCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.SelectOr)); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeOperators() (OperatorsInput, error) {
	unaryCount, err := decoder.count(artifactUnaryWireMin)
	if err != nil {
		return OperatorsInput{}, err
	}
	unaries := make([]Unary, unaryCount)
	for index := range unaries {
		owner, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		op, readErr := decoder.value(uint64(kind.UnaryBitNot))
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		operand, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		unaries[index] = Unary{Owner: owner, Op: kind.UnaryOp(op), Operand: operand}
	}

	binaryCount, err := decoder.count(artifactBinaryWireMin)
	if err != nil {
		return OperatorsInput{}, err
	}
	binaries := make([]Binary, binaryCount)
	for index := range binaries {
		owner, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		op, readErr := decoder.value(uint64(kind.BinaryGreaterEqual))
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		left, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		right, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		binaries[index] = Binary{Owner: owner, Op: kind.BinaryOp(op), Left: left, Right: right}
	}

	selectCount, err := decoder.count(artifactBinaryWireMin)
	if err != nil {
		return OperatorsInput{}, err
	}
	selects := make([]Select, selectCount)
	for index := range selects {
		owner, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		op, readErr := decoder.value(uint64(kind.SelectOr))
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		left, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		right, readErr := decoder.term()
		if readErr != nil {
			return OperatorsInput{}, readErr
		}
		selects[index] = Select{Owner: owner, Op: kind.SelectOp(op), Left: left, Right: right}
	}
	return OperatorsInput{Unaries: unaries, Binaries: binaries, Selects: selects}, nil
}

func (decoder *artifactDecoder) calls() ([]Call, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanCalls(); err != nil {
		return nil, err
	}
	return decoder.decodeCalls()
}

func (decoder *artifactDecoder) scanCalls() error {
	count, err := decoder.count(artifactCallWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeCalls() ([]Call, error) {
	count, err := decoder.count(artifactCallWireMin)
	if err != nil {
		return nil, err
	}
	rows := make([]Call, count)
	for index := range rows {
		owner, readErr := decoder.term()
		if readErr != nil {
			return nil, readErr
		}
		callee, readErr := decoder.term()
		if readErr != nil {
			return nil, readErr
		}
		receiver, readErr := decoder.term()
		if readErr != nil {
			return nil, readErr
		}
		actuals, readErr := decoder.term()
		if readErr != nil {
			return nil, readErr
		}
		rows[index] = Call{Owner: owner, Callee: callee, Receiver: receiver, Actuals: actuals}
	}
	return rows, nil
}

func (decoder *artifactDecoder) control() (ControlInput, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanControl(); err != nil {
		return ControlInput{}, err
	}
	return decoder.decodeControl()
}

func (decoder *artifactDecoder) scanControl() error {
	returnCount, err := decoder.count(artifactReturnWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < returnCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	breakCount, err := decoder.count(artifactBreakWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < breakCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	labelCount, err := decoder.count(artifactOwnerWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < labelCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	gotoCount, err := decoder.count(artifactGotoWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < gotoCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	branchCount, err := decoder.count(artifactBranchWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < branchCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	cellCount, err := decoder.count(artifactTermWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < cellCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	loopCount, err := decoder.count(artifactLoopWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < loopCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.LoopGenericFor)); err != nil {
			return err
		}
		if _, err := decoder.rangeFor(cellCount); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeControl() (ControlInput, error) {
	returnCount, err := decoder.count(artifactReturnWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	returns := make([]Return, returnCount)
	for index := range returns {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		values, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		returns[index] = Return{Owner: owner, Values: values}
	}

	breakCount, err := decoder.count(artifactBreakWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	breaks := make([]Break, breakCount)
	for index := range breaks {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		target, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		breaks[index] = Break{Owner: owner, Target: target}
	}

	labelCount, err := decoder.count(artifactOwnerWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	labels := make([]Label, labelCount)
	for index := range labels {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		labels[index] = Label{Owner: owner}
	}

	gotoCount, err := decoder.count(artifactGotoWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	gotos := make([]Goto, gotoCount)
	for index := range gotos {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		target, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		gotos[index] = Goto{Owner: owner, Target: target}
	}

	branchCount, err := decoder.count(artifactBranchWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	branches := make([]Branch, branchCount)
	for index := range branches {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		condition, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		whenTrue, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		whenFalse, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		branches[index] = Branch{Owner: owner, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse}
	}

	cellCount, err := decoder.count(artifactTermWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	cells := make([]keyspace.Term, cellCount)
	for index := range cells {
		cells[index], err = decoder.term()
		if err != nil {
			return ControlInput{}, err
		}
	}

	loopCount, err := decoder.count(artifactLoopWireMin)
	if err != nil {
		return ControlInput{}, err
	}
	loops := make([]Loop, loopCount)
	for index := range loops {
		owner, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		body, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		control, readErr := decoder.term()
		if readErr != nil {
			return ControlInput{}, readErr
		}
		loopKind, readErr := decoder.value(uint64(kind.LoopGenericFor))
		if readErr != nil {
			return ControlInput{}, readErr
		}
		cellRange, readErr := decoder.rangeFor(len(cells))
		if readErr != nil {
			return ControlInput{}, readErr
		}
		loops[index] = Loop{Owner: owner, Body: body, Control: control, Kind: kind.LoopKind(loopKind), Cells: cellRange}
	}
	return ControlInput{Returns: returns, Breaks: breaks, Labels: labels, Gotos: gotos, Branches: branches, Loops: loops, Cells: cells}, nil
}

func (decoder *artifactDecoder) claims() ([]ValueClaim, []TypeValue, error) {
	probeReader := *decoder.reader
	probe := artifactDecoder{reader: &probeReader}
	if err := probe.scanClaims(); err != nil {
		return nil, nil, err
	}
	return decoder.decodeClaims()
}

func (decoder *artifactDecoder) scanClaims() error {
	claimCount, err := decoder.count(artifactClaimWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < claimCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.term(); err != nil {
			return err
		}
		if _, err := decoder.value(uint64(kind.ValueClaimNonNil)); err != nil {
			return err
		}
	}
	typeValueCount, err := decoder.count(artifactOwnerWireMin)
	if err != nil {
		return err
	}
	for index := 0; index < typeValueCount; index++ {
		if _, err := decoder.term(); err != nil {
			return err
		}
	}
	return nil
}

func (decoder *artifactDecoder) decodeClaims() ([]ValueClaim, []TypeValue, error) {
	claimCount, err := decoder.count(artifactClaimWireMin)
	if err != nil {
		return nil, nil, err
	}
	claims := make([]ValueClaim, claimCount)
	for index := range claims {
		owner, readErr := decoder.term()
		if readErr != nil {
			return nil, nil, readErr
		}
		operand, readErr := decoder.term()
		if readErr != nil {
			return nil, nil, readErr
		}
		claimKind, readErr := decoder.value(uint64(kind.ValueClaimNonNil))
		if readErr != nil {
			return nil, nil, readErr
		}
		claims[index] = ValueClaim{Owner: owner, Operand: operand, Kind: kind.ValueClaimKind(claimKind)}
	}
	typeValueCount, err := decoder.count(artifactOwnerWireMin)
	if err != nil {
		return nil, nil, err
	}
	typeValues := make([]TypeValue, typeValueCount)
	for index := range typeValues {
		owner, readErr := decoder.term()
		if readErr != nil {
			return nil, nil, readErr
		}
		typeValues[index] = TypeValue{Owner: owner}
	}
	return claims, typeValues, nil
}

func maxIntValue() uint64 { return uint64(math.MaxInt) }
