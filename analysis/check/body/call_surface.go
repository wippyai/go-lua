package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// sealPreparedCallSurface independently enumerates lowering-owned calls and
// classifies their stable lexical targets. Facts are consulted only to bind an
// enumerated call to its semantic site; a missing fact remains an explicit
// rejected site rather than disappearing from the census.
func sealPreparedCallSurface(bindings *bind.Result, lowered *wir.Body, facts factflow.Facts, signatureCalls map[cfg.Point]operationplan.SignatureCallOperation, owner lexicalidentity.StableLexicalBodyID, namespace lexicalidentity.UnitNamespace, pointCount int) operationplan.CallSurface {
	if bindings == nil || lowered == nil || owner == (lexicalidentity.StableLexicalBodyID{}) || namespace == (lexicalidentity.UnitNamespace{}) {
		return operationplan.CallSurface{}
	}
	var extracted []cfg.Point
	var sites []operationplan.CallSurfaceSite
	lowered.ForEachCall(func(instruction wir.Instruction) bool {
		extracted = append(extracted, instruction.Point)
		target := preparedGuardedCallResidue(lowered, instruction)
		if lexical, exact := exactPreparedLexicalCallTarget(bindings, namespace, lowered, instruction, facts); exact {
			target, _ = operationplan.NewLexicalCallSurfaceTarget(lexical)
		} else if external, exact := signatureCalls[instruction.Point]; exact && exactPreparedExternalCallShape(lowered, instruction, facts) {
			if sealed, ok := operationplan.NewExternalCallSurfaceTarget(external); ok {
				target = sealed
			}
		}
		sites = append(sites, operationplan.CallSurfaceSite{Point: instruction.Point, Target: target})
		return true
	})
	surface, err := operationplan.SealCallSurface(owner, pointCount, extracted, sites)
	if err != nil {
		return operationplan.CallSurface{}
	}
	return surface
}

func preparedGuardedCallResidue(lowered *wir.Body, instruction wir.Instruction) operationplan.CallSurfaceTarget {
	if lowered == nil {
		return operationplan.RejectedCallSurfaceTarget()
	}
	if instruction.Call.Method != 0 {
		method := lowered.Const(instruction.Call.Method)
		if instruction.Call.Receiver.Kind == wir.OperandPath && method.Kind == wir.ConstString {
			receiver := lowered.Path(wir.PathRef(instruction.Call.Receiver.Ref))
			return operationplan.RejectedMethodCallSurfaceTarget(receiver.Key(), method.Str)
		} else if instruction.Call.Receiver.Kind == wir.OperandTemp {
			return operationplan.RejectedTemporaryCallSurfaceTarget(instruction.Call.Receiver.Ref)
		}
		return operationplan.RejectedCallSurfaceTarget()
	}
	switch instruction.Call.Callee.Kind {
	case wir.OperandPath:
		callee := lowered.Path(wir.PathRef(instruction.Call.Callee.Ref))
		return operationplan.RejectedPathCallSurfaceTarget(callee.Key())
	case wir.OperandTemp:
		return operationplan.RejectedTemporaryCallSurfaceTarget(instruction.Call.Callee.Ref)
	default:
		return operationplan.RejectedCallSurfaceTarget()
	}
}

func exactPreparedExternalCallShape(lowered *wir.Body, instruction wir.Instruction, facts factflow.Facts) bool {
	if lowered == nil {
		return false
	}
	if instruction.Call.Method != 0 {
		return exactPreparedExternalMethodCallShape(lowered, instruction, facts)
	}
	if instruction.Call.Callee.Kind != wir.OperandPath || instruction.Call.Receiver.Kind != wir.OperandNone {
		return false
	}
	calleePath := lowered.Path(wir.PathRef(instruction.Call.Callee.Ref))
	if calleePath.IsEmpty() || calleePath.Symbol == 0 {
		return false
	}
	site, ok := facts.CallSiteView(instruction.Point)
	if !ok || site.CalleeSymbol() != calleePath.Symbol || !site.CalleePathEqual(calleePath) ||
		site.CalleeMemberAccess() != (len(calleePath.Segments) != 0) || site.MethodName() != "" {
		return false
	}
	if _, method := site.ReceiverPath(); method {
		return false
	}
	if _, method := site.ReceiverSource(); method {
		return false
	}
	if _, method := site.MethodPath(); method {
		return false
	}
	return true
}

