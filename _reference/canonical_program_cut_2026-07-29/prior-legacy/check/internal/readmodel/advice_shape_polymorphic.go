package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

func (r Reader) ForEachShapePolymorphic(visit func(ShapePolymorphic) bool) bool {
	return projectBodyOccurrences(r, visit, (*body.Result).ForEachShapePolymorphicOccurrence, shapePolymorphicFromBody)
}

func shapePolymorphicFromBody(r Reader, occ body.ShapePolymorphicOccurrence) ShapePolymorphic {
	fields := make([]ShapeConditionalField, 0, len(occ.ConditionalFields))
	for _, field := range occ.ConditionalFields {
		fields = append(fields, ShapeConditionalField{Point: field.Point, Name: field.Name, Label: r.splitBirthFieldLabel(occ.Receiver, field.Name), Span: sourceSpanFromBody(field.Span)})
	}
	return ShapePolymorphic{Point: occ.Point, ReceiverLabel: r.displayPathCanonical(occ.Receiver), BirthPoint: occ.BirthPoint, BirthSpan: sourceSpanFromBodyRaw(occ.BirthSpan), UsePoint: occ.UsePoint, UseSpan: sourceSpanFromBody(occ.UseSpan), ConditionalFields: fields, UnionFields: append([]string(nil), occ.UnionFields...)}
}
