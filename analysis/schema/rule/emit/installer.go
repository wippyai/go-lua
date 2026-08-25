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
	out.WriteString("//\n")
	out.WriteString("// It holds NO rule ordinal. Which rule an installer authors is the claim it\n")
	out.WriteString("// was installed under, and the family table resolves an installer only for\n")
	out.WriteString("// that claim; a copy kept here would be a second answer to a question the\n")
	out.WriteString("// table already answers, and one that goes stale on its own.\n")
	fmt.Fprintf(out, "type %s struct {\n", installerType)
	for _, axis := range built.axes {
		fmt.Fprintf(out, "\t%s %s\n", axis.param, imports.typeName(axis.schemaType))
	}
	out.WriteString("}\n\n")

	parameters := make([]string, 0, len(built.axes))
	assignments := make([]string, 0, len(built.axes))
	for _, axis := range built.axes {
		parameters = append(parameters, axis.param+" "+imports.typeName(axis.schemaType))
		assignments = append(assignments, axis.param+": "+axis.param)
	}

	out.WriteString("// " + constructor + " seals this rule's family installer against the axis schemas\n")
	out.WriteString("// its declaration names. The bind arm that resolves those schemas from its\n")
	out.WriteString("// composition's authorities is the owner's own, because how an authority\n")
	out.WriteString("// record is reached is that composition's knowledge and not this rule's.\n")
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

	if built.shape == shapeExactFold && !built.exact.candidateOwned {
		if err := renderExactDestinationProjector(out, built); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "func (install %s) InstallRuleFamily(plane %s.FormPlane[%s, %s], _ uint32, rows []%s.FormRow) (%s.Family, []%s.FormAddress, bool) {\n",
		installerType, execution, dense, fact, execution, execution, execution)
	out.WriteString("\tif !install.available() || !plane.Valid() || len(rows) == 0 {\n\t\treturn nil, nil, false\n\t}\n")
	if state := built.fold.state; state != nil {
		arguments := make([]string, 0, len(state.staticAxes))
		for _, axis := range state.staticAxes {
			arguments = append(arguments, "install."+axis.param)
		}
		// The judgment's state is sealed once, here, from schemas that are
		// immutable for the life of this binding. Sealing it per row would be
		// the same answer stored many times; sealing it per invocation would
		// allocate on a warm path.
		fmt.Fprintf(out, "\tstate, stateOK := %s\n", imports.call(state.build, "", arguments...))
		out.WriteString("\tif !stateOK {\n\t\treturn nil, nil, false\n\t}\n")
	}
	fmt.Fprintf(out, "\tsealed := &%s{rows: make([]%s, 0, len(rows))", familyType, rowType)
	if built.fold.state != nil {
		out.WriteString(", state: state")
	}
	for _, axis := range built.familyAxes {
		fmt.Fprintf(out, ", %s: install.%s", axis.param, axis.param)
	}
	out.WriteString("}\n")
	if built.shape == shapeSelectedRoute {
		out.WriteString("\twidth := plane.RouteWidth()\n")
		out.WriteString("\tif width < 0 {\n\t\treturn nil, nil, false\n\t}\n")
		out.WriteString("\tsealed.plane, sealed.width = plane, width\n")
	}
	for _, join := range memberSetJoins(built) {
		// The cell buffer is sized once at the widest member set any row of
		// this rule declares, so a warm invocation over any row allocates
		// nothing.
		fmt.Fprintf(out, "\t%sWidth := 0\n", join.name)
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
	case shapeExactFold:
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
	if built.shape == shapeExactFold {
		fmt.Fprintf(out, "\t\tcarryMode, carryPresent := planRow.Rule.CarryMode()\n")
		if built.exact.carried {
			fmt.Fprintf(out, "\t\tif !carryPresent || carryMode != %s.CarryIdentity {\n\t\t\treturn nil, nil, false\n\t\t}\n", ruleprogram)
		} else {
			// The declaration names no carry, so a row that arrived carrying
			// one is a plan that no longer matches the declaration this family
			// was emitted from. The undeclared mode is named as the zero it is.
			out.WriteString("\t\tif carryPresent || carryMode != 0 {\n\t\t\treturn nil, nil, false\n\t\t}\n")
		}
	}

	if built.shape == shapeSelectedRoute {
		fmt.Fprintf(out, "\t\tcarryMode, carryPresent := planRow.Rule.CarryMode()\n")
		switch built.route.carryMode {
		case program.CarryTransform:
			fmt.Fprintf(out, "\t\tif !carryPresent || carryMode != %s.CarryTransform {\n\t\t\treturn nil, nil, false\n\t\t}\n", ruleprogram)
		case program.CarryIdentity:
			fmt.Fprintf(out, "\t\tif !carryPresent || carryMode != %s.CarryIdentity {\n\t\t\treturn nil, nil, false\n\t\t}\n", ruleprogram)
		default:
			out.WriteString("\t\tif carryPresent || carryMode != 0 {\n\t\t\treturn nil, nil, false\n\t\t}\n")
		}
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
		if join.memberSet != nil {
			if err := renderMemberSetSeal(out, built, join); err != nil {
				return err
			}
			sealedNames = append(sealedNames, fmt.Sprintf("%s: %sSealed", join.name, join.name),
				fmt.Sprintf("%sPolicy: %sPolicy", join.name, join.name))
			continue
		}
		expression, err := readExpression(built, join, firstExact)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s", expression)
		sealedNames = append(sealedNames, fmt.Sprintf("%s: %sSealed", join.name, join.name))
		if join.read.Form == program.Exact {
			sealedNames = append(sealedNames, fmt.Sprintf("%sPolicy: %sPolicy", join.name, join.name))
		}
	}
	switch built.shape {
	case shapeCarry:
		fmt.Fprintf(out, "\t\twriteSealed, writeSealedOK := plane.RowCarry(planRow, %s)\n",
			imports.methodValue(built.carry.transform.Implementation, "candidate"))
	case shapeExactFold:
		out.WriteString("\t\twriteSealed, writeSealedOK := plane.ExactWrite(planRow.Target, uint16(output.Slot))\n")
	case shapeSelectedRoute:
		out.WriteString("\t\twriteSealed, writeSealedOK := plane.RouteWrite(uint16(output.Slot))\n")
	}
	out.WriteString("\t\tif !writeSealedOK")
	for _, join := range built.joins {
		if join.memberSet != nil {
			continue
		}
		fmt.Fprintf(out, " || !%sSealedOK", join.name)
	}
	out.WriteString(" {\n\t\t\treturn nil, nil, false\n\t\t}\n")
	sealedNames = append(sealedNames, "write: writeSealed")

	fmt.Fprintf(out, "\t\taddresses = append(addresses, %s.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})\n", execution)
	fmt.Fprintf(out, "\t\tsealed.rows = append(sealed.rows, %s{candidate: candidate, %s})\n", rowType, strings.Join(sealedNames, ", "))
	out.WriteString("\t}\n")
	for _, join := range memberSetJoins(built) {
		fmt.Fprintf(out, "\tsealed.%sWidth = %sWidth\n", join.name, join.name)
	}
	out.WriteString("\treturn sealed, addresses, true\n}\n")
	return nil
}

