package factkey

import "testing"

func TestTruthAndFreezePayloadsRoundTripClosedDomains(t *testing.T) {
	for _, truth := range []Truth{TruthProven, TruthRefuted} {
		if decoded := DecodeTruth(EncodeTruth(truth)); decoded != truth {
			t.Fatalf("truth round trip = %v, want %v", decoded, truth)
		}
	}
	if DecodeTruth([]byte("yes")) != TruthUnknown {
		t.Fatal("unknown truth spelling entered the marker domain")
	}

	for _, freeze := range []FreezePayload{
		{Kind: FreezeUnconditional},
		{Kind: FreezeGuarded, Guard: "op-00000007", Edge: true},
		{Kind: FreezeGuarded, Guard: "op-00000007", Edge: false},
	} {
		decoded, ok := DecodeFreezePayload(EncodeFreezePayload(freeze))
		if !ok || decoded != freeze {
			t.Fatalf("freeze round trip = %+v/%v, want %+v", decoded, ok, freeze)
		}
	}
	if _, ok := DecodeFreezePayload([]byte("guard/op/maybe")); ok {
		t.Fatal("malformed freeze edge entered the payload domain")
	}
}

func TestNativeRelationPayloadsRetainEstablishedWire(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"call scc": {
			got: EncodeNativeCallSCCPayload(NativeCallSCCPayload{
				Arguments: "[number]", Edges: []string{"f->g"}, Members: []string{"f", "g"}, ResultSlots: 2,
			}),
			want: "arguments=[number] completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} edges_closed=[f->g] members=[f,g] results={'exact': True, 'count': 2}",
		},
		"stable shape": {
			got: EncodeNativeShapeIdentityPayload(NativeShapeIdentityPayload{
				Kind: NativeShapeIdentityStable, ShapeID: 0x123,
			}),
			want: "distinct_identities=1 field_offsets=identical field_order=canonical interned=true shape_id=0000000000000123 stable_across_modules=true stable_across_sites=true",
		},
		"field read shape": {
			got: EncodeNativeShapeIdentityPayload(NativeShapeIdentityPayload{
				Kind: NativeShapeIdentityFieldRead, ShapeID: 0x123,
			}),
			want: "epoch=field_read field_offsets=identical interned=true shape_id=0000000000000123 stable=true",
		},
		"transition shape": {
			got: EncodeNativeShapeIdentityPayload(NativeShapeIdentityPayload{
				Kind: NativeShapeIdentityTransition, ShapeID: 0x123,
			}),
			want: "field_offsets=identical field_order=canonical interned=true shape_id=0000000000000123",
		},
		"record": {
			got: EncodeNativeRecordConstructionPayload(NativeRecordConstructionPayload{
				Entries: 3, BooleanStorage: true, NumericUnion: true,
				DuplicateChildren: 1, Edges: 2, EvaluationOrder: true,
				Ownership: NativeRecordOwnershipMove,
			}),
			want: "entries=3 entry_storage=committed boolean_storage=canonical_tag field_carrier=numeric_union overflow=promote_integer_to_number duplicate_children=1 edges=2 evaluation_order=preserved fresh=true ownership=move",
		},
		"entry ownership": {
			got: EncodeNativeRecordEntryOwnershipPayload(NativeRecordEntryOwnershipPayload{
				Field: "child", Ownership: NativeRecordOwnershipMove,
			}),
			want: "field=child ownership=move producer_bound=true write_barrier=required",
		},
		"function entry": {
			got: EncodeNativeFunctionEntryPayload(NativeFunctionEntryPayload{
				Parameters: 2, Varargs: true, CanThrow: true, ResultOpen: true,
			}),
			want: "params={'exact': False, 'prefix': 2, 'open_tail': True} completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} results={'exact': False, 'prefix': 0, 'open_tail': True}",
		},
		"callee set": {
			got: EncodeNativeCalleeSetPayload(NativeCalleeSetPayload{
				Cardinality: 2, Completeness: NativeCalleeIncomplete,
			}),
			want: "cardinality=2 completeness=incomplete",
		},
		"discriminant": {
			got: EncodeNativeDiscriminantSelectPayload(NativeDiscriminantSelectPayload{
				Cases: 3, DefaultRequired: true, DiscriminantField: "kind",
			}),
			want: "cases=3 default_required=true dense_mapping=[0,1,2] discriminant_field=kind exhaustive=false",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("wire = %q, want %q", test.got, test.want)
			}
		})
	}
}
