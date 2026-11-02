//go:build re2_cgo

package cre2

/*
#cgo pkg-config: re2 absl_fixed_array
#cgo CXXFLAGS: -std=c++17
*/
import "C"
