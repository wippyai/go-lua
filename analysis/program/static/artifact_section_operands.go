package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

func (decoder *staticArtifactDecoder) operands(output *OperandsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightOperands(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactClaimWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Claim = make([]ClaimTarget, count)
	}
	var previous keyspace.Term
	for index := 0; index < count; index++ {
		claim, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(claim) != keyspace.FamilyValueClaim ||
			(index != 0 && keyspace.TermOrdinal(claim) <= keyspace.TermOrdinal(previous)) {
			return errInvalidArtifactSection
		}
		if !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		previous = claim
		if !decoder.probing {
			output.Claim[index] = ClaimTarget{Claim: claim, Target: target}
		}
	}

	count, err = decoder.count(staticArtifactTypeValueWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeValue = make([]TypeValueTarget, count)
	}
	for index := 0; index < count; index++ {
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(target) != keyspace.FamilyTypePrimitive && keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeValue[index] = TypeValueTarget{Target: target}
		}
	}

	count, err = decoder.count(staticArtifactAnnotationWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Annotation = make([]Annotation, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		values, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) ||
			!staticrole.AnnotationTargetFamily(keyspace.TermFamily(target)) ||
			keyspace.TermFamily(values) != keyspace.FamilyValues {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Annotation[index] = Annotation{Scope: scope, Target: target, Name: name, Values: values}
		}
	}
	return nil
}

// writeOperandsContent owns the sparse ClaimTarget relation and the two
// dense sidecars. CSR rows and dense claim lookup are derived indexes, so are
// intentionally excluded.
func writeOperandsContent(writer *framing.Writer, store operandsStore) error {
	if err := writer.Count(uint64(len(store.claims))); err != nil {
		return err
	}
	for _, row := range store.claims {
		if err := writer.Uint(uint64(row.claim)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.typeValues))); err != nil {
		return err
	}
	for _, target := range store.typeValues {
		if err := writer.Uint(uint64(target)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.annotations))); err != nil {
		return err
	}
	for _, row := range store.annotations {
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
