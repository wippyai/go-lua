package front

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeTopologyDraftSchemaCannotStateVerdicts(t *testing.T) {
	payloads := []any{
		NativeTopologyDraft{},
		NativeCallGraphDraft{},
		NativeBodyTypeDraft{},
		NativeConstantCandidateDraft{},
		NativePublicationSiteDraft{},
		NativeRecordTopologyDraft{},
		NativeShapeTopologyDraft{},
		NativeShapeEpochTopologyDraft{},
		NativeShapeTransitionDraft{},
		NativeDiscriminantDraft{},
		NativeRecursiveTopologyDraft{},
		NativeSummaryTopologyDraft{},
		NativeFunctionEntryDraft{},
		NativeCalleeTopologyDraft{},
		NativeEffectTopologyDraft{},
		NativeKernelOccurrenceDraft{},
	}
	for _, payload := range payloads {
		assertVerdictIncapableDraftType(t, reflect.TypeOf(payload), make(map[reflect.Type]bool))
	}
}

func assertVerdictIncapableDraftType(t *testing.T, value reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct || value.PkgPath() != reflect.TypeOf(NativeTopologyDraft{}).PkgPath() || seen[value] {
		return
	}
	seen[value] = true
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		name := strings.ToLower(field.Name)
		for _, forbidden := range []string{
			"verdict", "conclusion", "stable", "fresh", "exhaust",
			"ownership", "completion", "complete",
		} {
			if strings.Contains(name, forbidden) {
				t.Errorf("%s.%s can name a semantic conclusion", value.Name(), field.Name)
			}
		}
		if field.Type.Kind() == reflect.Bool {
			t.Errorf("%s.%s is bool; topology uses coordinates and counts, never assertions", value.Name(), field.Name)
		}
		assertVerdictIncapableDraftType(t, field.Type, seen)
	}
}

func TestNativeTopologyCodecRejectsVerdictField(t *testing.T) {
	encoded := []byte(`{"version":1,"drafts":[{"kind":3,"publication":{"site":{"body":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"position":1},"span":{},"stable":true}}]}`)
	if _, err := DecodeNativeTopologyDrafts(encoded); err == nil {
		t.Fatal("topology codec accepted a semantic verdict field")
	}
}
