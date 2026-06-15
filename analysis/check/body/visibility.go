package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/visibilityfacts"
)

func defaultVisibilityResolver(bindings *bind.Result, built *cfgbuild.Result, facts factflow.Facts) *visibility.Resolver {
	var graph cfg.Graph
	if built != nil {
		graph = built.Graph
	}
	return visibilityfacts.Resolver(bindings, graph, facts)
}