func exactPreparedExternalMethodCallShape(lowered *wir.Body, instruction wir.Instruction, facts factflow.Facts) bool {
	if instruction.Call.Callee.Kind != wir.OperandNone || instruction.Call.Receiver.Kind != wir.OperandPath || instruction.Call.Method == 0 {
		return false
	}
	method := lowered.Const(instruction.Call.Method)
	if method.Kind != wir.ConstString || method.Str == "" {
		return false
	}
	receiver := lowered.Path(wir.PathRef(instruction.Call.Receiver.Ref))
	if receiver.IsEmpty() || receiver.Symbol == 0 {
		return false
	}
	methodPath := receiver.Field(method.Str)
	site, ok := facts.CallSiteView(instruction.Point)
	if !ok || site.CalleeSymbol() != 0 || !site.CalleeMemberAccess() || site.MethodName() != method.Str || !site.CalleePathEqual(methodPath) {
		return false
	}
	siteReceiver, hasReceiver := site.ReceiverPath()
	siteMethod, hasMethod := site.MethodPath()
	memberReceiver, member, hasMember := site.CalleeMemberAccessPath()
	if !hasReceiver || !siteReceiver.Equal(receiver) || !hasMethod || !siteMethod.Equal(methodPath) ||
		!hasMember || !memberReceiver.Equal(receiver) || member.Kind != segment.SegmentField || member.Name != method.Str {
		return false
	}
	source, hasSource := site.ReceiverSource()
	return hasSource && exactPreparedReceiverSourcePath(facts, source, receiver)
}

func exactPreparedReceiverSourcePath(facts factflow.Facts, source factflow.ValueSource, receiver pathdom.Path) bool {
	switch source.Kind {
	case factflow.ValueSourcePath:
		return source.Valid() && source.PathKey == receiver.Key()
	case factflow.ValueSourceExpression:
		path, ok := facts.ExpressionPath(source.ExprRef)
		return source.Valid() && ok && path.Equal(receiver)
	default:
		return false
	}
}

func exactPreparedLexicalCallTarget(bindings *bind.Result, namespace lexicalidentity.UnitNamespace, lowered *wir.Body, instruction wir.Instruction, facts factflow.Facts) (lexicalidentity.StableLexicalBodyID, bool) {
	if instruction.Call.Callee.Kind != wir.OperandPath || instruction.Call.Receiver.Kind != wir.OperandNone || instruction.Call.Method != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	path := lowered.Path(wir.PathRef(instruction.Call.Callee.Ref))
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	site, ok := facts.CallSiteView(instruction.Point)
	if !ok || site.CalleeSymbol() != path.Symbol || site.CalleeMemberAccess() || site.MethodName() != "" || site.TypeArgCount() != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	calleePath := site.CalleePathRef()
	if calleePath.Symbol != path.Symbol || len(calleePath.Segments) != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	// Keep the local safety checks at the consumer boundary even though the
	// binder query also fails closed. The surface must never classify a global
	// or reassigned binding as a lexical direct target.
	kind, kindOK := bindings.Kind(path.Symbol)
	if !kindOK || kind == symbol.Global || bindings.HasWrite(path.Symbol) {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	functionIdentity, exact := bindings.StableLocalFunctionIdentity(path.Symbol)
	if !exact {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	target := lexicalidentity.FunctionBody(namespace, uint64(functionIdentity))
	return target, target != (lexicalidentity.StableLexicalBodyID{})
}
