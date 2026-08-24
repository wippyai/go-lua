package engine

type Widget struct{}

func NewWidget() *Widget {
	return &Widget{}
}

func (w *Widget) Name() string {
	return "widget"
}

func helper() int {
	return 1
}

type internalThing struct{}
