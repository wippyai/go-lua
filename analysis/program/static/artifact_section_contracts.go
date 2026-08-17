package static

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
