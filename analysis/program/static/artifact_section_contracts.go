package static

import "github.com/wippyai/go-lua/internal/framing"

func (decoder *staticArtifactDecoder) contracts(output *ContractsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightContracts(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactContractFunctionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Function = make([]FunctionContract, count)
	}
	for index := 0; index < count; index++ {
		typeParams, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		returnsKnown, err := decoder.boolean()
		if err != nil {
			return err
		}
		returns, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		returnsCount := decoder.lastTermCount
		if !returnsKnown && returnsCount != 0 {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Function[index] = FunctionContract{TypeParams: typeParams, ReturnsKnown: returnsKnown, Returns: returns}
		}
	}

	count, err = decoder.count(staticArtifactContractCallWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Call = make([]CallContract, count)
	}
	for index := 0; index < count; index++ {
		typeArguments, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Call[index] = CallContract{TypeArguments: typeArguments}
		}
	}
	return nil
}

// writeContractsContent owns the dense static sidecars for opaque Flow
// Function and Call identities. It hashes semantic sequences, never their
// shared-pool offsets.
func writeContractsContent(writer *framing.Writer, store contractsStore) error {
	if err := writer.Count(uint64(len(store.functions))); err != nil {
		return err
	}
	for _, row := range store.functions {
		if err := writeTypeTermsContent(writer, store.terms[row.typeParams.Start:row.typeParams.End]); err != nil {
			return err
		}
		if err := writer.Bool(row.returnsKnown); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.terms[row.returns.Start:row.returns.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.calls))); err != nil {
		return err
	}
	for _, row := range store.calls {
		if err := writeTypeTermsContent(writer, store.terms[row.Start:row.End]); err != nil {
			return err
		}
	}
	return nil
}
