package readmodel

import (
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (r Reader) assignmentCallResultSource(source sourceprovenance.ASTSource) readapi.CallResultAssignmentSource {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return readapi.CallResultAssignmentSource{}
	}
	site, ok := r.result.CallSite(source.CallPoint)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	_, ok = contract.Contract.ResultAt(source.ResultIndex)
	name := contract.Source.Name
	if name == "" {
		name = r.callContractSourceName(site)
	}
	if !ok {
		return readapi.CallResultAssignmentSource{
			Present:       true,
			CallableName:  name,
			ResultIndex:   source.ResultIndex,
			UnderSupplied: true,
		}
	}
	return readapi.CallResultAssignmentSource{
		Present:      true,
		CallableName: name,
		ResultIndex:  source.ResultIndex,
		ReturnSpan:   contract.Source.ResultSpan(source.ResultIndex),
	}
}

// CallResultSourceType projects a source-provenance call result through the
// canonical solved call contract. It is the readmodel-owned replacement for
// diagnostics re-lowering function contracts from syntax.
func (r Reader) CallResultSourceType(source sourceprovenance.ASTSource) (typ.Type, bool) {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return nil, false
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return nil, false
	}
	ret, ok := contract.Contract.ResultAt(source.ResultIndex)
	if !ok {
		return typ.Nil, true
	}
	if ret.Type == nil || refinement.ContainsFreeTypeParam(ret.Type) {
		return nil, false
	}
	return ret.Type, true
}

func (r Reader) callResultSourceUnderSupplied(source sourceprovenance.ASTSource) bool {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return false
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return false
	}
	_, ok = contract.Contract.ResultAt(source.ResultIndex)
	return !ok
}
