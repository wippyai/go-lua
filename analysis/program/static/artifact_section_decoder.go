package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/internal/framing"

	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

type staticArtifactDecoder struct {
	reader        *framing.Reader
	probing       bool
	preflighted   bool
	lastTermCount int
	lastKeyCount  int
}

func (decoder *staticArtifactDecoder) preflightSection() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordTypes); err != nil {
		return err
	}
	if err := statictypes.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordReferences); err != nil {
		return err
	}
	if err := staticrefs.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordDeclarations); err != nil {
		return err
	}
	if err := staticdecl.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordSignatures); err != nil {
		return err
	}
	if err := staticsig.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordContracts); err != nil {
		return err
	}
	if err := staticcontracts.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordOperators); err != nil {
		return err
	}
	if err := staticoperators.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordOperands); err != nil {
		return err
	}
	if err := staticoperands.Scan(probe.reader); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordPublications); err != nil {
		return err
	}
	if err := staticpubs.Scan(probe.reader); err != nil {
		return err
	}
	return nil
}

// probeReader makes a value copy of framing.Reader. Reader's semantic
// methods advance only that copy, so a complete collection can be checked
// without touching the live stream or allocating any decoded rows. The
// subsequent real pass consumes the same bytes and uses the same row schema.
func (decoder *staticArtifactDecoder) probeReader() (staticArtifactDecoder, error) {
	if decoder == nil || decoder.reader == nil {
		return staticArtifactDecoder{}, framing.ErrMalformed
	}
	reader := *decoder.reader
	probe := staticArtifactDecoder{reader: &reader, probing: true}
	return probe, nil
}

func (decoder *staticArtifactDecoder) record(want uint64) error {
	if decoder == nil || decoder.reader == nil {
		return framing.ErrMalformed
	}
	got, err := decoder.reader.Record()
	if err != nil {
		return err
	}
	if got != want {
		return errInvalidArtifactSection
	}
	return nil
}

// count is the allocation gate for every decoded collection.  The ordinal
// ceiling is deliberately stricter than a machine's int/uint32 ceiling, and
// the remaining-byte floor ensures an adversarial arity cannot reserve more
// rows than the unread canonical stream can possibly contain.
func (decoder *staticArtifactDecoder) count(rowMinimum uint64) (int, error) {
	value, err := decoder.reader.Count()
	if err != nil {
		return 0, err
	}
	if value > uint64(keyspace.MaxTermOrdinal) ||
		value > uint64(^uint(0)>>1) ||
		rowMinimum == 0 || value > uint64(decoder.reader.Remaining())/rowMinimum {
		return 0, errInvalidArtifactSection
	}
	return int(value), nil
}

func (decoder *staticArtifactDecoder) uint32() (uint32, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, errInvalidArtifactSection
	}
	return uint32(value), nil
}

func (decoder *staticArtifactDecoder) enum(max uint64) (uint8, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value == 0 || value > max {
		return 0, errInvalidArtifactSection
	}
	return uint8(value), nil
}

func (decoder *staticArtifactDecoder) term() (keyspace.Term, error) {
	value, err := decoder.uint32()
	if err != nil {
		return 0, err
	}
	term := keyspace.Term(value)
	if term != 0 && keyspace.TermFamily(term) == keyspace.FamilyInvalid {
		return 0, errInvalidArtifactSection
	}
	return term, nil
}

type staticArtifactTermConstraint uint8

const (
	staticArtifactAnyTerm staticArtifactTermConstraint = iota + 1
	staticArtifactStaticNodeTerm
	staticArtifactTypeRefTerm
	staticArtifactTypeParamTerm
	staticArtifactTypeFieldTerm
)

func validDecodedTerm(term keyspace.Term, constraint staticArtifactTermConstraint) bool {
	family := keyspace.TermFamily(term)
	if family == keyspace.FamilyInvalid {
		return false
	}
	switch constraint {
	case staticArtifactAnyTerm:
		return true
	case staticArtifactStaticNodeTerm:
		return staticrole.NodeFamily(family)
	case staticArtifactTypeRefTerm:
		return family == keyspace.FamilyTypeRef
	case staticArtifactTypeParamTerm:
		return family == keyspace.FamilyTypeParam
	case staticArtifactTypeFieldTerm:
		return family == keyspace.FamilyTypeField
	default:
		return false
	}
}

func (decoder *staticArtifactDecoder) constrainedTerm(constraint staticArtifactTermConstraint) (keyspace.Term, error) {
	term, err := decoder.term()
	if err != nil {
		return 0, err
	}
	if !validDecodedTerm(term, constraint) {
		return 0, errInvalidArtifactSection
	}
	return term, nil
}

func (decoder *staticArtifactDecoder) key() (keyspace.Key, error) {
	value, err := decoder.uint32()
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, errInvalidArtifactSection
	}
	return keyspace.Key(value), nil
}

func (decoder *staticArtifactDecoder) boolean() (bool, error) {
	return decoder.reader.Bool()
}

func (decoder *staticArtifactDecoder) coordinate() (source.Coordinate, error) {
	startLine, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	startColumn, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	endLine, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	endColumn, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	coordinate, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	if !ok {
		return source.Coordinate{}, errInvalidArtifactSection
	}
	return coordinate, nil
}

func (decoder *staticArtifactDecoder) termSequenceConstraint(minimum int, constraint staticArtifactTermConstraint) ([]keyspace.Term, error) {
	if !decoder.probing && !decoder.preflighted {
		probe, err := decoder.probeReader()
		if err != nil {
			return nil, err
		}
		if _, err := probe.termSequenceConstraint(minimum, constraint); err != nil {
			return nil, err
		}
	}
	count, err := decoder.count(staticArtifactUintWireMin)
	if err != nil {
		return nil, err
	}
	if count < minimum {
		return nil, errInvalidArtifactSection
	}
	decoder.lastTermCount = count
	var terms []keyspace.Term
	if !decoder.probing {
		terms = make([]keyspace.Term, count)
	}
	for index := 0; index < count; index++ {
		term, readErr := decoder.constrainedTerm(constraint)
		if readErr != nil {
			return nil, readErr
		}
		if !decoder.probing {
			terms[index] = term
		}
	}
	return terms, nil
}
