// Package memberdefinition is the generator-only base vocabulary of the
// suspension-evidence Factor. The axis is a Factor of its own, so it declares
// its own key normalization, signature and carriers; it sits apart from the
// rule contribution that folds into it because an axis base and a rule
// declaration are two statements, and one package holding both is the central
// list this composition replaces.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	heapPackagePath       = "github.com/wippyai/go-lua/domain/heap"
	placementPackagePath  = "github.com/wippyai/go-lua/domain/placement"
	suspensionPackagePath = "github.com/wippyai/go-lua/domain/placement/suspension"
)

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func builtin(name string) definition.GoType { return definition.GoType{Name: name} }

func placementMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: placementPackagePath,
		Name:        name,
		Receiver:    goType(placementPackagePath, receiver),
		ResultIndex: resultIndex,
	}
}

// EvidenceSource is the suspension-evidence axis's own member vocabulary. Its
// coordinate is the Placement key, because the Factor is mounted over the very
// schema Placement mounted - a second normalization here would be a second
// coordinate authority over one directory - and its fact is the evidence cell
// this axis alone writes.
func EvidenceSource() definition.Definition {
	return definition.Definition{
		Name:       "PlacementSuspensionEvidence",
		Axis:       "placement-suspension-evidence",
		ImportPath: suspensionPackagePath,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "PlacementKeyCarrier",
			Dense:      builtin("uint32"),
			Normalizer: placementMethod("KeyIndex", "Schema", 0),
		}},
		Signature: definition.Signature{
			Key:  "PlacementKeyCarrier",
			Fact: "EvidenceFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
			{Name: "EvidenceFactCarrier", Key: "carrier/placement/suspension-evidence/fact", Type: goType(suspensionPackagePath, "Evidence")},
		},
	}
}
