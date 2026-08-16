package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func signatureCallOperations(reg *axis.Registry, bindings *bind.Result, types *typeresolve.Resolver, graph cfg.Graph, facts factflow.Facts, plan *operationplan.Plan, producer *effectlowering.SignatureProducer) map[cfg.Point]operationplan.SignatureCallOperation {
	if reg == nil || bindings == nil || types == nil || graph == nil || plan == nil || producer == nil {
		return nil
	}
	out := make(map[cfg.Point]operationplan.SignatureCallOperation, facts.CallSiteCount())
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		site, ok := facts.CallSiteView(point)
		if !ok {
			continue
		}
		sig, ok := producer.SignatureForSite(transfer.NodeContext{Point: point}, site)
		if !ok && (exactGuardedStringMethodReceiver(reg, bindings, graph, facts, point, site) ||
			exactBoundaryStringMethodReceiver(reg, bindings, plan, site) ||
			exactDeclaredStringMethodReceiver(reg, bindings, types, plan, site)) {
			sig, ok = producer.LookupStringMethodSignature(site.MethodName())
			if ok {
				sig, ok = effectlowering.RefineStaticStringMethodSignature(reg, sig, site)
			}
			if ok {
				// Iterator-producing methods are owned by the generic-for protocol,
				// not by the scalar return projector. Admit their signature descriptor
				// exactly when this is an iterator-source site and the canonical effect
				// law exposes one active iterator. All other method calls retain the
				// existing finite scalar-return proof.
				_, collectionIterator := iteration.ActiveIterator(sig.Effect.Labels)
				_, callableIterator := effectlowering.CallableIteratorSignature(sig)
				if site.Context() != factflow.CallSiteContextIteratorSource || (!collectionIterator && !callableIterator) {
					_, ok = effectlowering.StaticScalarStringMethodReturns(reg, nil, sig, site)
				}
			}
		}
		if !ok {
			continue
		}
		intrinsic, hasIntrinsic := producer.IntrinsicForSite(transfer.NodeContext{Point: point}, site)
		var op operationplan.SignatureCallOperation
		if hasIntrinsic {
			op, ok = operationplan.NewSignatureIntrinsicCallOperation(sig, intrinsic)
		} else {
			op, ok = operationplan.NewSignatureCallOperation(sig)
		}
		if !ok {
			continue
		}
		out[point] = op
	}
	return out
}

// exactDeclaredStringMethodReceiver extends the canonical method-signature
// surface to an explicitly annotated local root. This is intentionally
// independent of solve-time value state: the annotation is the checked
// contract for every assignment to the local. Descendants, aliases, optional
// types, and missing annotations remain outside the signature surface.
func exactDeclaredStringMethodReceiver(reg *axis.Registry, bindings *bind.Result, types *typeresolve.Resolver, plan *operationplan.Plan, site factflow.CallSiteView) bool {
	if reg == nil || bindings == nil || types == nil || plan == nil {
		return false
	}
	receiver, ok := exactStringMethodReceiverRoot(site)
	if !ok {
		return false
	}
	if _, boundary := plan.BoundaryParamIndex(receiver.Symbol); boundary {
		return false
	}
	if _, capture := plan.BoundaryCaptureIndex(receiver.Symbol); capture {
		return false
	}
	annotation, ok := bindings.SymbolTypeAnnotation(receiver.Symbol)
	if !ok || annotation == nil {
		return false
	}
	declared, ok := types.Type(annotation)
	return ok && typ.TypeEquals(declared, typ.String)
}

// exactBoundaryStringMethodReceiver grants canonical string-method lookup only
// when the receiver is an immutable, unversioned root parameter whose plan-
// owned declared contract reconstructs to exactly string. The boundary product
// is the authority: any, optional string, descendants, writes, and path drift
// all fail closed rather than being inferred from source spelling.
func exactBoundaryStringMethodReceiver(reg *axis.Registry, bindings *bind.Result, plan *operationplan.Plan, site factflow.CallSiteView) bool {
	if reg == nil || bindings == nil || plan == nil || !plan.BoundaryParamsValid() {
		return false
	}
	receiver, ok := exactStringMethodReceiverRoot(site)
	if !ok || bindings.HasWrite(receiver.Symbol) {
		return false
	}
	params := plan.BoundaryParams()
	contracts := plan.BoundaryParamContracts()
	if len(params) == 0 || len(contracts) != len(params) {
		return false
	}
	for i, param := range params {
		if param != receiver.Symbol {
			continue
		}
		return product.Equal(reg, contracts[i], typevalue.String(reg))
	}
	return false
}

