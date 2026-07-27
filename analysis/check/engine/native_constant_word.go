package engine

import (
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
)

func numericLiteralIsInteger(text string) bool {
	_, err := strconv.ParseInt(text, 10, 64)
	return err == nil && !strings.ContainsAny(text, ".eE")
}

// nativeConstantWord is the exact machine arm the closed value partition
// published for a scalar. Publication consumers may render it; they may not
// recover it by walking the lowering IR.
type nativeConstantWord struct {
	representation string
	text           string
}

func nativePublishedConstantWord(value []byte) (nativeConstantWord, bool) {
	scalar, ok := shapefact.DecodeScalar(value)
	if !ok {
		return nativeConstantWord{}, false
	}
	switch scalar.Kind {
	case shapefact.ScalarNumber:
		number := string(scalar.Data)
		if numericLiteralIsInteger(number) {
			_, err := strconv.ParseInt(number, 10, 64)
			return nativeConstantWord{representation: "integer", text: number}, err == nil
		}
		parsed, err := strconv.ParseFloat(number, 64)
		return nativeConstantWord{representation: "float", text: number}, err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	case shapefact.ScalarString:
		return nativeConstantWord{representation: "string", text: string(scalar.Data)}, true
	case shapefact.ScalarBool, shapefact.ScalarOptionalNilComparison:
		text, _ := scalar.BooleanText()
		return nativeConstantWord{representation: "boolean", text: text}, true
	default:
		return nativeConstantWord{}, false
	}
}
