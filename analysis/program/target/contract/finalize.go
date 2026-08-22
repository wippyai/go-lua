package contract

import (
	"errors"

	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	exactkeyvalue "github.com/wippyai/go-lua/analysis/program/target/exactkey"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
)

// Input is the complete set of immutable subordinate owners admitted to one
// Contract. Compiler constructs these values and then hands them over once;
// Contract never exposes a partially assembled value or a second builder.
type Input struct {
	Table      bootvalue.Table
	Operations operationvalue.Core
	Protocols  protocolvalue.Table
	ExactKeys  exactkeyvalue.Table
}

// New atomically finalizes one contract from already sealed subordinate
// values. Semantic identities, host reverse indexes, and denominator rows are
// built inside this constructor and become private immutable Contract state
// before the pointer escapes.
func New(input Input) (*Contract, error) {
	if input.Operations.OperationCount() == 0 {
		return nil, errors.New("target/contract: unavailable operation core")
	}
	contract := &Contract{
		Table: input.Table, Operations: input.Operations,
		protocols: input.Protocols, exactKeys: input.ExactKeys,
	}
	if err := contract.sealSemanticIdentities(); err != nil {
		return nil, err
	}
	contract.sealed = true
	counts, err := contract.publishCountRows()
	if err != nil {
		return nil, err
	}
	contract.counts = counts
	// The whole-contract identity is derived here, once, after every table the
	// canonical encoding reads is final. A contract whose identity cannot be
	// derived is malformed and never escapes this constructor.
	id, err := contract.sealContentID()
	if err != nil {
		return nil, err
	}
	contract.contractContentID = id
	return contract, nil
}
