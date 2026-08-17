package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (decoder *staticArtifactDecoder) publications(output *PublicationsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightPublications(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactPublicationWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Type = make([]Publication, count)
	}
	for index := 0; index < count; index++ {
		assign, err := decoder.term()
		if err != nil {
			return err
		}
		pair, err := decoder.uint32()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(assign) != keyspace.FamilyAssign || keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Type[index] = Publication{Assign: assign, Pair: pair, Target: target}
		}
	}
	return nil
}