// renderExactDestinationProjector emits the construction-time bridge of a
// heterogeneous exact rule: fetch the candidate from its owner, apply the
// consumer-declared typed destination accessor, then normalize that key with
// the written axis.  This method is the only place both schemas coexist.
func renderExactDestinationProjector(out *strings.Builder, built *plan) error {
	imports := built.imports
	if built.exact == nil {
		return unexpressible(built.target.Spec.Key, "an exact fold with no destination projection", "the installer cannot mint its construction target")
	}
	fmt.Fprintf(out, "func (install %s) ProjectExactDestination(ordinal uint32) (uint64, bool) {\n", installerType)
	out.WriteString("\tif !install.available() {\n\t\treturn 0, false\n\t}\n")
	fmt.Fprintf(out, "\tcandidate, candidateOK := %s\n",
		imports.call(built.candidate.at, "install."+built.candidate.axis.param, "int(ordinal)"))
	out.WriteString("\tif !candidateOK {\n\t\treturn 0, false\n\t}\n")
	key, err := projectionExpressionRefusing(built, built.exact.destination, "candidate", "\t", "return 0, false", out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\tdense, denseOK := %s\n",
		imports.call(built.write.normalizer, "install."+built.write.param, key))
	out.WriteString("\tif !denseOK {\n\t\treturn 0, false\n\t}\n")
	out.WriteString("\treturn uint64(dense), true\n}\n\n")
	return nil
}

