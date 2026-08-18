package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
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

// writePublicationsContent owns the exact authored Assign-pair-to-TypeRef
// relation. Duplicate-detection state and any future export projection are
// deliberately absent.
func writePublicationsContent(writer *framing.Writer, rows []publicationRow) error {
	if err := writer.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Uint(uint64(row.assign)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.pair)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
	}
	return nil
}
