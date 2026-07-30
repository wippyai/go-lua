package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// freezeOutboundFrameRootSchema records only caller addresses. Callee values
// return through its own formal roots; re-evaluating caller value terms after
// the child coordinate has advanced can pair a newer caller alternative with
// an older callee outcome and is neither necessary nor exact.
func freezeOutboundFrameRootSchema(reg *axis.Registry, caller *relationProgramBody, frame *linkedRelationFrame) (state.BoundaryRoots, error) {
	if reg == nil || caller == nil || frame == nil || caller.relation.arena == nil || len(frame.rootCircuit) != frame.shape.InputCount() {
		return nil, fmt.Errorf("transformer: outbound frame schema is unowned")
	}
	roots := make(state.BoundaryRoots, 0, frame.inputRootCount())
	for index, wire := range frame.rootCircuit {
		root := state.BoundaryRoot{Value: product.Bottom(reg)}
		if wire.path != 0 {
			path, slot, syntax, exact := outboundFramePathTerm(caller, wire.path)
			if !exact {
				return nil, fmt.Errorf("transformer: outbound frame path %d has no structural image", index)
			}
			root.Path, root.Slot = path, slot
			if err := freezeOutboundFrameVisiblePath(caller, frame.point, wire.root.Kind, index, "path", syntax, &root.Path); err != nil {
				return nil, err
			}
			if root.Path.Kind == keyspace.KindInvalid || caller.keys.FormatReadOnly(root.Path) == "" {
				return nil, fmt.Errorf("transformer: outbound frame path %d is outside caller keyspace", index)
			}
		} else {
			node := caller.relation.arena.values[wire.value]
			switch node.op {
			case valueRoot:
				for _, carrier := range caller.roots.roots {
					if carrier.root == node.root {
						root.Slot, root.Path = carrier.slot, carrier.path
						break
					}
				}
			case valueEnvironment:
				root.Slot = node.slot
				path, syntax, exact := outboundFrameValueSlotPath(caller, node.slot)
				if exact {
					root.Path = path
					if err := freezeOutboundFrameVisiblePath(caller, frame.point, wire.root.Kind, index, "value", syntax, &root.Path); err != nil {
						return nil, err
					}
				}
			}
		}
		roots = append(roots, root)
	}
	for index, wire := range frame.ambientCircuit {
		path, slot, syntax, exact := outboundFramePathTerm(caller, wire.path)
		if !exact {
			return nil, fmt.Errorf("transformer: outbound ambient path %d has no structural image", index)
		}
		root := state.BoundaryRoot{Value: product.Bottom(reg), Slot: slot, Path: path}
		if root.Slot == 0 {
			root.Slot = statekey.SymbolValue(wire.target.symbol)
		}
		if err := freezeOutboundFrameVisiblePath(caller, frame.point, RootCapture, index, "ambient path", syntax, &root.Path); err != nil {
			return nil, err
		}
		if root.Path.Kind == keyspace.KindInvalid || caller.keys.FormatReadOnly(root.Path) == "" {
			return nil, fmt.Errorf("transformer: outbound ambient path %d is outside caller keyspace", index)
		}
		roots = append(roots, root)
	}
	return roots, nil
}

// outboundFramePathTerm freezes the structural image of one already-sealed
// path term. Executable path evaluation is intentionally not used: Middle is
// a value register, while rootPathKey is the canonical MID/input boundary
// lens. Segment extension remains owned by KeySpace.
func outboundFramePathTerm(caller *relationProgramBody, term PathTerm) (keyspace.Key, statekey.Value, pathdom.Path, bool) {
	if caller == nil || caller.relation.arena == nil || caller.keys == nil || !caller.keys.Valid() ||
		term == 0 || int(term) >= len(caller.relation.arena.paths) {
		return keyspace.Key{}, 0, pathdom.Path{}, false
	}
	node := caller.relation.arena.paths[term]
	var base keyspace.Key
	var slot statekey.Value
	if node.environment != 0 {
		slot = statekey.SymbolValue(node.environment)
		if !caller.relation.arena.validEnvironmentSlot(slot) {
			return keyspace.Key{}, 0, pathdom.Path{}, false
		}
		base = caller.keys.FromPath(pathdom.NewPath(node.environment, ""))
	} else {
		var exact bool
		base, exact = caller.rootPathKey(node.root)
		if !exact {
			return keyspace.Key{}, 0, pathdom.Path{}, false
		}
		slot, _ = caller.rootValueSlot(node.root)
	}
	path := base
	syntax, syntaxExact := caller.keys.StatePath(base)
	if syntaxExact {
		for _, suffix := range node.segments {
			syntax = syntax.Append(suffix)
		}
		path = caller.keys.FromPath(syntax)
	} else {
		var exact bool
		for _, suffix := range node.segments {
			path, exact = caller.keys.AppendSegment(path, suffix)
			if !exact {
				return keyspace.Key{}, 0, pathdom.Path{}, false
			}
		}
	}
	if len(node.segments) != 0 {
		slot = 0
	}
	if !syntaxExact {
		syntax, _ = caller.keys.StatePath(path)
	}
	return path, slot, syntax, path.Kind != keyspace.KindInvalid && caller.keys.FormatReadOnly(path) != ""
}

func outboundFrameValueSlotPath(caller *relationProgramBody, slot statekey.Value) (keyspace.Key, pathdom.Path, bool) {
	if caller == nil || caller.relation.arena == nil || slot == 0 {
		return keyspace.Key{}, pathdom.Path{}, false
	}
	root, exact := caller.relation.arena.middleRoot(slot)
	if !exact {
		return keyspace.Key{}, pathdom.Path{}, false
	}
	path, exact := caller.rootPathKey(root)
	if !exact {
		return keyspace.Key{}, pathdom.Path{}, false
	}
	syntax, _ := caller.keys.StatePath(path)
	return path, syntax, true
}

func freezeOutboundFrameVisiblePath(
	caller *relationProgramBody,
	point cfg.Point,
	targetKind RootKind,
	index int,
	role string,
	syntax pathdom.Path,
	path *keyspace.Key,
) error {
	if syntax.IsEmpty() {
		// Private call-result carriers are already point-owned structural roots;
		// they deliberately have no source-language path to visibility-resolve.
		return nil
	}
	if targetKind == RootParam {
		if caller.pathSemantics == nil || !caller.pathSemantics.Valid() {
			return fmt.Errorf("transformer: outbound parameter %s %d has no visibility authority", role, index)
		}
		address, err := caller.pathSemantics.FreezePathAddress(point, syntax)
		if err != nil {
			return fmt.Errorf("transformer: outbound parameter %s %d: %w", role, index, err)
		}
		visible, exact := address.LocalKey()
		if !exact {
			return fmt.Errorf("transformer: outbound parameter %s %d has no point-visible root", role, index)
		}
		*path = visible
		return nil
	}
	if targetKind == RootCapture && caller.pathSemantics != nil && caller.pathSemantics.Valid() {
		if address, err := caller.pathSemantics.FreezePathAddress(point, syntax); err == nil {
			if visible, exact := address.LocalKey(); exact {
				*path = visible
			}
		}
	}
	return nil
}