// renderMemberSetSeal emits the install-time enumeration of one nested member
// set. The census is the candidate row's own, the coordinate of each ordinal is
// the join's declared key projection over that member row, and the read is
// sealed at that coordinate through the read axis's foreign handle. Nothing
// here consults a route table or mints a selection tag: the set is already
// addressed by (parent, ordinal).
func renderMemberSetSeal(out *strings.Builder, built *plan, join *joinPlan) error {
	imports := built.imports
	execution := imports.use(executionPackagePath)
	dense := imports.typeName(join.axis.dense)
	fact := imports.typeName(join.axis.fact)
	if join.key.Result != join.axis.source.Binding.Key.Carrier {
		return unexpressible(built.target.Spec.Key, "a member key that is not the axis's own key carrier",
			fmt.Sprintf("projection %q publishes %s", join.key.Name, join.key.Result))
	}
	fmt.Fprintf(out, "\t\tforeign%d, foreign%dOK := plane.Foreign(plan%d.Factor)\n", join.position, join.position, join.position)
	fmt.Fprintf(out, "\t\tif !foreign%dOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.position)
	// The set is the plan row's own: the engine enumerated it at the row this
	// read is ADDRESSED by and lowered every member's coordinate here. Walking
	// the candidate's members again would be a second enumeration of one set,
	// and it is only expressible at all when the set happens to live on the
	// candidate's own axis.
	fmt.Fprintf(out, "\t\t%sCount, %sCountOK := planRow.MemberCount(%d)\n", join.name, join.name, join.position)
	fmt.Fprintf(out, "\t\tif !%sCountOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.name)
	fmt.Fprintf(out, "\t\t%sSealed := make([]%s.ExactRead[%s, %s], %sCount)\n", join.name, execution, dense, fact, join.name)
	fmt.Fprintf(out, "\t\tfor index := 0; index < %sCount; index++ {\n", join.name)
	fmt.Fprintf(out, "\t\t\tmemberDense, memberDenseOK := planRow.MemberAt(%d, index)\n", join.position)
	out.WriteString("\t\t\tif !memberDenseOK {\n\t\t\t\treturn nil, nil, false\n\t\t\t}\n")
	fmt.Fprintf(out, "\t\t\tmemberRead, memberReadOK := %s.ForeignMemberExactRead[%s, %s](foreign%d, memberDense, uint16(plan%d.Input))\n",
		execution, dense, fact, join.position, join.position)
	out.WriteString("\t\t\tif !memberReadOK {\n\t\t\t\treturn nil, nil, false\n\t\t\t}\n")
	fmt.Fprintf(out, "\t\t\t%sSealed[index] = memberRead\n", join.name)
	out.WriteString("\t\t}\n")
	fmt.Fprintf(out, "\t\tif %sCount > %sWidth {\n\t\t\t%sWidth = %sCount\n\t\t}\n", join.name, join.name, join.name, join.name)
	fmt.Fprintf(out, "\t\t%sPolicy, %sPolicyOK := %s.ForeignReadCellPolicy[%s, %s](foreign%d, plan%d.Contract)\n",
		join.name, join.name, execution, dense, fact, join.position, join.position)
	fmt.Fprintf(out, "\t\tif !%sPolicyOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.name)
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
		fmt.Fprintf(&out, "\t\t%sPolicy, %sPolicyOK := %s.ForeignReadCellPolicy[%s, %s](foreign%d, plan%d.Contract)\n",
			join.name, join.name, execution, dense, fact, join.position, join.position)
		fmt.Fprintf(&out, "\t\tif !%sPolicyOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.name)
	case join.read.Form == program.Exact:
		if join.position != firstExact {
			return "", unexpressible(built.target.Spec.Key, fmt.Sprintf("a second axis-local exact read at join %d", join.position),
				"only the first exact join's Unit is the plan row's own; a later one needs a row-bound axis-local read primitive, which the execution vocabulary does not publish")
		}
		fmt.Fprintf(&out, "\t\t%sSealed, %sSealedOK := plane.ExactRead(planRow.Unit, uint16(plan%d.Input))\n",
			join.name, join.name, join.position)
		fmt.Fprintf(&out, "\t\t%sPolicy, %sPolicyOK := plane.ReadCellPolicy(plan%d.Contract)\n",
			join.name, join.name, join.position)
		fmt.Fprintf(&out, "\t\tif !%sPolicyOK {\n\t\t\treturn nil, nil, false\n\t\t}\n", join.name)
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

// readCellPolicy is the policy value a sealed selected read is opened with. It
// is the zero policy for every declared sparsity, because the substitution a
// read delivers is derived by the read primitive itself from the contract and
// the Factor it is sealed against - the Factor's own Default fills an unwritten
// coordinate and its own Top widens an opaque one. Naming a substitution here
// would be a second authority over what a delivered cell holds, and the
// primitive does not read one.
func readCellPolicy(built *plan, join *joinPlan) (string, error) {
	imports := built.imports
	execution := imports.use(executionPackagePath)
	return fmt.Sprintf("%s.ReadCellPolicy[%s]{}", execution, imports.typeName(join.axis.fact)), nil
}
