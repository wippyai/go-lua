package emit

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// renderInstaller writes the sealed half of the emitted family: the shape
// fence every plan row is held to, and the primitive each declared read and
// write is sealed through.
//
// The fence restates the declaration against the sealed descriptor the engine
// hands over. It is not a second classification: a row whose descriptor
// disagrees with the declaration this file was emitted from is a plan that no
// longer matches its source, and the only sound answer to that is refusal.
func renderInstaller(out *strings.Builder, built *plan) error {
	imports := built.imports
	execution := imports.use(executionPackagePath)
	ruleprogram := imports.use(programPackagePath)
	dense := imports.typeName(built.write.dense)
	fact := imports.typeName(built.write.fact)

	out.WriteString("// " + installerType + " authors this rule's execution family. The axis schemas it\n")
	out.WriteString("// holds are the ones the declaration names: the candidate directory it resolves\n")
	out.WriteString("// dense candidates through, and every static axis a declared relation derives\n")
	out.WriteString("// against. It reaches no owner callback and no runtime capability.\n")
	fmt.Fprintf(out, "type %s struct {\n", installerType)
	for _, axis := range built.axes {
		fmt.Fprintf(out, "\t%s %s\n", axis.param, imports.typeName(axis.schemaType))
	}
	out.WriteString("\trule uint32\n}\n\n")

	parameters := make([]string, 0, len(built.axes)+1)
	assignments := make([]string, 0, len(built.axes)+1)
	for _, axis := range built.axes {
		parameters = append(parameters, axis.param+" "+imports.typeName(axis.schemaType))
		assignments = append(assignments, axis.param+": "+axis.param)
	}
	parameters = append(parameters, "rule uint32")
	assignments = append(assignments, "rule: rule")

	out.WriteString("// " + constructor + " seals this rule's family installer against the axis schemas\n")
	out.WriteString("// its declaration names and the sealed ordinal it authors. The bind arm that\n")
	out.WriteString("// resolves those schemas from its composition's authorities is the owner's own,\n")
	out.WriteString("// because how an authority record is reached is that composition's knowledge\n")
	out.WriteString("// and not this rule's.\n")
	fmt.Fprintf(out, "func %s(%s) (%s.RuleFamilyInstaller[%s, %s], bool) {\n",
		constructor, strings.Join(parameters, ", "), execution, dense, fact)
	fmt.Fprintf(out, "\tinstall := %s{%s}\n", installerType, strings.Join(assignments, ", "))
	out.WriteString("\tif !install.available() {\n\t\treturn nil, false\n\t}\n")
	out.WriteString("\treturn install, true\n}\n\n")

	out.WriteString("// available proves every schema this installer seals against was supplied.\n")
	out.WriteString("// Whether a supplied schema is itself admissible is that schema's own answer,\n")
	out.WriteString("// given below when a candidate or a projection is resolved through it.\n")
	fmt.Fprintf(out, "func (install %s) available() bool {\n", installerType)
	guards := make([]string, 0, len(built.axes))
	for _, axis := range built.axes {
		if axis.schemaType.Pointer {
			guards = append(guards, "install."+axis.param+" != nil")
		}
	}
	if len(guards) == 0 {
		out.WriteString("\treturn true\n}\n\n")
	} else {
		fmt.Fprintf(out, "\treturn %s\n}\n\n", strings.Join(guards, " && "))
	}

	fmt.Fprintf(out, "func (install %s) InstallRuleFamily(plane %s.FormPlane[%s, %s], ruleOrdinal uint32, rows []%s.FormRow) (%s.Family, []%s.FormAddress, bool) {\n",
		installerType, execution, dense, fact, execution, execution, execution)
	out.WriteString("\tif !install.available() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {\n\t\treturn nil, nil, false\n\t}\n")
	fmt.Fprintf(out, "\tsealed := &%s{rows: make([]%s, 0, len(rows))", familyType, rowType)
	for _, axis := range built.familyAxes {
		fmt.Fprintf(out, ", %s: install.%s", axis.param, axis.param)
	}
	out.WriteString("}\n")
	if built.shape == shapeSelectedRoute {
		out.WriteString("\twidth := plane.RouteWidth()\n")
		out.WriteString("\tif width < 0 {\n\t\treturn nil, nil, false\n\t}\n")
		out.WriteString("\tsealed.plane, sealed.width = plane, width\n")
	}
	fmt.Fprintf(out, "\taddresses := make([]%s.FormAddress, 0, len(rows))\n", execution)
	out.WriteString("\tfor _, planRow := range rows {\n")
	fmt.Fprintf(out, "\t\tif planRow.Form != %s.%s || !planRow.Rule.Available() || planRow.Rule.ReadCount() != %d || planRow.Rule.OutputCount() != %d {\n\t\t\treturn nil, nil, false\n\t\t}\n",
		execution, built.form, len(built.joins), len(built.target.Spec.Program.Fold.Outputs))
	out.WriteString("\t\toutput, outputOK := planRow.Rule.OutputAt(0)\n")
	out.WriteString("\t\tif !outputOK")
	switch built.shape {
	case shapeCarry:
		fmt.Fprintf(out, " || output.Mode != %s.ModeExact || output.RouteJoinPresent", ruleprogram)
	case shapeSelectedRoute:
		fmt.Fprintf(out, " || output.Mode != %s.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != %d",
			ruleprogram, built.route.join.position)
	}
	fmt.Fprintf(out, " || output.Slot != %d {\n\t\t\treturn nil, nil, false\n\t\t}\n", built.outputSlot)

	if built.shape == shapeCarry {
		fmt.Fprintf(out, "\t\tcarryMode, carryPresent := planRow.Rule.CarryMode()\n")
		fmt.Fprintf(out, "\t\tif !carryPresent || carryMode != %s.CarryTransform {\n\t\t\treturn nil, nil, false\n\t\t}\n", ruleprogram)
	}

	firstExact := -1
	for _, join := range built.joins {
		if join.read.Form == program.Exact {
			firstExact = join.position
			break
		}
	}
	for _, join := range built.joins {
		form, formOK := readFormExpression(join.read.Form)
		if !formOK {
			return unexpressible(built.target.Spec.Key, fmt.Sprintf("a %s read", readFormName(join.read.Form)),
				fmt.Sprintf("join %d names no read form the emitted fence can restate", join.position))
		}
		fmt.Fprintf(out, "\t\tplan%d, plan%dOK := planRow.Rule.ReadAt(%d)\n", join.position, join.position, join.position)
		fmt.Fprintf(out, "\t\tif !plan%dOK || plan%d.Form != %s.%s || plan%d.Input != %d || plan%d.PointBound != %s.%s",
			join.position, join.position, ruleprogram, form, join.position, uint64(join.read.Input),
			join.position, ruleprogram, pointBoundExpression(join.read.PointBound))
		if join == built.routeJoin() {
			fmt.Fprintf(out, " || plan%d.Factor != output.Factor", join.position)
		}
		out.WriteString(" {\n\t\t\treturn nil, nil, false\n\t\t}\n")
	}

	fmt.Fprintf(out, "\t\tcandidate, candidateOK := %s\n",
		imports.call(built.candidate.at, "install."+built.candidate.axis.param, "int(planRow.Candidate)"))
	out.WriteString("\t\tif !candidateOK {\n\t\t\treturn nil, nil, false\n\t\t}\n")

	sealedNames := make([]string, 0, len(built.joins)+1)
	for _, join := range built.joins {
		expression, err := readExpression(built, join, firstExact)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s", expression)
		sealedNames = append(sealedNames, fmt.Sprintf("%s: %sSealed", join.name, join.name))
	}
	switch built.shape {
	case shapeCarry:
		fmt.Fprintf(out, "\t\twriteSealed, writeSealedOK := plane.RowCarry(planRow, %s)\n",
			imports.methodValue(built.carry.transform.Implementation, "candidate"))
	case shapeSelectedRoute:
		out.WriteString("\t\twriteSealed, writeSealedOK := plane.RouteWrite(uint16(output.Slot))\n")
	}
	out.WriteString("\t\tif !writeSealedOK")
	for _, join := range built.joins {
		fmt.Fprintf(out, " || !%sSealedOK", join.name)
	}
	out.WriteString(" {\n\t\t\treturn nil, nil, false\n\t\t}\n")
	sealedNames = append(sealedNames, "write: writeSealed")

	fmt.Fprintf(out, "\t\taddresses = append(addresses, %s.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})\n", execution)
	fmt.Fprintf(out, "\t\tsealed.rows = append(sealed.rows, %s{candidate: candidate, %s})\n", rowType, strings.Join(sealedNames, ", "))
	out.WriteString("\t}\n")
	out.WriteString("\treturn sealed, addresses, true\n}\n")
	return nil
}

