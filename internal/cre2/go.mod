// A go.mod is required here to prevent cgo from being required even when the re2_cgo build tag
// isn't enabled. It seems Go scans all folders, even if they are not actually imported.

module github.com/wasilibs/go-re2/internal/cre2

go 1.18
