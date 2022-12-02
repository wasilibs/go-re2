//go:build tinygo.wasm

package re2

import (
	"reflect"
	"unsafe"
)

func aliasString(sPtr unsafe.Pointer, size int) string {
	return *(*string)(unsafe.Pointer(&reflect.StringHeader{
		Data: uintptr(sPtr),
		Len:  uintptr(size),
	}))
}
