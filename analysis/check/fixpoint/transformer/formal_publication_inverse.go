package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type formalPublicationEnvironment uint8

const (
	formalPublicationPointInput formalPublicationEnvironment = iota + 1
	formalPublicationPointOutput
)

// freezeFormalPointPublicationInverse seals the exact resolver-root
// environment represented by one formal relation cell. The generic registered
// lane law remains body-wide; only this finite root selection is point-owned.
func freezeFormalPointPublicationInverse(body *relationProgramBody, span formalFiberDescriptorSpan, point cfg.Point, environment formalPublicationEnvironment) (state.CoordinateFormalPublicationProjection, error) {
	if body == nil || !body.productDomain.Valid() || body.pathSemantics == nil || !body.pathSemantics.Valid() ||
		body.graph == nil || span.variable != body.variable ||
		(environment != formalPublicationPointInput && environment != formalPublicationPointOutput) {
		return state.CoordinateFormalPublicationProjection{}, fmt.Errorf("transformer: formal point publication environment is unowned")
	}
	if uint64(point) >= uint64(body.graph.Size()) {
		return state.CoordinateFormalPublicationProjection{}, fmt.Errorf("transformer: formal point publication environment is unowned")
	}
	node := body.graph.Node(point)
	if node == nil || node.Point != point {
		return state.CoordinateFormalPublicationProjection{}, fmt.Errorf("transformer: formal point publication environment is unowned")
	}
	required, err := body.productDomain.CoordinateFormalRoots(span.rekey)
	if err != nil {
		return state.CoordinateFormalPublicationProjection{}, err
	}
	values, present := span.valuesGroup()
	if !present {
		return state.CoordinateFormalPublicationProjection{}, fmt.Errorf("transformer: formal point publication has no Values vocabulary")
	}
	rootByFormal := make(map[formal.Root]Root, len(values.descriptor.valueSlots))
	for _, member := range values.descriptor.valueSlots {
		key, exact := member.slot.Root()
		root, rootExact := member.slot.relationRoot()
		if exact && rootExact {
			rootByFormal[key] = root
		}
	}
	bindings := make([]state.CoordinateFormalInverseRootBinding, 0, len(required))
	for _, formalRoot := range required {
		// Structural Input and Output roots are inverted from the exact source
		// roots sealed in the body-wide forward law. Only resolver-backed Middle
		// roots vary by publication point.
		if formalRoot.Vocabulary() != formal.Middle {
			continue
		}
		root, exact := rootByFormal[formalRoot]
		if !exact {
			return state.CoordinateFormalPublicationProjection{}, fmt.Errorf("transformer: formal publication root has no relation identity")
		}
		concrete, resolverBacked, concreteErr := formalPointResolverRoot(body, point, environment, root)
		if concreteErr != nil {
			return state.CoordinateFormalPublicationProjection{}, concreteErr
		}
		if resolverBacked {
			bindings = append(bindings, state.CoordinateFormalInverseRootBinding{Source: formalRoot, Target: concrete})
		}
	}
	return body.productDomain.SealCoordinateFormalPublicationProjection(span.rekey, span.coordinates, bindings)
}

func formalPointResolverRoot(body *relationProgramBody, point cfg.Point, environment formalPublicationEnvironment, root Root) (keyspace.Key, bool, error) {
	visible := func(path pathdom.Path) (keyspace.Key, bool) {
		if environment == formalPublicationPointInput {
			return body.pathSemantics.VisibleInputLocalPathKey(point, path)
		}
		return body.pathSemantics.VisibleLocalPathKey(point, path)
	}
	if root.Kind != RootMiddle || int(root.Index) >= len(body.relation.arena.middle.registers) {
		return keyspace.Key{}, false, nil
	}
	register := body.relation.arena.middle.registers[root.Index]
	if register.kind != relationMiddleRegisterSymbol {
		return keyspace.Key{}, false, nil
	}
	concrete, _ := visible(pathdom.NewPath(register.symbol, ""))
	if concrete.Kind == keyspace.KindInvalid {
		return keyspace.Key{}, false, nil
	}
	structural, exact := body.keys.StructuralRoot(concrete)
	if !exact || structural != concrete {
		return keyspace.Key{}, false, fmt.Errorf("transformer: formal publication Middle root %#v is not structural", root)
	}
	return concrete, true, nil
}
