package check

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func copyConfig(config Config) Config {
	config.Globals = append([]string(nil), config.Globals...)
	config.Signatures.Manifests = append([]*manifest.Manifest(nil), config.Signatures.Manifests...)
	if len(config.ExpressionValues) != 0 {
		values := make(map[factflow.ExprRef]product.Value, len(config.ExpressionValues))
		for ref, value := range config.ExpressionValues {
			values[ref] = value
		}
		config.ExpressionValues = values
	}
	return config
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

func configGlobals(config Config) []string {
	globals := append([]string(nil), config.Globals...)
	if hasSignatures(config.Signatures) {
		for _, m := range config.Signatures.Manifests {
			if m != nil && m.Path != "" {
				globals = append(globals, m.Path)
			}
		}
		for name := range config.Signatures.Signatures() {
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

func hasSignatures(source signaturelookup.Source) bool {
	return source.IncludeStdlib || len(source.Manifests) != 0
}
