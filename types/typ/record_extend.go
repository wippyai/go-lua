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
	unwrapped := base
	for a, ok := unwrapped.(*Alias); ok; a, ok = unwrapped.(*Alias) {
		unwrapped = a.Target
	}
	if unwrapped == nil || unwrapped.Kind() == Unknown.Kind() || unwrapped.Kind() == Nil.Kind() {
		return NewRecord().SetOpen(true).Field(field, fieldType).Build()
	}

	rec, ok := unwrapped.(*Record)
	if !ok {
		return base
	}

	builder := NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	added := false
	for _, f := range rec.Fields {
		if f.Name == field {
			builder.Field(f.Name, fieldType)
			added = true
			continue
		}
		builder.AddField(f)
	}
	if !added {
		builder.Field(field, fieldType)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}
