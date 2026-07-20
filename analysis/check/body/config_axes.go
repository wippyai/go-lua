package body

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// configAxisDescriptor is the registration boundary for per-solve body config
// axes. Config and SolveConfig keep their public fields, but copy/transform
// logic walks this table so a new per-solve axis has one operational owner.
type configAxisDescriptor struct {
	fieldName string
	clone     func(reflect.Value) reflect.Value
}

func configAxis(fieldName string) configAxisDescriptor {
	return configAxisDescriptor{fieldName: fieldName}
}

func clonedConfigAxis(fieldName string, clone func(reflect.Value) reflect.Value) configAxisDescriptor {
	return configAxisDescriptor{fieldName: fieldName, clone: clone}
}

func (d configAxisDescriptor) copyConfig(dst *Config, src Config) {
	d.copyField(reflect.ValueOf(dst).Elem(), reflect.ValueOf(src), "Config")
}

func (d configAxisDescriptor) copySolve(dst *SolveConfig, src Config) {
	d.copyField(reflect.ValueOf(dst).Elem(), reflect.ValueOf(src), "SolveConfig")
}

func (d configAxisDescriptor) copyField(dst, src reflect.Value, dstTypeName string) {
	srcField := src.FieldByName(d.fieldName)
	if !srcField.IsValid() {
		panic(fmt.Sprintf("body: per-solve config axis %q has no Config field", d.fieldName))
	}
	dstField := dst.FieldByName(d.fieldName)
	if !dstField.IsValid() {
		panic(fmt.Sprintf("body: per-solve config axis %q has no %s field", d.fieldName, dstTypeName))
	}
	value := srcField
	if d.clone != nil {
		value = d.clone(srcField)
	}
	if !value.Type().AssignableTo(dstField.Type()) {
		panic(fmt.Sprintf("body: per-solve config axis %q type %s is not assignable to %s.%s %s",
			d.fieldName, value.Type(), dstTypeName, d.fieldName, dstField.Type()))
	}
	dstField.Set(value)
}

var perSolveConfigAxes = func() []configAxisDescriptor {
	axes := []configAxisDescriptor{
		configAxis("EntryState"),
		configAxis("Initial"),
		configAxis("TypeValues"),
		clonedConfigAxis("ClosedDynamicAllValues", cloneClosedDynamicAllValuesAxis),
		configAxis("Context"),
		clonedConfigAxis("StateLanes", cloneStateLanesAxis),
		configAxis("CallOutcome"),
		configAxis("CallOutcomeFactory"),
		configAxis("SignatureArgumentType"),
		configAxis("SignatureArgumentTypeFactory"),
		configAxis("SummaryInputDigests"),
		configAxis("SummaryInputs"),
		configAxis("SummaryInputsComplete"),
		configAxis("WidenAt"),
		configAxis("WidenDelay"),
		configAxis("Stats"),
	}
	validatePerSolveConfigAxes(axes)
	return axes
}()

func copyPerSolveConfigAxes(dst *Config, src Config) {
	for _, axis := range perSolveConfigAxes {
		axis.copyConfig(dst, src)
	}
}

func solveConfigFromConfig(config Config) SolveConfig {
	var solve SolveConfig
	for _, axis := range perSolveConfigAxes {
		axis.copySolve(&solve, config)
	}
	return solve
}

func cloneClosedDynamicAllValuesAxis(value reflect.Value) reflect.Value {
	in := value.Interface().([]factapply.ClosedDynamicAllValueInvariant)
	return reflect.ValueOf(append([]factapply.ClosedDynamicAllValueInvariant(nil), in...))
}

func cloneStateLanesAxis(value reflect.Value) reflect.Value {
	in := value.Interface().([]state.LaneID)
	return reflect.ValueOf(state.CloneLanes(in))
}

func validatePerSolveConfigAxes(axes []configAxisDescriptor) {
	configType := reflect.TypeOf(Config{})
	solveType := reflect.TypeOf(SolveConfig{})
	seen := make(map[string]struct{}, len(axes))
	for _, axis := range axes {
		if axis.fieldName == "" {
			panic("body: per-solve config axis with empty field name")
		}
		if _, ok := seen[axis.fieldName]; ok {
			panic(fmt.Sprintf("body: duplicate per-solve config axis %q", axis.fieldName))
		}
		seen[axis.fieldName] = struct{}{}

		configField, ok := configType.FieldByName(axis.fieldName)
		if !ok {
			panic(fmt.Sprintf("body: per-solve config axis %q has no Config field", axis.fieldName))
		}
		solveField, ok := solveType.FieldByName(axis.fieldName)
		if !ok {
			panic(fmt.Sprintf("body: per-solve config axis %q has no SolveConfig field", axis.fieldName))
		}
		if !configField.Type.AssignableTo(solveField.Type) {
			panic(fmt.Sprintf("body: per-solve config axis %q Config type %s is not assignable to SolveConfig type %s",
				axis.fieldName, configField.Type, solveField.Type))
		}
	}
}
