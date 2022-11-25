//go:build tinygo.wasm

package re2

/*
#cgo LDFLAGS: -Lwasm -lcre2 -lre2
*/
import "C"
