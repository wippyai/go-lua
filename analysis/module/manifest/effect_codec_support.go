package manifest

import (
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
)

func encodeParamRef(ref effect.ParamRef) *paramRefWire {
	return &paramRefWire{Index: encodeInt(ref.Index)}
}

func encodeInt(v int) *int {
	return &v
}

func decodeRequiredInt(v *int, missing string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("manifest: %s", missing)
	}
	return *v, nil
}

func encodeInt64(v int64) *int64 {
	return &v
}

func decodeRequiredInt64(v *int64, missing string) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("manifest: %s", missing)
	}
	return *v, nil
}

func compareOptionalInt(a, b *int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	case *a < *b:
		return -1
	case *a > *b:
		return 1
	default:
		return 0
	}
}

func decodeRequiredParamRef(w *paramRefWire, missing string) (effect.ParamRef, error) {
	if w == nil {
		return effect.ParamRef{}, fmt.Errorf("manifest: %s", missing)
	}
	index, err := decodeRequiredInt(w.Index, "param ref index missing")
	if err != nil {
		return effect.ParamRef{}, err
	}
	return effect.ParamRef{Index: index}, nil
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
