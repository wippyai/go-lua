package static

import (
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
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
