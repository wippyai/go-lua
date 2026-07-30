package factapply

import (
	"fmt"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func prepareRootAssignmentRegisteredScalars(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	point cfg.Point,
	targetPath pathdom.Path,
	source factflow.ValueSource,
	domain state.ProductDomain,
) (state.RootAssignmentScalarFactorTransaction, bool, error) {
	if reg == nil || resolver == nil || targetPath.IsEmpty() || targetPath.Symbol == 0 || !domain.Valid() || domain.Registry() != reg {
		return state.RootAssignmentScalarFactorTransaction{}, false, nil
	}
	targetKey, ok := visibility.AddressAt(resolver, point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return state.RootAssignmentScalarFactorTransaction{}, false, nil
	}
	config := state.RootAssignmentScalarTransferConfig{Keys: resolver.KeySpace(), Target: targetKey}
	if sourceKey, sourceOK := userLatticeSourceStateKey(resolver, point, facts, source); sourceOK {
		config.UserSource = sourceKey
	}
	if numeric, numericOK := sourcevalue.PlanNumericAffineSource(reg, resolver, point, facts, source); numericOK {
		if exact, exactOK := numeric.Exact(); exactOK {
			config.NumFloor = state.NewRootAssignmentNumBound(exact)
			config.NumCeil = state.NewRootAssignmentNumBound(exact)
		} else if sourceKey, offset, sourceOK := numeric.Source(); sourceOK {
			floor, floorErr := state.NewRootAssignmentNumBoundSource(sourceKey, offset)
			ceil, ceilErr := state.NewRootAssignmentNumBoundSource(sourceKey, offset)
			if floorErr != nil || ceilErr != nil {
				return state.RootAssignmentScalarFactorTransaction{}, false, fmt.Errorf("compile numeric source: floor=%v ceil=%v", floorErr, ceilErr)
			}
			config.NumFloor, config.NumCeil = floor, ceil
		}
	}
	transfer, err := state.SealRootAssignmentScalarTransfer(config)
	if err != nil {
		return state.RootAssignmentScalarFactorTransaction{}, false, err
	}
	transaction, err := domain.SealRootAssignmentScalarTransfer(transfer)
	if err != nil {
		return state.RootAssignmentScalarFactorTransaction{}, false, err
	}
	return transaction, true, nil
}

func userLatticeSourceStateKey(
	resolver *visibility.Resolver,
	point cfg.Point,
	facts factflow.Facts,
	source factflow.ValueSource,
) (pathaddr.StateKey, bool) {
	if source.Kind == factflow.ValueSourcePath && source.PathKey != "" {
		if stateKey, ok := pathaddr.StateKeyFromPathKey(source.PathKey); ok {
			return stateKey, true
		}
	}
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return "", false
	}
	return visibility.AddressAt(resolver, point, sourcePath).RootOrVisibleStateKey()
}
