package census

import (
	"sort"
	"strconv"
)

// FieldMemberRow names one member of the closed constant family a carrier is
// declared over. It refines the same carrier addressed by a field-state row.
func FieldMemberRow(form, field, member string) string {
	return "member:" + form + "." + field + "@" + member
}

// ProductRow names one construction of one action. Owner and ordinal are both
// required because one action may construct the same form more than once.
func ProductRow(owner string, ordinal int, constructor string) string {
	return "product:" + owner + "#" + strconv.Itoa(ordinal) + ":" + constructor
}

// products projects the per-construction denominator. Builds records assigned
// carriers; Discriminants records the exact closed-family member left in each
// carrier, including an omitted coordinate whose family declares a zero member.
func products(value Census) []Row {
	result := make([]Row, 0, len(value.Products))
	for _, product := range value.Products {
		row := Row{
			Key:        ProductRow(product.Owner, product.Ordinal, product.Constructor),
			Kind:       RowProduct,
			Constructs: FormRow(product.Constructor),
		}
		for _, field := range product.Fields {
			if field.Member != "" {
				row.Discriminants = append(row.Discriminants, FieldMemberRow(product.Constructor, field.Field, field.Member))
			}
			if field.Assigned {
				row.Builds = append(row.Builds, CarrierRow(product.Constructor, field.Field))
			}
		}
		result = append(result, row)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}
