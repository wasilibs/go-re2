//go:build !tinygo.wasm && !re2_cgo && !re2_wasm2go && !wasm

package internal

const defaultMaxPages = uint32(65536)
