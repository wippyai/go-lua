package projection

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func projectStateOwnedNormalReturnFacts(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	params []path.Path,
	visibleParams []path.Path,
	returns []exitFactReturnPath,
	captured map[symbol.ID]struct{},
) (callboundary.NormalReturnFacts, error) {
	keys := result.KeySpace()
	if reg == nil || keys == nil || !keys.Valid() {
		return callboundary.NormalReturnFacts{}, fmt.Errorf("projection: State normal-return projection has no registry/keyspace authority")
	}
	roots := make(state.BoundaryRoots, 0, len(params)*2+len(returns)*2+len(captured)*2)
	seenSymbols := make(map[symbol.ID]struct{}, len(params)+len(captured))
	for index, param := range params {
		if param.Symbol == 0 {
			return callboundary.NormalReturnFacts{}, fmt.Errorf("projection: parameter boundary root has no symbol")
		}
		var pathKey keyspace.Key
		if index < len(visibleParams) && !visibleParams[index].IsEmpty() {
			source := path.NewPath(param.Symbol, "")
			pathKey = keys.FromPath(source)
			if keys.FormatReadOnly(pathKey) == "" {
				return callboundary.NormalReturnFacts{}, fmt.Errorf("projection: parameter boundary root is outside keyspace")
			}
		}
		roots = append(roots, state.BoundaryRoot{Slot: key.SymbolValue(param.Symbol), Path: pathKey, Value: exit.ReadValue(reg, key.SymbolValue(param.Symbol))})
		seenSymbols[param.Symbol] = struct{}{}
	}
	for index, param := range params {
		if index >= len(visibleParams) || visibleParams[index].IsEmpty() {
			continue
		}
		source := path.NewPath(param.Symbol, "")
		source.Version = 1
		pathKey := keys.FromPath(source)
		if keys.FormatReadOnly(pathKey) == "" {
			continue
		}
		roots = append(roots, state.BoundaryRoot{Slot: key.SymbolValue(param.Symbol), Path: pathKey, Value: exit.ReadValue(reg, key.SymbolValue(param.Symbol))})
	}
	// Later equal-prefix roots win in the canonical converter. Reverse order
	// preserves the historical first-return-slot choice without a second State
	// lane projector.
	for index := len(returns) - 1; index >= 0; index-- {
		returned := returns[index]
		resultIndex, ok := callboundary.ReturnSlotIndex(returned.target)
		if !ok || returned.source.IsEmpty() {
			continue
		}
		pathKey := keys.FromPath(returned.source)
		if keys.FormatReadOnly(pathKey) == "" {
			continue
		}
		roots = append(roots, state.BoundaryRoot{Slot: key.ReturnSlot(resultIndex), Path: pathKey, Value: boundaryRootValue(reg, keys, exit, returned.source)})
		if returned.source.Symbol != 0 && returned.source.Version == 0 {
			versioned := returned.source.Clone()
			versioned.Version = 1
			versionedKey := keys.FromPath(versioned)
			if keys.FormatReadOnly(versionedKey) != "" {
				roots = append(roots, state.BoundaryRoot{Slot: key.ReturnSlot(resultIndex), Path: versionedKey, Value: boundaryRootValue(reg, keys, exit, versioned)})
			}
		}
	}
	var kinds symbolKindReader
	if reader, ok := result.(symbolKindReader); ok {
		kinds = reader
	}
	values := exit.ValuesSnapshot()
	visibleSlots := make([]key.Value, 0, len(values.Values))
	for slot := range values.Values {
		visibleSlots = append(visibleSlots, slot)
	}
	sort.Slice(visibleSlots, func(i, j int) bool { return visibleSlots[i] < visibleSlots[j] })
	for _, slot := range visibleSlots {
		sym, ok := key.ParseSymbolValue(slot)
		if !ok || !persistentSinkSymbol(kinds, captured, sym) {
			continue
		}
		if _, duplicate := seenSymbols[sym]; duplicate {
			continue
		}
		stable, ok := keys.FromStateKey(pathaddr.SymbolPathKey(sym, nil))
		if !ok {
			continue
		}
		value := values.Values[slot]
		roots = append(roots, state.BoundaryRoot{Slot: slot, Path: stable, Value: value})
		// State value cells use the stable symbol address, while path-keyed
		// lanes use the resolver's initial version. Both are views of the same
		// persistent capture/global boundary root. Supplying only the stable
		// spelling disconnects dynamic-index, member, and invalidation facts
		// from the boundary closure.
		resolver := path.NewPath(sym, "")
		resolver.Version = 1
		resolverKey := keys.FromPath(resolver)
		if keys.FormatReadOnly(resolverKey) != "" {
			roots = append(roots, state.BoundaryRoot{Slot: slot, Path: resolverKey, Value: value})
		}
		seenSymbols[sym] = struct{}{}
	}
	artifact, err := state.ProjectBoundary(reg, keys, exit, roots)
	if err != nil {
		return callboundary.NormalReturnFacts{}, err
	}
	world, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		return callboundary.NormalReturnFacts{}, err
	}
	return callboundary.NormalReturnFactsFromProjectedState(reg, keys, world, projectedRoots, len(params))
}

func boundaryRootValue(reg *axis.Registry, keys *keyspace.KeySpace, exit state.State, source path.Path) product.Value {
	if source.Symbol != 0 && len(source.Segments) == 0 {
		return exit.ReadValue(reg, key.SymbolValue(source.Symbol))
	}
	if sourceKey := keys.FromPath(source); keys.FormatReadOnly(sourceKey) != "" {
		return exit.ReadLocalPathKey(reg, sourceKey)
	}
	return product.Bottom(reg)
}
