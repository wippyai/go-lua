package program

import (
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// TailProducerKind is Program's closed proof classification for the only two
// authored occurrences permitted as an open Values tail.
type TailProducerKind uint8

const (
	TailProducerInvalid TailProducerKind = iota
	TailProducerCall
	TailProducerVararg
)

// TailProducer is an opaque exact Program proof for one existing executable
// Call or Vararg occurrence. Its authored coordinate remains private.
type TailProducer struct {
	input    TransformerInput
	span     Span
	kind     TailProducerKind
	path     keyspace.ContentID
	context  keyspace.ContentID
	catalog  *valuesCatalog
	rowIndex int
}

// issueTailProducer is a cold-only Program seal helper. It classifies the
// exact tail while the Values catalog is being published; the returned
// capability is not public until its catalog/row fence is installed.
func issueTailProducer(input TransformerInput, span Span) (TailProducer, bool) {
	if !input.OwnsSpan(span) || !input.owner.Flow().Executable().Contains(span.authored) {
		return TailProducer{}, false
	}
	var kind TailProducerKind
	switch keyspace.TermFamily(span.authored) {
	case keyspace.FamilyCall:
		_, _, _, _, ok := input.owner.Flow().Authored().Calls().Get(span.authored)
		if !ok {
			return TailProducer{}, false
		}
		kind = TailProducerCall
	case keyspace.FamilyVararg:
		_, _, ok := input.owner.Flow().Authored().Storage().Varargs().Get(span.authored)
		if !ok {
			return TailProducer{}, false
		}
		kind = TailProducerVararg
	default:
		return TailProducer{}, false
	}
	path, pathOK := input.owner.Flow().SemanticTermPath(span.authored)
	if !pathOK {
		return TailProducer{}, false
	}
	context := transformerSemanticID("program/transformer/tail-producer", func(writer *canonical.Writer) bool {
		return writer.Uint(uint64(kind)) == nil && writer.Bytes(path[:]) == nil
	})
	producer := TailProducer{input: input, span: span, kind: kind, path: path, context: context}
	return producer, true
}

// Available is the immutable receipt fence. Full authored classification was
// performed only by TransformerInput.TailProducer at issuance.
func (producer TailProducer) Available() bool {
	if producer.catalog == nil || !producer.catalog.valid() || producer.catalog.input != producer.input || producer.rowIndex < 0 || producer.rowIndex >= len(producer.catalog.rows) {
		return false
	}
	row := producer.catalog.rows[producer.rowIndex]
	return row.tail != 0 && row.tailProof.catalog == producer.catalog && row.tailProof.rowIndex == producer.rowIndex &&
		row.tailProof.input == producer.input && row.tailProof.span == producer.span &&
		row.tailProof.context == producer.context && row.tailProof.path == producer.path && row.tailProof.kind == producer.kind
}

func (producer TailProducer) Kind() TailProducerKind {
	if !producer.Available() {
		return TailProducerInvalid
	}
	return producer.kind
}

func (producer TailProducer) Span() (Span, bool) {
	if !producer.Available() {
		return Span{}, false
	}
	return producer.span, true
}

func (producer TailProducer) ContextID() keyspace.ContentID {
	if !producer.Available() {
		return keyspace.ContentID{}
	}
	return producer.context
}

func (input TransformerInput) OwnsTailProducer(producer TailProducer) bool {
	return input.Available() && producer.input == input && producer.Available()
}
