package target

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"
)

func (d *operationDraft) freezeSubedgeArgumentOrigins(input []vocabulary.ArgumentOrigin, arguments valuesDraft) ([]subedgeArgumentOriginDraft, error) {
	want := len(arguments.types) + len(arguments.suffix)
	if arguments.tail == vocabulary.ValuesVariable {
		want++
	}
	// An empty set deliberately means this contextual endpoint is wholly fed by
	// sibling/admission routes. That alternative is checked after local refs
	// resolve; accepting a partial set here would create an implicit merge.
	if len(input) == 0 {
		return nil, nil
	}
	if len(input) != want {
		return nil, errors.New("argument origins are incomplete")
	}
	if _, err := vocabulary.CheckedStoredLength("subedge argument origin table", len(input)); err != nil {
		return nil, err
	}
	out := make([]subedgeArgumentOriginDraft, len(input))
	for index, item := range input {
		if err := d.validateArgumentOrigin(item, arguments); err != nil {
			return nil, fmt.Errorf("argument origin %d: %w", index, err)
		}
		out[index] = subedgeArgumentOriginDraft{
			segment: item.Segment, index: item.Index, kind: item.Kind, source: item.Source,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareArgumentOrigin(out[left], out[right]) < 0 })
	for index := range out {
		if index != 0 && out[index-1].segment == out[index].segment && out[index-1].index == out[index].index {
			return nil, errors.New("duplicate argument origin")
		}
		if !argumentOriginExpected(out[index], arguments) {
			return nil, errors.New("argument origin does not name a Values segment")
		}
	}
	return out, nil
}

func (d *operationDraft) validateArgumentOrigin(origin vocabulary.ArgumentOrigin, arguments valuesDraft) error {
	if !argumentOriginExpected(subedgeArgumentOriginDraft{segment: origin.Segment, index: origin.Index}, arguments) {
		return errors.New("segment outside argument Values")
	}
	switch origin.Kind {
	case vocabulary.ArgumentSourceRule:
		if origin.Source != (vocabulary.InputSource{}) {
			return errors.New("rule origin carries direct input")
		}
		return nil
	case vocabulary.ArgumentSourceInput:
		return d.validateDirectArgumentOrigin(origin, arguments)
	default:
		return errors.New("invalid argument source")
	}
}

func (d *operationDraft) validateDirectArgumentOrigin(origin vocabulary.ArgumentOrigin, arguments valuesDraft) error {
	switch origin.Segment {
	case vocabulary.ArgumentFixed, vocabulary.ArgumentSuffix:
		if origin.Source.Kind != vocabulary.InputSourceValueFormal || uint64(origin.Source.Ordinal) >= uint64(len(d.input.types)) {
			return errors.New("fixed argument origin is not an owner ValueFormal")
		}
		destination, ok := argumentSegmentType(arguments, origin.Segment, origin.Index)
		if !ok {
			return errors.New("fixed argument origin is type-incompatible")
		}
		sourceType, sourceOK := d.declarations[d.input.types[origin.Source.Ordinal]]
		destinationType, destinationOK := d.declarations[destination]
		if !sourceOK || !destinationOK {
			return errors.New("fixed argument origin type relation: type declaration is not admitted")
		}
		assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
		if relationErr != nil {
			return fmt.Errorf("fixed argument origin type relation: %w", relationErr)
		}
		if !assignable {
			return errors.New("fixed argument origin is type-incompatible")
		}
		return nil
	case vocabulary.ArgumentTail:
		if origin.Source.Kind != vocabulary.InputSourceValuesVar || d.input.tail != vocabulary.ValuesVariable || arguments.tail != vocabulary.ValuesVariable ||
			origin.Source.Ordinal != uint32(d.input.varID) || arguments.varID != d.input.varID {
			return errors.New("tail argument origin is not the owner input tail")
		}
		if d.input.tailType != arguments.tailType {
			return errors.New("tail argument origin is type-incompatible")
		}
		return nil
	default:
		return errors.New("invalid argument segment")
	}
}

func argumentOriginExpected(origin subedgeArgumentOriginDraft, values valuesDraft) bool {
	switch origin.segment {
	case vocabulary.ArgumentFixed:
		return uint64(origin.index) < uint64(len(values.types))
	case vocabulary.ArgumentSuffix:
		return uint64(origin.index) < uint64(len(values.suffix))
	case vocabulary.ArgumentTail:
		return origin.index == 0 && values.tail == vocabulary.ValuesVariable
	default:
		return false
	}
}

func argumentSegmentType(values valuesDraft, segment vocabulary.ArgumentSegment, index uint32) (string, bool) {
	switch segment {
	case vocabulary.ArgumentFixed:
		if uint64(index) < uint64(len(values.types)) {
			return values.types[index], true
		}
	case vocabulary.ArgumentSuffix:
		if uint64(index) < uint64(len(values.suffix)) {
			return values.suffix[index], true
		}
	}
	return "", false
}

func compareArgumentOrigin(left, right subedgeArgumentOriginDraft) int {
	if left.segment < right.segment {
		return -1
	}
	if left.segment > right.segment {
		return 1
	}
	if left.index < right.index {
		return -1
	}
	if left.index > right.index {
		return 1
	}
	return 0
}