// routeJoin answers the selected join a routed output publishes through, or
// nil for a shape that publishes at its own coordinate.
func (built *plan) routeJoin() *joinPlan {
	if built.route == nil {
		return nil
	}
	return built.route.join
}

func pointBoundExpression(bound program.PointBoundDecl) string {
	if bound == program.PointBoundSelf {
		return "PointBoundSelf"
	}
	return "PointBound"
}

// readExpression emits the one primitive that seals one declared read. Which
// primitive it is follows from the read's own declaration: the axis it names
// against the axis the rule writes, and its declared form.
func readExpression(built *plan, join *joinPlan, firstExact int) (string, error) {
	imports := built.imports
	execution := imports.use(executionPackagePath)
	dense := imports.typeName(join.axis.dense)
	fact := imports.typeName(join.axis.fact)
	var out strings.Builder
	if join.foreign {
		fmt.Fprintf(&out, "\t\tforeign%d, foreign%dOK := plane.Foreign(plan%d.Factor)\n", join.position, join.position, join.position)
		fmt.Fprintf(&out, "\t\tif !foreign%dOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.position)
	}
	switch {
	case join.read.Form == program.Exact && join.foreign:
		fmt.Fprintf(&out, "\t\t%sSealed, %sSealedOK := %s.ForeignRowExactRead[%s, %s](foreign%d, planRow, %d)\n",
			join.name, join.name, execution, dense, fact, join.position, join.position)
	case join.read.Form == program.Exact:
		if join.position != firstExact {
			return "", unexpressible(built.target.Spec.Key, fmt.Sprintf("a second axis-local exact read at join %d", join.position),
				"only the first exact join's Unit is the plan row's own; a later one needs a row-bound axis-local read primitive, which the execution vocabulary does not publish")
		}
		fmt.Fprintf(&out, "\t\t%sSealed, %sSealedOK := plane.ExactRead(planRow.Unit, uint16(plan%d.Input))\n",
			join.name, join.name, join.position)
	case join.read.Form == program.Selected:
		policy, err := readCellPolicy(built, join)
		if err != nil {
			return "", err
		}
		if join.foreign {
			fmt.Fprintf(&out, "\t\t%sSealed, %sSealedOK := %s.ForeignSelectedRead[%s, %s](foreign%d, uint16(plan%d.Input), plan%d.Contract, %s)\n",
				join.name, join.name, execution, dense, fact, join.position, join.position, join.position, policy)
			break
		}
		fmt.Fprintf(&out, "\t\t%sSealed, %sSealedOK := plane.SelectedRead(uint16(plan%d.Input), plan%d.Contract, %s)\n",
			join.name, join.name, join.position, join.position, policy)
	default:
		return "", unexpressible(built.target.Spec.Key, fmt.Sprintf("a %s read", readFormName(join.read.Form)),
			fmt.Sprintf("join %d has no sealed read primitive in the emitted vocabulary", join.position))
	}
	return out.String(), nil
}

// readCellPolicy is the substitution one read's declared sparsity derives. An
// explicitly sparse read delivers every coordinate unchanged, which is the
// zero policy. A read that declares the Factor's default or a dense
// denominator substitutes a fact this emitter cannot name: the axis publishes
// no declared default or top symbol, so the substitution is refused rather
// than guessed.
func readCellPolicy(built *plan, join *joinPlan) (string, error) {
	imports := built.imports
	execution := imports.use(executionPackagePath)
	if join.read.Contract.Sparse != program.SparseExplicit {
		return "", unexpressible(built.target.Spec.Key, fmt.Sprintf("a selected read with %s sparsity", sparseName(join.read.Contract.Sparse)),
			fmt.Sprintf("join %d substitutes the Factor's default at an unwritten coordinate, and axis %q declares no default or top accessor for the emitted policy to seal",
				join.position, string(join.axis.key)))
	}
	return fmt.Sprintf("%s.ReadCellPolicy[%s]{}", execution, imports.typeName(join.axis.fact)), nil
}
