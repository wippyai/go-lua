package summary

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRegistryFactMapLanesAreCached(t *testing.T) {
	reg := standard.Registry()

	returnFirst := returnConditionMap(reg)
	returnSecond := returnConditionMap(reg)
	if reflect.ValueOf(returnFirst.Key).Pointer() != reflect.ValueOf(returnSecond.Key).Pointer() {
		t.Fatal("return condition fact-map lane was rebuilt for the same registry")
	}

	exposureFirst := paramSinkExposureMap(reg)
	exposureSecond := paramSinkExposureMap(reg)
	if reflect.ValueOf(exposureFirst.Key).Pointer() != reflect.ValueOf(exposureSecond.Key).Pointer() {
		t.Fatal("param sink exposure fact-map lane was rebuilt for the same registry")
	}

	slotFirst := returnConditionSlotMap(reg)
	slotSecond := returnConditionSlotMap(reg)
	if reflect.ValueOf(slotFirst.Key).Pointer() != reflect.ValueOf(slotSecond.Key).Pointer() {
		t.Fatal("return condition slot fact-map lane was rebuilt for the same registry")
	}
}
