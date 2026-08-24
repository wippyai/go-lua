package areaa

// Widget is a plain value with no legacy tokens.
type Widget struct {
	Name string
}

func NewWidget(name string) Widget {
	return Widget{Name: name}
}
