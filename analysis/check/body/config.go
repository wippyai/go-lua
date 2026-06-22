package body

import (
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func copyConfig(config Config) Config {
	config.Globals = append([]string(nil), config.Globals...)
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
	return config
}

func solveConfigFromConfig(config Config) SolveConfig {
	return SolveConfig{
		EntryState:                   config.EntryState,
		Initial:                      config.Initial,
		TypeValues:                   config.TypeValues,
		CallOutcome:                  config.CallOutcome,
		CallOutcomeFactory:           config.CallOutcomeFactory,
		SignatureArgumentType:        config.SignatureArgumentType,
		SignatureArgumentTypeFactory: config.SignatureArgumentTypeFactory,
		WidenAt:                      config.WidenAt,
		WidenDelay:                   config.WidenDelay,
		Stats:                        config.Stats,
	}
}

func mergeExpressionValues(base, override map[factflow.ExprRef]product.Value) map[factflow.ExprRef]product.Value {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]product.Value, len(base)+len(override))
	for ref, value := range base {
		out[ref] = value
	}
	for ref, value := range override {
		out[ref] = value
	}
	return out
}

func exprRefSet(paths map[factflow.ExprRef]pathdom.Path) map[factflow.ExprRef]struct{} {
	if len(paths) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]struct{}, len(paths))
	for ref := range paths {
		out[ref] = struct{}{}
	}
	return out
}

func addDynamicIndexExprRefs(set map[factflow.ExprRef]struct{}, dynamic map[factflow.ExprRef]factflow.DynamicIndexExpression) map[factflow.ExprRef]struct{} {
	if len(dynamic) == 0 {
		return set
	}
	if set == nil {
		set = make(map[factflow.ExprRef]struct{}, len(dynamic))
	}
	for ref := range dynamic {
		set[ref] = struct{}{}
	}
	return set
}

func configGlobals(config Config) []string {
	globals := append([]string(nil), config.Globals...)
	if config.Signatures.IncludeStdlib {
		for name := range signaturelookup.StdlibSignatures() {
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
