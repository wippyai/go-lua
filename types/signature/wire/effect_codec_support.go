package wire

import (
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/iteration"
)

func encodeParamRef(ref effect.ParamRef) *paramRefWire {
	return &paramRefWire{Index: encodeInt(ref.Index)}
}

func encodeInt(v int) *int {
	return &v
}

func decodeRequiredInt(v *int, missing string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("signature/wire: %s", missing)
	}
	return *v, nil
}

func encodeInt64(v int64) *int64 {
	return &v
}

func decodeRequiredInt64(v *int64, missing string) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("signature/wire: %s", missing)
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
		return effect.ParamRef{}, fmt.Errorf("signature/wire: %s", missing)
	}
	index, err := decodeRequiredInt(w.Index, "param ref index missing")
	if err != nil {
		return effect.ParamRef{}, err
	}
	return effect.ParamRef{Index: index}, nil
}

// The iteration-protocol vocabulary is the iteration package's. What the
// wire boundary owns is the spelling of its members, and that spelling
// lives in the one table below, indexed by the kind's own ordinal, so the write
// side and the read side consult a single statement.
//
// The tokens are the codec's serialization commitment and are written here
// rather than read off IteratorKind.String: the two happen to agree today and a
// law pins that agreement, but the display spelling stays free to change
// without moving the wire.
var iteratorKindWireTokens = [iteration.IteratorKindCount]string{
	iteration.IterateIndexed: "indexed",
	iteration.IterateKeyed:   "keyed",
}

func encodeIteratorKind(kind iteration.IteratorKind) (string, error) {
	if !kind.Valid() {
		return "", fmt.Errorf("signature/wire: unknown iterator kind %d", kind)
	}
	token := iteratorKindWireTokens[kind]
	if token == "" {
		return "", fmt.Errorf("signature/wire: unknown iterator kind %d", kind)
	}
	return token, nil
}

func decodeIteratorKind(kind string) (iteration.IteratorKind, error) {
	for _, declared := range iteration.IteratorKinds() {
		if token := iteratorKindWireTokens[declared]; token != "" && token == kind {
			return declared, nil
		}
	}
	return 0, fmt.Errorf("signature/wire: unknown iterator kind %q", kind)
}

// effectLabelWireKey is the stated ordering basis for the effect labels of one
// manifest row. A label sorts by the wire kind it is written under, and within
// a kind by the exact bytes the codec writes for the row. Those bytes are the
// row's own serialization, so a manifest's recorded order is a function of the
// frozen wire format and of nothing outside it -- the field order included,
// since the field order of the wire struct is that format.
func effectLabelWireKey(w effectLabelWire) string {
	payload, err := json.Marshal(w)
	if err != nil {
		// effectLabelWire carries only strings, integers, and slices and
		// pointers to the same, all of which serialize; a failure here would
		// leave the ordering basis partial and the recorded order undefined.
		// TestEffectLabelWireSerializes holds the struct to that shape.
		panic(fmt.Sprintf("signature/wire: effect label wire does not serialize: %v", err))
	}
	return w.Kind + "\x00" + string(payload)
}