func staticScalarSignature(reg *axis.Registry, sig signature.Function) bool {
	_, exact := effectlowering.StaticScalarSignatureReturns(reg, nil, sig)
	return exact
}

// exactGuardedStringMethodReceiver recognizes only an unversioned root-path
// receiver with an exact string constraint on one CFG edge that dominates the
// call. Every reaching path must pass through that edge, and no root write may
// occur between proof and use. Joins, inverted guards, calls before the guard,
// alternate receivers, and version drift therefore fail closed.
func exactGuardedStringMethodReceiver(reg *axis.Registry, bindings *bind.Result, graph cfg.Graph, facts factflow.Facts, point cfg.Point, site factflow.CallSiteView) bool {
	if reg == nil || bindings == nil || graph == nil || site.MethodName() == "" {
		return false
	}
	receiver, ok := exactStringMethodReceiverRoot(site)
	if !ok || bindings.HasWrite(receiver.Symbol) {
		return false
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	reachability := cfg.NewReachability(graph)
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			edge := conditions[index]
			if !guardedMethodProofEdgeDominates(graph, dominators, branch, succ, point) || guardedMethodReceiverReassigned(facts, graph, reachability, succ, point, receiver) {
				continue
			}
			for _, refinement := range facts.BranchRefinements(branch) {
				if !refinement.TargetPathRef().Equal(receiver) {
					continue
				}
				valueRefinement, active := refinement.ValueForEdge(edge)
				constraint, constrained := valueRefinement.Constraint()
				if !active || !constrained {
					continue
				}
				receiverType, exact := typevalue.TypeOf(reg, constraint)
				if exact && receiverType != nil && typ.TypeEquals(receiverType, typ.String) {
					return true
				}
			}
		}
	}
	return false
}

func exactStringMethodReceiverRoot(site factflow.CallSiteView) (pathdom.Path, bool) {
	if site.MethodName() == "" {
		return pathdom.Path{}, false
	}
	receiver, ok := site.ReceiverPath()
	if !ok || receiver.Symbol == 0 || receiver.Version != 0 || len(receiver.Segments) != 0 {
		return pathdom.Path{}, false
	}
	methodPath, hasMethod := site.MethodPath()
	if !hasMethod || !methodPath.Equal(receiver.Field(site.MethodName())) || !site.CalleePathEqual(methodPath) {
		return pathdom.Path{}, false
	}
	receiverSource, hasSource := site.ReceiverSource()
	if !hasSource || !receiverSource.Valid() || receiverSource.Kind != factflow.ValueSourcePath ||
		receiverSource.PathKey != receiver.Key() || receiverSource.Expanded || receiverSource.Adjusted || receiverSource.OpenTail {
		return pathdom.Path{}, false
	}
	return receiver, true
}

func guardedMethodProofEdgeDominates(graph cfg.Graph, dominators *dominance.ImmediateDominators, branch, succ, point cfg.Point) bool {
	if dominators == nil || !dominators.Dominates(succ, point) {
		return false
	}
	for _, pred := range cfg.PredecessorsReadOnly(graph, succ) {
		if pred != branch && !dominators.Dominates(succ, pred) {
			return false
		}
	}
	return true
}

func guardedMethodReceiverReassigned(facts factflow.Facts, graph cfg.Graph, reachability *cfg.Reachability, from, to cfg.Point, receiver pathdom.Path) bool {
	for _, candidate := range graph.RPO() {
		if candidate == to || !reachability.CanReach(from, candidate) || !reachability.CanReach(candidate, to) {
			continue
		}
		if assignment, ok := facts.RootAssignment(candidate); ok && assignment.TargetSymbol() == receiver.Symbol {
			return true
		}
	}
	return false
}

func signatureAllocationOperations(plan *operationplan.Plan, owner uint64) map[cfg.Point]operationplan.SignatureAllocationOperation {
	if plan == nil || owner == 0 {
		return nil
	}
	out := make(map[cfg.Point]operationplan.SignatureAllocationOperation)
	for rawPoint := 0; rawPoint < plan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		call, ok := plan.SignatureCallOperation(point)
		if !ok {
			continue
		}
		template, exact := effectlowering.StaticSignatureAllocationTemplate(call.Signature())
		if !exact {
			continue
		}
		op, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
			Owner: owner, Template: template.Root, Ordinal: uint32(point),
		}, template)
		if ok {
			out[point] = op
		}
	}
	return out
}
