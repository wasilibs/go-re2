//go:build re2_cgo

package re2

import (
	"reflect"
	"unsafe"
)

func aliasString(sPtr unsafe.Pointer, size int) string {
	return *(*string)(unsafe.Pointer(&reflect.StringHeader{
		Data: uintptr(sPtr),
		Len:  size,
	}))
}
