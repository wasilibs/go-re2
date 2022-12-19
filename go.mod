module github.com/wasilibs/go-re2

go 1.18

require (
	github.com/magefile/mage v1.14.0
	github.com/tetratelabs/wazero v1.0.0-pre.4.0.20221213074253-2e13f57f56a1
	github.com/wasilibs/go-re2/cre2 v0.0.0-00010101000000-000000000000
)

replace github.com/wasilibs/go-re2/cre2 => ./cre2
