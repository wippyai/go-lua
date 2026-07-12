package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SummaryIndexBase is the owned, immutable lookup surface shared by every
// summary-backed call provider for one program-key set. Summary values still
// come from the live summary.Reader; this only owns static routing indexes so
// providers do not repeatedly defensively clone the same maps while the fixpoint
// is running.
type SummaryIndexBase struct {
	functionKeys  map[symbol.ID]summary.SummaryKey
	functionIDs   map[identity.ID]summary.SummaryKey
	pathKeys      map[factflow.CalleePathKey]summary.SummaryKey
	pathMultiKeys map[factflow.CalleePathKey][]summary.SummaryKey
	functionTypes map[summary.SummaryKey]*typ.Function
}

// SummaryIndex adds an owner-local callback expression index to a shared base.
type SummaryIndex struct {
	*SummaryIndexBase
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey
	owner                  summary.SummaryKey
}

// SummaryIndexConfig describes the raw maps that should be copied into a
// SummaryIndex. After construction, callers may mutate their input maps without
// changing provider behavior.
type SummaryIndexConfig struct {
	FunctionKeys           map[symbol.ID]summary.SummaryKey
	FunctionExpressionKeys map[factflow.ExprRef]summary.SummaryKey
	FunctionIDs            map[identity.ID]summary.SummaryKey
	PathKeys               map[factflow.CalleePathKey]summary.SummaryKey
	PathMultiKeys          map[factflow.CalleePathKey][]summary.SummaryKey
	FunctionTypes          map[summary.SummaryKey]*typ.Function
}

// SummaryIndexBaseConfig describes the raw maps that are shared by every owner
// for one program-key set.
type SummaryIndexBaseConfig struct {
	FunctionKeys  map[symbol.ID]summary.SummaryKey
	FunctionIDs   map[identity.ID]summary.SummaryKey
	PathKeys      map[factflow.CalleePathKey]summary.SummaryKey
	PathMultiKeys map[factflow.CalleePathKey][]summary.SummaryKey
	FunctionTypes map[summary.SummaryKey]*typ.Function
}

// NewSummaryIndex copies config into a reusable read-only summary lookup index.
func NewSummaryIndex(config SummaryIndexConfig) *SummaryIndex {
	base := NewSummaryIndexBase(SummaryIndexBaseConfig{
		FunctionKeys:  config.FunctionKeys,
		FunctionIDs:   config.FunctionIDs,
		PathKeys:      config.PathKeys,
		PathMultiKeys: config.PathMultiKeys,
		FunctionTypes: config.FunctionTypes,
	})
	return base.WithFunctionExpressionKeys(config.FunctionExpressionKeys)
}

// NewSummaryIndexBase copies the owner-independent summary routing maps.
func NewSummaryIndexBase(config SummaryIndexBaseConfig) *SummaryIndexBase {
	return &SummaryIndexBase{
		functionKeys:  mapedit.Clone(config.FunctionKeys),
		functionIDs:   mapedit.Clone(config.FunctionIDs),
		pathKeys:      mapedit.Clone(config.PathKeys),
		pathMultiKeys: clonePathMultiKeys(config.PathMultiKeys),
		functionTypes: mapedit.Clone(config.FunctionTypes),
	}
}

// WithFunctionExpressionKeys returns an index for one owner using this shared
// base and an owned copy of the owner-local callback-expression map.
func (base *SummaryIndexBase) WithFunctionExpressionKeys(keys map[factflow.ExprRef]summary.SummaryKey) *SummaryIndex {
	if base == nil {
		base = NewSummaryIndexBase(SummaryIndexBaseConfig{})
	}
	return &SummaryIndex{
		SummaryIndexBase:       base,
		functionExpressionKeys: mapedit.Clone(keys),
	}
}

// WithOwnerFunctionExpressionKeys returns an owner-scoped index. The owner is
// also the stable caller scope for returned-allocation instantiation.
func (base *SummaryIndexBase) WithOwnerFunctionExpressionKeys(owner summary.SummaryKey, keys map[factflow.ExprRef]summary.SummaryKey) *SummaryIndex {
	out := base.WithFunctionExpressionKeys(keys)
	out.owner = owner
	return out
}

func summaryIndexFromProviderConfig(config ProviderConfig) *SummaryIndex {
	if config.Index != nil {
		return config.Index
	}
	return NewSummaryIndex(SummaryIndexConfig{
		FunctionKeys:           config.FunctionKeys,
		FunctionExpressionKeys: config.FunctionExpressionKeys,
		FunctionIDs:            config.FunctionIDs,
		PathKeys:               config.PathKeys,
		PathMultiKeys:          config.PathMultiKeys,
		FunctionTypes:          config.FunctionTypes,
	})
}
