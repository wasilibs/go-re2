//go:build tinygo.wasm

package re2

/*
#cgo LDFLAGS: ${SRCDIR}/wasm/libc++.a ${SRCDIR}/wasm/libc++abi.a ${SRCDIR}/wasm/libcre2.a ${SRCDIR}/wasm/libre2.a
*/
import "C"
