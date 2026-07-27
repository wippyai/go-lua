package front

import (
	"fmt"
	"testing"
)

func TestDraftWireCodecsDistinguishAbsentFromMalformed(t *testing.T) {
	t.Run("branch predicate", func(t *testing.T) {
		if _, present, err := DecodeBranchPredicateWire([]byte("scalar/nil")); present || err != nil {
			t.Fatalf("unrelated wire = present %t, error %v", present, err)
		}
		if _, present, err := DecodeBranchPredicateWire([]byte("front/branch-predicate/v1/{")); !present || err == nil {
			t.Fatalf("malformed wire = present %t, error %v", present, err)
		}
	})
	t.Run("branch difference", func(t *testing.T) {
		if _, present, err := DecodeBranchDiffWire([]byte("scalar/nil")); present || err != nil {
			t.Fatalf("unrelated wire = present %t, error %v", present, err)
		}
		if _, present, err := DecodeBranchDiffWire([]byte("front/branch-diff/v1/{}")); !present || err == nil {
			t.Fatalf("malformed wire = present %t, error %v", present, err)
		}
	})
	t.Run("module provider", func(t *testing.T) {
		if _, present, err := DecodeModuleProviderWire([]byte("provider/global/\"print\"")); present || err != nil {
			t.Fatalf("unrelated wire = present %t, error %v", present, err)
		}
		if _, present, err := DecodeModuleProviderWire([]byte("provider/module/v1/not-base64")); !present || err == nil {
			t.Fatalf("malformed wire = present %t, error %v", present, err)
		}
	})
}

func TestDraftWireCodecsRoundTripCanonicalValues(t *testing.T) {
	predicate := BranchPredicateWire{
		Kind:      "path-equal",
		Path:      "result.channel",
		OtherPath: "ch",
		PathShape: BranchChainPathWire{
			Key: "result.channel", Display: "result.channel",
			ParentKey: "result", ParentDisplay: "result", FinalField: "channel",
		},
		OtherPathShape: BranchChainPathWire{Key: "ch", Display: "ch"},
	}
	predicateEncoded, err := EncodeBranchPredicateWire(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, present, decodeErr := DecodeBranchPredicateWire(predicateEncoded); decodeErr != nil || !present || decoded != predicate {
		t.Fatalf("predicate round trip = %#v, present %t, error %v", decoded, present, decodeErr)
	}

	difference := BranchDiffWire{CoHi: 1, HiPath: "i", LoPath: "xs", LoIsLen: true, Edge: true}
	differenceEncoded, err := EncodeBranchDiffWire(difference)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, present, decodeErr := DecodeBranchDiffWire(differenceEncoded); decodeErr != nil || !present || decoded != difference {
		t.Fatalf("difference round trip = %#v, present %t, error %v", decoded, present, decodeErr)
	}

	provider := ModuleProviderWire{Module: "net", Suffix: ".connect"}
	providerEncoded, err := EncodeModuleProviderWire(provider)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, present, decodeErr := DecodeModuleProviderWire(providerEncoded); decodeErr != nil || !present || decoded != provider {
		t.Fatalf("provider round trip = %#v, present %t, error %v", decoded, present, decodeErr)
	}
}

func TestDraftWireCodecsRejectInvalidConstruction(t *testing.T) {
	for name, encode := range map[string]func() error{
		"predicate": func() error {
			_, err := EncodeBranchPredicateWire(BranchPredicateWire{})
			return err
		},
		"difference": func() error {
			_, err := EncodeBranchDiffWire(BranchDiffWire{HiPath: "i"})
			return err
		},
		"provider": func() error {
			_, err := EncodeModuleProviderWire(ModuleProviderWire{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := encode(); err == nil {
				t.Fatal("invalid semantic wire construction succeeded")
			}
		})
	}
}

func TestPathRelationRejectsFormattedChannelSuffixWithoutStructuralIdentity(t *testing.T) {
	_, err := EncodeBranchPredicateWire(BranchPredicateWire{
		Kind: "path-equal", Path: fmt.Sprint("result", ".", "channel"), OtherPath: "ch",
	})
	if err == nil {
		t.Fatal("path relation accepted formatted channel suffix without typed path structure")
	}
}
