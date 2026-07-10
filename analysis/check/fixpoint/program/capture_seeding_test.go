package program

import (
	"fmt"
	"testing"
)

func TestCapturePolicyLawMatrix(t *testing.T) {
	type captureAsset struct {
		name          string
		requireModule bool
		functionValue bool
	}
	assets := []captureAsset{
		{name: "plain table"},
		{name: "nested graph"},
		{name: "array elements"},
		{name: "module require", requireModule: true},
		{name: "recursive function", functionValue: true},
	}
	writes := []string{"unwritten", "written by sibling", "written after capture"}
	reaches := []string{"local", "escaped", "opaque callback reachable"}

	for _, write := range writes {
		for _, reach := range reaches {
			for _, asset := range assets {
				t.Run(fmt.Sprintf("%s/%s/%s", write, reach, asset.name), func(t *testing.T) {
					written := write != "unwritten"
					opaque := reach == "opaque callback reachable"
					policy := CapturePolicy{mode: captureFactModeFor(capturePolicyFacts{
						structuralWrite:         written,
						opaqueCallbackReachable: opaque,
						requireModule:           asset.requireModule,
						functionValue:           asset.functionValue,
					})}

					invalidated := !written && opaque && !asset.requireModule && !asset.functionValue
					want := capturePolicyLawFactClasses{fullGraph: !written && !invalidated, pathProofs: !written && !invalidated, exactIdentity: true}
					if written {
						want.invariantHeap = true
					}
					if invalidated {
						want.invariantHeap = true
						want.invariantMember = true
					}
					got := capturePolicyFactClasses(policy)
					if got != want {
						t.Fatalf("fact classes = %#v, want %#v", got, want)
					}
				})
			}
		}
	}
}

type capturePolicyLawFactClasses struct {
	fullGraph       bool
	pathProofs      bool
	invariantHeap   bool
	invariantMember bool
	exactIdentity   bool
}

func capturePolicyFactClasses(policy CapturePolicy) capturePolicyLawFactClasses {
	switch policy.mode {
	case captureFullFactGraph:
		return capturePolicyLawFactClasses{fullGraph: true, pathProofs: true, exactIdentity: true}
	case captureWriteInvariantFacts:
		return capturePolicyLawFactClasses{invariantHeap: true, exactIdentity: true}
	case captureEscapedInvariantFacts:
		return capturePolicyLawFactClasses{invariantHeap: true, invariantMember: true, exactIdentity: true}
	default:
		return capturePolicyLawFactClasses{}
	}
}
