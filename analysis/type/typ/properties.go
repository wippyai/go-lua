package typ

type typeProperties struct {
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsGeneric       bool
	containsRecursive     bool
	containsOpenRecursive bool
}

func typePropertiesOf(types ...Type) typeProperties {
	var props typeProperties
	for _, t := range types {
		props.include(t)
	}
	return props
}

func typePropertiesOfFields(fields []Field) typeProperties {
	var props typeProperties
	props.includeFields(fields)
	return props
}

func typePropertiesOfMethods(methods []Method) typeProperties {
	var props typeProperties
	props.includeMethods(methods)
	return props
}

func typePropertiesOfUnionMembers(types []Type) typeProperties {
	var props typeProperties
	props.includeUnionMembers(types)
	return props
}

func typePropertiesOfTypeParams(params []*TypeParam) typeProperties {
	var props typeProperties
	props.includeTypeParams(params)
	return props
}

func (p *typeProperties) include(t Type) {
	p.includeWithOpenRecursive(t, knownContainsOpenRecursive)
}

func (p *typeProperties) includeWithOpenRecursive(t Type, openRecursive func(Type) bool) {
	if !p.containsAny && knownContainsAny(t) {
		p.containsAny = true
	}
	if !p.containsNever && knownContainsNever(t) {
		p.containsNever = true
	}
	if !p.containsTypeParam && knownContainsTypeParam(t) {
		p.containsTypeParam = true
	}
	if !p.containsInstantiated && knownContainsInstantiated(t) {
		p.containsInstantiated = true
	}
	if !p.containsGeneric && knownContainsGeneric(t) {
		p.containsGeneric = true
	}
	if !p.containsRecursive && knownContainsRecursive(t) {
		p.containsRecursive = true
	}
	if !p.containsOpenRecursive && openRecursive != nil && openRecursive(t) {
		p.containsOpenRecursive = true
	}
}

func (p *typeProperties) includeTypes(types ...Type) {
	for _, t := range types {
		p.include(t)
	}
}

func (p *typeProperties) includeFields(fields []Field) {
	for _, f := range fields {
		p.include(f.Type)
	}
}

func (p *typeProperties) includeStaticMembers(members []StaticMember) {
	for _, m := range members {
		p.include(m.Type)
	}
}

func (p *typeProperties) includeMethods(methods []Method) {
	for _, m := range methods {
		p.include(m.Type)
	}
}

func (p *typeProperties) includeUnionMembers(types []Type) {
	for _, t := range types {
		p.includeUnionMember(t)
	}
}

func (p *typeProperties) includeUnionMember(t Type) {
	p.includeWithOpenRecursive(t, unionMemberContainsOpenRecursive)
}

func (p *typeProperties) includeParams(params []Param) {
	for _, param := range params {
		p.include(param.Type)
	}
}

func (p *typeProperties) includeTypeParams(params []*TypeParam) {
	for _, param := range params {
		if param != nil {
			p.include(param)
		}
	}
}
