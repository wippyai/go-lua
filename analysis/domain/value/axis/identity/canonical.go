package identity

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const canonicalValueRecord uint64 = 1

func canonicalDescriptor() axis.CanonicalDescriptor[Value] {
	return axis.ReadyCanonicalBidirectional("value.axis.identity", 2, encodeCanonical, decodeCanonical)
}

func decodeCanonical(_ context.Context, reader *canonical.Reader) (Value, error) {
	record, err := reader.Record()
	if err != nil {
		return Bottom(), err
	}
	if record != canonicalValueRecord {
		return Bottom(), fmt.Errorf("identity: invalid canonical record %d", record)
	}
	rawState, err := reader.Uint()
	if err != nil {
		return Bottom(), err
	}
	if rawState > uint64(top) {
		return Bottom(), fmt.Errorf("identity: invalid canonical state")
	}
	decodedState := state(rawState)
	if decodedState == singleton {
		term, err := decodeCanonicalTerm(reader)
		if err != nil {
			return Bottom(), err
		}
		if !term.Valid() {
			return Value{state: decodedState}, nil
		}
		return SingletonTerm(term), nil
	}
	// Bottom and Top have no payload in the constructor-reachable carrier.
	return Value{state: decodedState}, nil
}

// encodeCanonical writes exactly the state observed by Equal. Only singleton
// state gives the stored ID semantic meaning; all other states ignore it.
func encodeCanonical(writer *canonical.Writer, v Value) error {
	if err := writer.Record(canonicalValueRecord); err != nil {
		return err
	}
	if err := writer.Uint(uint64(v.state)); err != nil {
		return err
	}
	if v.state != singleton {
		return nil
	}
	return encodeCanonicalTerm(writer, v.term)
}

func decodeCanonicalTerm(reader *canonical.Reader) (Term, error) {
	rawKind, err := reader.Uint()
	if err != nil {
		return Term{}, err
	}
	switch TermKind(rawKind) {
	case TermConcrete:
		kind, err := reader.String()
		if err != nil {
			return Term{}, err
		}
		site, err := reader.String()
		if err != nil {
			return Term{}, err
		}
		index, err := reader.Uint()
		if err != nil {
			return Term{}, err
		}
		term := ConcreteTerm(ID{Kind: kind, Site: site, Index: index})
		if !term.Valid() {
			return Term{}, fmt.Errorf("identity: invalid canonical concrete term")
		}
		return term, nil
	case TermFormal:
		owner, err := reader.Bytes()
		if err != nil {
			return Term{}, err
		}
		ordinal, err := reader.Uint()
		if err != nil {
			return Term{}, err
		}
		vocabulary, err := reader.Uint()
		if err != nil {
			return Term{}, err
		}
		var body lexicalidentity.StableLexicalBodyID
		if len(owner) != len(body) {
			return Term{}, fmt.Errorf("identity: invalid formal owner")
		}
		copy(body[:], owner)
		term := FormalTerm(NewFormalVar(NewFormalSchemaID(body, ordinal), formal.Vocabulary(vocabulary)))
		if !term.Valid() {
			return Term{}, fmt.Errorf("identity: invalid canonical formal term")
		}
		return term, nil
	case TermAllocation:
		owner, err := reader.Bytes()
		if err != nil {
			return Term{}, err
		}
		allocation, err := reader.Uint()
		if err != nil {
			return Term{}, err
		}
		object, err := reader.Uint()
		if err != nil {
			return Term{}, err
		}
		var body lexicalidentity.StableLexicalBodyID
		if len(owner) != len(body) {
			return Term{}, fmt.Errorf("identity: invalid allocation owner")
		}
		copy(body[:], owner)
		term := AllocationTerm(ManifestAllocationTemplate(body, uint32(allocation), uint32(object)))
		if !term.Valid() || allocation > uint64(^uint32(0)) || object > uint64(^uint32(0)) {
			return Term{}, fmt.Errorf("identity: invalid canonical allocation term")
		}
		return term, nil
	case TermInvalid:
		return Term{}, nil
	default:
		return Term{}, fmt.Errorf("identity: invalid canonical term kind")
	}
}

func encodeCanonicalTerm(writer *canonical.Writer, term Term) error {
	if !term.Valid() {
		return writer.Uint(uint64(TermInvalid))
	}
	if err := writer.Uint(uint64(term.kind)); err != nil {
		return err
	}
	switch term.kind {
	case TermConcrete:
		if err := writer.String(term.concrete.Kind); err != nil {
			return err
		}
		if err := writer.String(term.concrete.Site); err != nil {
			return err
		}
		return writer.Uint(term.concrete.Index)
	case TermFormal:
		root := term.formal.root
		owner := root.Owner()
		if err := writer.Bytes(owner[:]); err != nil {
			return err
		}
		if err := writer.Uint(root.Ordinal()); err != nil {
			return err
		}
		return writer.Uint(uint64(root.Vocabulary()))
	case TermAllocation:
		if err := writer.Bytes(term.allocation.owner[:]); err != nil {
			return err
		}
		if err := writer.Uint(uint64(term.allocation.allocation)); err != nil {
			return err
		}
		return writer.Uint(uint64(term.allocation.object))
	default:
		return fmt.Errorf("identity: invalid singleton term")
	}
}
