package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// SourceParamAnnotated reports whether fn's source parameter carries an explicit
// annotation. Annotated parameters keep their declared contract and are not
// inferred from caller entry evidence.
func SourceParamAnnotated(fn *ast.FunctionExpr, sourceParam int) bool {
	if fn == nil || fn.ParList == nil || sourceParam < 0 || sourceParam >= len(fn.ParList.Types) {
		return false
	}
	return fn.ParList.Types[sourceParam] != nil
}

// ParamSlotForSourceParam maps a source parameter index to the runtime graph
// parameter slot it fills. It consumes the graph's canonical ParamSlots layout so
// implicit receiver slots do not drift from the CFG/binder model.
func ParamSlotForSourceParam(g *cfg.Graph, fn *ast.FunctionExpr, sourceParam int) (int, int, bool) {
	if fn == nil || fn.ParList == nil || sourceParam < 0 || sourceParam >= len(fn.ParList.Names) {
		return 0, 0, false
	}
	slot := sourceParam
	if g != nil {
		for i, ps := range g.ParamSlotsReadOnly() {
			if srcIdx, ok := ps.SourceParamIndex(); ok && srcIdx == sourceParam {
				slot = i
				break
			}
		}
	}
	return sourceParam, slot, true
}

// ParamSlotForCallArg maps a concrete call argument to the callee source
// parameter and runtime slot it fills.
//
// The runtime slot is the primary identity: source parameters can be shifted by
// implicit method receivers, while the product fixed point stores entry values
// and body demands by slot. Colon-call syntax supplies the receiver before the
// listed Args, so call.Args[0] fills runtime slot 1; plain calls fill slot 0.
func ParamSlotForCallArg(g *cfg.Graph, fn *ast.FunctionExpr, call *ast.FuncCallExpr, argIdx int) (int, int, bool) {
	if argIdx < 0 {
		return 0, 0, false
	}
	slotIdx := argIdx
	if call != nil && call.Method != "" {
		slotIdx++
	}
	return ParamSlotForRuntimeArg(g, fn, slotIdx)
}

// ParamSlotForRuntimeArg maps an already-normalized runtime argument index to
// the callee source parameter and runtime slot it fills. Runtime index 0 is the
// receiver for method calls and the first argument for plain calls.
func ParamSlotForRuntimeArg(g *cfg.Graph, fn *ast.FunctionExpr, runtimeIdx int) (int, int, bool) {
	if runtimeIdx < 0 {
		return 0, 0, false
	}
	if g != nil {
		slots := g.ParamSlotsReadOnly()
		if runtimeIdx >= 0 && runtimeIdx < len(slots) {
			return slots[runtimeIdx].SourceIndex, runtimeIdx, true
		}
	}
	return ParamSlotForSourceParam(g, fn, runtimeIdx)
}

// CallArgContractConfig supplies the callee-owned facts needed to project a
// concrete call site's argument obligations. The projection is source/slot aware:
// method receiver offsets and implicit slots come from the callee CFG's ParamSlots
// layout, not from ad hoc driver indexing.
type CallArgContractConfig struct {
	Graph                *cfg.Graph
	Function             *ast.FunctionExpr
	Call                 *ast.FuncCallExpr
	Contracts            Contracts
	DeclaredSlotType     func(slot int) typ.Type
	EntrySlotType        func(slot int) typ.Type
	SourceParamAnnotated func(sourceParam int) bool
}

// CallArgContractTypes projects declared parameter contracts plus solved body
// demands onto the actual argument expressions of one call. It is the call-boundary
// counterpart to entry seeding: signatures stay signatures, while Summary.Params
// remain obligations consumed at the call edge.
func CallArgContractTypes(config CallArgContractConfig) []typ.Type {
	return callobligation.Types(CallArgContractObligations(config))
}

// CallArgContractObligations projects declared parameter contracts plus solved
// body demands onto the actual argument expressions of one call, preserving the
// caller-boundary policy of each obligation.
func CallArgContractObligations(config CallArgContractConfig) []callobligation.Obligation {
	if config.Graph == nil || config.Function == nil || config.Call == nil || len(config.Call.Args) == 0 {
		return nil
	}
	out := make([]callobligation.Obligation, len(config.Call.Args))
	for argIdx := range config.Call.Args {
		source, slot, ok := ParamSlotForCallArg(config.Graph, config.Function, config.Call, argIdx)
		if !ok {
			continue
		}
		declared := callArgDeclaredType(config.DeclaredSlotType, slot)
		contract := callArgContract(config.Contracts, slot)
		entry := callArgEntryType(config.EntrySlotType, slot)
		if callArgSourceAnnotated(config, source) {
			out[argIdx] = callobligation.Signature(declared)
			continue
		}
		out[argIdx] = mergeCallArgObligations(declared, contract, entry)
	}
	return callobligation.Normalize(out)
}

