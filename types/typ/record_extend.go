package typ

// ExtendRecordWithField returns a record type extended with a field.
// A dynamic base stays dynamic (WriteInto). If the base type is absent, unknown,
// or Lua nil, creates a new record with just the field. If the base type is
// already a record, adds or updates the field.
func ExtendRecordWithField(base Type, field string, fieldType Type) Type {
	if field == "" || fieldType == nil {
		return base
	}
	return WriteInto(base, func(base Type) Type {
		return extendRecordWithField(base, field, fieldType)
	})
}

func extendRecordWithField(base Type, field string, fieldType Type) Type {
	valueType, optional := SplitNilableFieldType(fieldType)
	addField := func(builder *RecordBuilder) {
		if optional {
			builder.OptField(field, valueType)
		} else {
			builder.Field(field, valueType)
		}
	}
	unwrapped := base
	for a, ok := unwrapped.(*Alias); ok; a, ok = unwrapped.(*Alias) {
		unwrapped = a.Target
	}
	if unwrapped == nil || unwrapped.Kind() == Unknown.Kind() || unwrapped.Kind() == Nil.Kind() {
		builder := NewRecord().SetOpen(true)
		addField(builder)
		return builder.Build()
	}

	rec, ok := unwrapped.(*Record)
	if !ok {
		return base
	}

	builder := rec.Builder()
	builder.fields = nil
	added := false
	for _, f := range rec.Fields {
		if f.Name == field {
			addField(builder)
			added = true
			continue
		}
		builder.AddField(f)
	}
	if !added {
		addField(builder)
	}
	return builder.Build()
}
