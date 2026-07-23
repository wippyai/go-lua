package body

import (
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func copyConfig(config Config) Config {
	config.Globals = append([]string(nil), config.Globals...)
	if len(config.GlobalTypes) != 0 {
		globalTypes := make(map[string]typ.Type, len(config.GlobalTypes))
		for name, t := range config.GlobalTypes {
			globalTypes[name] = t
		}
		config.GlobalTypes = globalTypes
	}
	copyPerSolveConfigAxes(&config, config)
	config.Signatures.Manifests = append([]*manifest.Manifest(nil), config.Signatures.Manifests...)
	config.ModuleExports.Manifests = append([]*manifest.Manifest(nil), config.ModuleExports.Manifests...)
	config.ModuleTypes.Manifests = append([]*manifest.Manifest(nil), config.ModuleTypes.Manifests...)
	if len(config.ExpressionValues) != 0 {
		values := make(map[factflow.ExprRef]product.Value, len(config.ExpressionValues))
		for ref, value := range config.ExpressionValues {
			values[ref] = value
		}
		config.ExpressionValues = values
	}
	if len(config.MethodReceiverTypes) != 0 {
		receivers := make(map[symbol.ID]typ.Type, len(config.MethodReceiverTypes))
		for id, t := range config.MethodReceiverTypes {
			receivers[id] = t
		}
		config.MethodReceiverTypes = receivers
	}
	return config
}

// SolveConfig returns the per-solve view of config. This is the single owner for
// moving full-check configuration axes into a prepared-body solve.
func (config Config) SolveConfig() SolveConfig {
	return solveConfigFromConfig(config)
}

func expressionValuesFromFacts(facts factflow.Facts, override map[factflow.ExprRef]product.Value) map[factflow.ExprRef]product.Value {
	var out map[factflow.ExprRef]product.Value
	facts.ForEachExpressionValue(func(ref factflow.ExprRef, value product.Value) bool {
		if out == nil {
			out = map[factflow.ExprRef]product.Value{}
		}
		out[ref] = value
		return true
	})
	if len(override) != 0 && out == nil {
		out = make(map[factflow.ExprRef]product.Value, len(override))
	}
	for ref, value := range override {
		out[ref] = value
	}
	return out
}

func expressionPathRefsFromFacts(facts factflow.Facts) map[factflow.ExprRef]struct{} {
	var out map[factflow.ExprRef]struct{}
	add := func(ref factflow.ExprRef) {
		if out == nil {
			out = map[factflow.ExprRef]struct{}{}
		}
		out[ref] = struct{}{}
	}
	facts.ForEachExpressionPath(func(ref factflow.ExprRef, _ pathdom.Path) bool {
		add(ref)
		return true
	})
	facts.ForEachDynamicIndexExpression(func(ref factflow.ExprRef, _ factflow.DynamicIndexExpression) bool {
		add(ref)
		return true
	})
	return out
}

func expressionOperationsFromFacts(facts factflow.Facts) map[factflow.ExprRef]factflow.ExpressionOperation {
	var out map[factflow.ExprRef]factflow.ExpressionOperation
	facts.ForEachExpressionOperation(func(ref factflow.ExprRef, op factflow.ExpressionOperation) bool {
		if out == nil {
			out = map[factflow.ExprRef]factflow.ExpressionOperation{}
		}
		out[ref] = op
		return true
	})
	return out
}

func expressionConditionsFromFacts(facts factflow.Facts) map[factflow.ExprRef]factflow.ExpressionCondition {
	var out map[factflow.ExprRef]factflow.ExpressionCondition
	facts.ForEachExpressionCondition(func(ref factflow.ExprRef, condition factflow.ExpressionCondition) bool {
		if out == nil {
			out = map[factflow.ExprRef]factflow.ExpressionCondition{}
		}
		out[ref] = condition
		return true
	})
	return out
}

func dynamicIndexExpressionsFromFacts(facts factflow.Facts) map[factflow.ExprRef]factflow.DynamicIndexExpression {
	var out map[factflow.ExprRef]factflow.DynamicIndexExpression
	facts.ForEachDynamicIndexExpression(func(ref factflow.ExprRef, expr factflow.DynamicIndexExpression) bool {
		if out == nil {
			out = map[factflow.ExprRef]factflow.DynamicIndexExpression{}
		}
		out[ref] = expr
		return true
	})
	return out
}

func configGlobals(config Config) []string {
	globals := append([]string(nil), config.Globals...)
	for name := range config.GlobalTypes {
		globals = append(globals, name)
	}
	// The Lua base globals are always present in the environment, independent of
	// whether typed stdlib signatures are loaded for this check.
	globals = append(globals, signaturelookup.StdlibBareGlobals()...)
	if config.Signatures.IncludeStdlib {
		for _, name := range signaturelookup.StdlibSignatureNames() {
			root := name
			if dot := strings.IndexByte(root, '.'); dot >= 0 {
				root = root[:dot]
			}
			if root != "" {
				globals = append(globals, root)
			}
		}
	}
	return globals
}

// Globals returns the lexical globals implied by a body configuration.
func Globals(config Config) []string {
	return configGlobals(config)
}

func hasSignatures(source signaturelookup.Source) bool {
	return source.IncludeStdlib || len(source.Manifests) != 0
}

func hasModuleExports(source importlookup.Source) bool {
	return len(source.Manifests) != 0
}
