module github.com/wasilibs/go-re2/e2e

go 1.22.0

require github.com/wasilibs/go-re2 v1.5.2

require (
	github.com/tetratelabs/wazero v1.9.0 // indirect
	github.com/wasilibs/wazero-helpers v0.0.0-20240620070341-3dff1577cd52 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/wasilibs/go-re2 => ../..
