package manifest

import (
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
)

func encodeParamRef(ref effect.ParamRef) *paramRefWire {
	return &paramRefWire{Index: ref.Index}
}

func decodeRequiredParamRef(w *paramRefWire, missing string) (effect.ParamRef, error) {
	if w == nil {
		return effect.ParamRef{}, fmt.Errorf("manifest: %s", missing)
	}
	return effect.ParamRef{Index: w.Index}, nil
}

func encodeIteratorKind(kind iteration.IteratorKind) (string, error) {
	switch kind {
	case iteration.IterateIndexed:
		return "indexed", nil
	case iteration.IterateKeyed:
		return "keyed", nil
	default:
		return "", fmt.Errorf("manifest: unknown iterator kind %d", kind)
	}
}

func decodeIteratorKind(kind string) (iteration.IteratorKind, error) {
	switch kind {
	case "indexed":
		return iteration.IterateIndexed, nil
	case "keyed":
		return iteration.IterateKeyed, nil
	default:
		return 0, fmt.Errorf("manifest: unknown iterator kind %q", kind)
	}
}

func effectLabelWireKey(w effectLabelWire) string {
	data, err := json.Marshal(w)
	if err != nil {
		return w.Kind
	}
	return string(data)
}
