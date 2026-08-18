package static

import (
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

func (decoder *staticArtifactDecoder) operators(output *OperatorsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightOperators(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactTypeOfWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeOf = make([]TypeOf, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) {
			return errInvalidArtifactSection
		}
		operand, err := decoder.term()
		if err != nil {
			return err
		}
		// TypeOf's operand is a cross-owner Flow value occurrence. Reject
		// static nodes, storage handles, and Module Import terms at decode time;
		// Build performs the counted ordinal check after root denominators are
		// injected.
		if !flowrole.ValueOccurrenceFamily(keyspace.TermFamily(operand)) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeOf[index] = TypeOf{Scope: scope, Operand: operand}
		}
	}

	count, err = decoder.count(staticArtifactKeyOfWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.KeyOf = make([]KeyOf, count)
	}
	for index := 0; index < count; index++ {
		inner, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.KeyOf[index] = KeyOf{Inner: inner}
		}
	}

	count, err = decoder.count(staticArtifactIndexAccessWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.IndexAccess = make([]IndexAccess, count)
	}
	for index := 0; index < count; index++ {
		object, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		indexTerm, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.IndexAccess[index] = IndexAccess{Object: object, Index: indexTerm}
		}
	}

	count, err = decoder.count(staticArtifactConditionalWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Conditional = make([]Conditional, count)
	}
	for index := 0; index < count; index++ {
		check, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		extends, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		then, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		elseTerm, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Conditional[index] = Conditional{Check: check, Extends: extends, Then: then, Else: elseTerm}
		}
	}
	return nil
}

// writeOperatorsContent owns all four exact authored static operator rows.
func writeOperatorsContent(writer *framing.Writer, store operatorsStore) error {
	if err := writer.Count(uint64(len(store.typeOf))); err != nil {
		return err
	}
	for _, row := range store.typeOf {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Operand)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.keyOf))); err != nil {
		return err
	}
	for _, row := range store.keyOf {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.indexAccess))); err != nil {
		return err
	}
	for _, row := range store.indexAccess {
		if err := writer.Uint(uint64(row.Object)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Index)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.conditional))); err != nil {
		return err
	}
	for _, row := range store.conditional {
		if err := writer.Uint(uint64(row.Check)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Extends)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Then)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Else)); err != nil {
			return err
		}
	}
	return nil
}