// FunctionCallArgContractTypes projects a concrete callable signature onto the
// listed call arguments. It is the signature-owned half of call-edge obligation
// projection; body-demand summary obligations use CallArgContractTypes.
func FunctionCallArgContractTypes(call *ast.FuncCallExpr, fn *typ.Function) []typ.Type {
	return callobligation.Types(FunctionCallArgObligations(call, fn))
}

// FunctionCallArgObligations projects a concrete callable signature onto the
// listed call arguments. Signature obligations remain gradual-consistent.
func FunctionCallArgObligations(call *ast.FuncCallExpr, fn *typ.Function) []callobligation.Obligation {
	return FunctionCallArgObligationsWithSelf(call, fn, nil)
}

// FunctionCallArgObligationsWithSelf projects a callable signature onto concrete
// call arguments after substituting free Self references with the resolved runtime
// receiver type when the call site has one.
func FunctionCallArgObligationsWithSelf(call *ast.FuncCallExpr, fn *typ.Function, selfType typ.Type) []callobligation.Obligation {
	if call == nil || fn == nil || len(call.Args) == 0 {
		return nil
	}
	out := make([]callobligation.Obligation, len(call.Args))
	offset := 0
	if call.Method != "" {
		offset = 1
	}
	for i := range call.Args {
		idx := i + offset
		switch {
		case idx < len(fn.Params):
			if !fn.Params[idx].Optional {
				out[i] = callobligation.Signature(callArgSignatureType(fn.Params[idx].Type, selfType))
			}
		case fn.Variadic != nil:
			out[i] = callobligation.Signature(callArgSignatureType(fn.Variadic, selfType))
		}
	}
	return callobligation.Normalize(out)
}

func callArgSignatureType(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return subst.Self(t, selfType)
}

// JoinCallArgContractTypes accumulates next into out pointwise using the hard
// contract join. The vector index is the concrete call argument index; nil,
// unknown, absent, and any carry no caller obligation.
func JoinCallArgContractTypes(out []typ.Type, next []typ.Type) []typ.Type {
	if len(next) == 0 {
		return out
	}
	if len(out) < len(next) {
		grown := make([]typ.Type, len(next))
		copy(grown, out)
		out = grown
	}
	for i, demand := range next {
		if !InformativeCallArgContract(demand) {
			continue
		}
		out[i] = JoinCallArgContractType(out[i], demand)
	}
	return out
}

// JoinCallArgObligations accumulates next into out pointwise using the hard
// contract join while preserving strict body provenance.
func JoinCallArgObligations(out []callobligation.Obligation, next []callobligation.Obligation) []callobligation.Obligation {
	if len(next) == 0 {
		return out
	}
	if len(out) < len(next) {
		grown := make([]callobligation.Obligation, len(next))
		copy(grown, out)
		out = grown
	}
	for i, demand := range next {
		if !demand.Informative() {
			continue
		}
		out[i] = JoinCallArgObligation(out[i], demand)
	}
	return out
}

// JoinCallArgObligation joins two obligations for the same concrete call
// argument. Body strictness wins if either side came from Summary.Params.
func JoinCallArgObligation(prev, next callobligation.Obligation) callobligation.Obligation {
	if !prev.Informative() {
		return next
	}
	if !next.Informative() {
		return prev
	}
	source := callobligation.JoinSource(prev.Source, next.Source)
	if joined := HardContractJoin(prev.Type, next.Type); joined != nil {
		return callArgObligationWithSource(joined, source, joinObligationContracts(prev, next))
	}
	return prev
}

// JoinCallArgContractType joins two obligations for the same concrete call
// argument. If the hard join cannot prove a stronger shared obligation, the
// existing obligation is retained to avoid fabricating a stricter caller
// contract.
func JoinCallArgContractType(prev, next typ.Type) typ.Type {
	if !InformativeCallArgContract(prev) {
		return next
	}
	if !InformativeCallArgContract(next) {
		return prev
	}
	if joined := HardContractJoin(prev, next); joined != nil {
		return joined
	}
	return prev
}

