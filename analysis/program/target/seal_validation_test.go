package target

import "testing"

func TestSealValidationAdmitsOnlyNormalizedBindingRows(t *testing.T) {
	if !validBinding(BindingSpec{Namespace: BindingBuiltin, Member: []string{"ok"}}) {
		t.Fatal("valid direct binding rejected")
	}
	if validBinding(BindingSpec{Namespace: BindingBuiltin}) {
		t.Fatal("binding without a member rejected law")
	}
	if validBinding(BindingSpec{Namespace: BindingNamespace(0), Member: []string{"bad"}}) {
		t.Fatal("invalid binding namespace admitted")
	}
	if !validValuesTail(ValuesClosed, 0, 0, false) || validValuesTail(ValuesUnknown, 0, 0, false) {
		t.Fatal("Values tail validation admitted an invalid closed/unknown combination")
	}
}
