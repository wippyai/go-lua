package typ

import (
	"reflect"
	"unsafe"
)

func typePointer(t Type) uintptr {
	switch tt := t.(type) {
	case *Union:
		return uintptr(unsafe.Pointer(tt))
	case *Intersection:
		return uintptr(unsafe.Pointer(tt))
	case *Record:
		return uintptr(unsafe.Pointer(tt))
	case *Function:
		return uintptr(unsafe.Pointer(tt))
	case *Generic:
		return uintptr(unsafe.Pointer(tt))
	case *Instantiated:
		return uintptr(unsafe.Pointer(tt))
	case *Interface:
		return uintptr(unsafe.Pointer(tt))
	case *Recursive:
		return uintptr(unsafe.Pointer(tt))
	}

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}

	return v.Pointer()
}
