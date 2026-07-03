package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

type paramSinkExposureFactMap = factmap.Map[pathaddr.RootPlaceholderKey, ParamSinkExposure, product.Value]

var paramSinkExposureMaps registrycache.Cache[paramSinkExposureFactMap]

// paramSinkExposureMap is the canonical may (union) map lattice for param-to-sink
// exposures: an exposure is a MAY fact (any normal-return path that stores the
// parameter into a persistent sink exposes the argument), so exposures survive a
// join when present on either path, and two exposures of the same source widen to
// the join of their carried sink-slot contracts.
func paramSinkExposureMap(reg *axis.Registry) paramSinkExposureFactMap {
	return paramSinkExposureMaps.GetFor(reg, newParamSinkExposureMap)
}

func newParamSinkExposureMap(reg *axis.Registry) paramSinkExposureFactMap {
	return paramSinkExposureFactMap{
		Key:   func(e ParamSinkExposure) pathaddr.RootPlaceholderKey { return e.Source },
		Value: func(e ParamSinkExposure) product.Value { return e.Contract },
		WithValue: func(e ParamSinkExposure, v product.Value) ParamSinkExposure {
			e.Contract = v
			return e
		},
		Less: func(a, b ParamSinkExposure) bool { return a.Source < b.Source },
		Valid: func(e ParamSinkExposure) bool {
			return e.Source.Valid() && !product.Equal(reg, e.Contract, product.Bottom(reg)) && !product.Equal(reg, e.Contract, product.Top())
		},
		Domain:  product.Domain(reg),
		Collide: func(a, b product.Value) product.Value { return product.Join(reg, a, b) },
	}
}

func normalizeParamSinkExposures(reg *axis.Registry, in []ParamSinkExposure) []ParamSinkExposure {
	return paramSinkExposureMap(reg).Normalize(in)
}

func paramSinkExposuresEqual(reg *axis.Registry, a, b []ParamSinkExposure) bool {
	return paramSinkExposureMap(reg).Equal(a, b)
}

func paramSinkExposuresLessOrEq(reg *axis.Registry, a, b []ParamSinkExposure) bool {
	return paramSinkExposureMap(reg).LessOrEq(a, b)
}

func joinParamSinkExposures(reg *axis.Registry, a, b []ParamSinkExposure) []ParamSinkExposure {
	return paramSinkExposureMap(reg).Join(a, b)
}