// NormalizeCallArgContractTypes canonicalizes an all-empty vector to nil.
func NormalizeCallArgContractTypes(in []typ.Type) []typ.Type {
	for _, t := range in {
		if InformativeCallArgContract(t) {
			return in
		}
	}
	return nil
}

// InformativeCallArgContract reports whether t carries an enforceable caller
// obligation.
func InformativeCallArgContract(t typ.Type) bool {
	return callobligation.InformativeType(t)
}

func callArgSourceAnnotated(config CallArgContractConfig, source int) bool {
	if source < 0 {
		return false
	}
	if config.SourceParamAnnotated != nil {
		return config.SourceParamAnnotated(source)
	}
	return SourceParamAnnotated(config.Function, source)
}

func callArgDeclaredType(resolve func(slot int) typ.Type, slot int) typ.Type {
	if resolve == nil || slot < 0 {
		return nil
	}
	return informativeOrNil(resolve(slot))
}

func callArgEntryType(resolve func(slot int) typ.Type, slot int) typ.Type {
	if resolve == nil || slot < 0 {
		return nil
	}
	return informativeOrNil(resolve(slot))
}

func callArgContractType(contracts Contracts, slot int) typ.Type {
	contract := callArgContract(contracts, slot)
	if ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
		return nil
	}
	return informativeOrNil(contract.ProjectValue())
}

func callArgContract(contracts Contracts, slot int) ParamContract {
	if slot < 0 || len(contracts) == 0 {
		return ParamContractDomain.Bottom()
	}
	contract, ok := contracts[slot]
	if !ok || ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
		return ParamContractDomain.Bottom()
	}
	return contract
}

func mergeCallArgContracts(declared, contract typ.Type) typ.Type {
	return mergeCallArgObligations(declared, DemandFromType(contract), nil).Type
}

func mergeCallArgObligations(declared typ.Type, contract ParamContract, entry typ.Type) callobligation.Obligation {
	declared = informativeOrNil(declared)
	contractType := informativeOrNil(contract.ProjectValue())
	entry = informativeOrNil(entry)
	if declared == nil {
		if EntryContradictsBodyContract(entry, contractType) {
			return callobligation.Obligation{}
		}
		return bodyContractObligation(contractType, contract)
	}
	if contractType == nil {
		return callobligation.Signature(declared)
	}
	if EntryContradictsBodyContract(entry, contractType) {
		return callobligation.Signature(declared)
	}
	if joined := HardContractJoin(declared, contractType); joined != nil {
		shape := ParamContractDomain.Join(DemandFromType(declared), contract)
		return bodyContractObligation(joined, shape)
	}
	return callobligation.Signature(declared)
}

func callArgObligationWithSource(t typ.Type, source callobligation.Source, contract ParamContract) callobligation.Obligation {
	if source == callobligation.SourceBody {
		return bodyContractObligation(t, contract)
	}
	return callobligation.Signature(t)
}

func bodyContractObligation(t typ.Type, contract ParamContract) callobligation.Obligation {
	if !ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
		return callobligation.BodyShape(t, contract)
	}
	return callobligation.Body(t)
}

func joinObligationContracts(prev, next callobligation.Obligation) ParamContract {
	contract := ParamContractDomain.Bottom()
	for _, obligation := range []callobligation.Obligation{prev, next} {
		if obligation.Source == callobligation.SourceSignature {
			contract = ParamContractDomain.Join(contract, DemandFromType(obligation.Type))
			continue
		}
		if obligation.Source != callobligation.SourceBody {
			continue
		}
		if shaped, ok := obligationParamContract(obligation); ok {
			contract = ParamContractDomain.Join(contract, shaped)
			continue
		}
		contract = ParamContractDomain.Join(contract, DemandFromType(obligation.Type))
	}
	return contract
}

func obligationParamContract(obligation callobligation.Obligation) (ParamContract, bool) {
	switch shape := obligation.Shape.(type) {
	case ParamContract:
		return shape, !ParamContractDomain.Equal(shape, ParamContractDomain.Bottom())
	case *ParamContract:
		if shape == nil {
			return ParamContractDomain.Bottom(), false
		}
		return *shape, !ParamContractDomain.Equal(*shape, ParamContractDomain.Bottom())
	default:
		return ParamContractDomain.Bottom(), false
	}
}

func informativeOrNil(t typ.Type) typ.Type {
	if informativeContractType(t) {
		return t
	}
	return nil
}

func informativeContractType(t typ.Type) bool {
	return callobligation.InformativeType(t)
}
