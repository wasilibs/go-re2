module github.com/wasilibs/go-re2/e2e

go 1.25.0

require github.com/wasilibs/go-re2 v1.5.2

require (
	github.com/tetratelabs/wazero v1.11.0 // indirect
	github.com/wasilibs/wazero-helpers v0.0.0-20250123031827-cd30c44769bb // indirect
	golang.org/x/sys v0.44.0 // indirect
)

replace github.com/wasilibs/go-re2 => ../..
