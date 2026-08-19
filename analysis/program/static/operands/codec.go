package operands

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	claimWireMin      = wire.UintWireMin * 2
	typeValueWireMin  = wire.UintWireMin
	annotationWireMin = wire.UintWireMin * 4
)

// WriteContent emits the sparse ClaimTarget relation and the two dense
// sidecars. The dense claim lookup and the annotation query index are derived
// indexes and are intentionally excluded from the stream.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.claim.Count())); err != nil {
		return err
	}
	for _, row := range table.claim.All() {
		if err := writer.Uint(uint64(row.Claim)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.typeValue.Count())); err != nil {
		return err
	}
	for _, target := range table.typeValue.All() {
		if err := writer.Uint(uint64(target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.annotation.Count())); err != nil {
		return err
	}
	for _, row := range table.annotation.All() {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Values)); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Operands vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Operands vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input
	count, err := wire.Count(reader, claimWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Claim = make([]ClaimTarget, count)
	}
	// The sparse relation travels in canonical claim order, so the stream
	// carries a strictly ascending sequence and cannot smuggle a duplicate.
	var previous keyspace.Term
	for index := 0; index < count; index++ {
		claim, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		target, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(claim) != keyspace.FamilyValueClaim ||
			(index != 0 && keyspace.TermOrdinal(claim) <= keyspace.TermOrdinal(previous)) {
			return Input{}, framing.ErrMalformed
		}
		previous = claim
		if retain {
			input.Claim[index] = ClaimTarget{Claim: claim, Target: target}
		}
	}

	count, err = wire.Count(reader, typeValueWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeValue = make([]TypeValueTarget, count)
	}
	for index := 0; index < count; index++ {
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(target) != keyspace.FamilyTypePrimitive &&
			keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.TypeValue[index] = TypeValueTarget{Target: target}
		}
	}

	count, err = wire.Count(reader, annotationWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Annotation = make([]Annotation, count)
	}
	for index := 0; index < count; index++ {
		scope, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		name, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		values, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) ||
			!staticrole.AnnotationTargetFamily(keyspace.TermFamily(target)) ||
			keyspace.TermFamily(values) != keyspace.FamilyValues {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.Annotation[index] = Annotation{Scope: scope, Target: target, Name: name, Values: values}
		}
	}
	return input, nil
}
