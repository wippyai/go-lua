package specimen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/moduleload"
)

// ModuleLoadColumns are the owner column codecs the module-load binding reads
// and publishes through.
type ModuleLoadColumns struct {
	Candidate  *relbindgen.Column[valuedomain.ModuleLoadCall]
	Argument   *relbindgen.Column[valuedomain.Value]
	Dispatched *relbindgen.Column[calldomain.Value]
	Result     *relbindgen.Column[valuedomain.Value]
}

// ModuleLoadArgument is the decoded frame of one module-load judgment.
type ModuleLoadArgument struct {
	Candidate  valuedomain.ModuleLoadCall
	Argument   valuedomain.Value
	Dispatched calldomain.Value
}

// ModuleLoadOperation is the owner judgment. It holds the sealed judgment and
// nothing else: no relation, no cell, no buffer, no engine value is nameable
// from inside Evaluate.
type ModuleLoadOperation struct {
	judgment moduleload.Judgment
}

// Evaluate answers one candidate module load.
func (operation ModuleLoadOperation) Evaluate(argument ModuleLoadArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	result, reduction := operation.judgment.Result(argument.Candidate, argument.Argument, argument.Dispatched)
	switch reduction {
	case structure.Concrete:
		if !emitter.Put(result) {
			return outcome.Refused
		}
		return outcome.Produced
	case structure.AuthenticatedOpaque:
		if !emitter.PutOpaque(result) {
			return outcome.Refused
		}
		return outcome.Opaque
	case structure.NoCandidate:
		return outcome.NoCandidate
	case structure.NoSelection:
		return outcome.NoSelection
	default:
		return outcome.Refused
	}
}

// BindModuleLoad admits the scalar specimen: three scalar inputs, one row
// published at the candidate's own row.
func BindModuleLoad(operation signature.Signature, judgment moduleload.Judgment, columns ModuleLoadColumns, refusal model.RefusalID) (binding.Factory, bool) {
	if !judgment.Valid() || !columns.available() {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[ModuleLoadArgument, valuedomain.Value]{
		Signature: operation,
		Decoder:   moduleLoadDecoder{columns: columns},
		Encoder:   moduleLoadEncoder{result: columns.Result},
		Operation: ModuleLoadOperation{judgment: judgment},
		Address:   0,
		Refusal:   refusal,
	})
}

func (columns ModuleLoadColumns) available() bool {
	return columns.Candidate.Available() && columns.Argument.Available() && columns.Dispatched.Available() && columns.Result.Available()
}

type moduleLoadDecoder struct {
	columns ModuleLoadColumns
}

func (decoder moduleLoadDecoder) Decode(inputs relbindgen.Inputs) (ModuleLoadArgument, bool) {
	candidate, ok := relbindgen.ScalarAt(inputs, 0, decoder.columns.Candidate)
	if !ok {
		return ModuleLoadArgument{}, false
	}
	argument, ok := relbindgen.ScalarAt(inputs, 1, decoder.columns.Argument)
	if !ok {
		return ModuleLoadArgument{}, false
	}
	dispatched, ok := relbindgen.ScalarAt(inputs, 2, decoder.columns.Dispatched)
	if !ok {
		return ModuleLoadArgument{}, false
	}
	return ModuleLoadArgument{Candidate: candidate, Argument: argument, Dispatched: dispatched}, true
}

type moduleLoadEncoder struct {
	result *relbindgen.Column[valuedomain.Value]
}

func (encoder moduleLoadEncoder) Encode(outputs relbindgen.Outputs, value valuedomain.Value) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.result, value)
}
