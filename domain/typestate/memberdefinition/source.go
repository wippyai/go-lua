// Package memberdefinition is the generator-only base vocabulary of the
// typestate axis: the coordinate it is keyed by, the fact it holds, and the
// normalization that carries one cell into the dense coordinate the engine
// addresses it at.
//
// It sits apart from the rule contribution that folds into it because an axis
// base and a rule declaration are two statements, and one package holding both
// is the central list this composition replaces.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	typestatePackagePath = "github.com/wippyai/go-lua/domain/typestate"
	statecellPackagePath = "github.com/wippyai/go-lua/domain/typestate/statecell"
)

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func builtin(name string) definition.GoType { return definition.GoType{Name: name} }

// StateSource is the typestate axis's own member vocabulary.
//
// One coordinate is one resource under one protocol, as statecell seals it,
// and the fact is the abstract state that cell holds. Both are borrowed from
// the two owners that already have them: the space numbers the cells and the
// state machine values them, so this declaration mints neither.
//
// The vocabulary is generated into statecell rather than beside the state
// machine, because the coordinate space is the axis's key authority and the
// judgment kernel declares no row of any surface.
func StateSource() definition.Definition {
	return definition.Definition{
		Name:       "Typestate",
		Axis:       "typestate",
		ImportPath: statecellPackagePath,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier: "CellCarrier",
			Dense:   builtin("uint32"),
			Normalizer: definition.GoSymbol{
				PackagePath: statecellPackagePath,
				Name:        "DenseIndex",
				Receiver:    goType(statecellPackagePath, "Space"),
				ResultIndex: 0,
			},
		}},
		Signature: definition.Signature{
			Key:  "CellCarrier",
			Fact: "StateCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "CellCarrier", Key: "carrier/typestate/cell", Type: goType(statecellPackagePath, "Cell")},
			{Name: "StateCarrier", Key: "carrier/typestate/state", Type: goType(typestatePackagePath, "Abstract")},
		},
	}
}
