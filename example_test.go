package re2_test

import (
	"fmt"
	regexp "github.com/anuraaga/re2-go"
)

func ExampleRegexp_FindStringSubmatch() {
	re := regexp.MustCompile(`a(x*)b(y|z)c`)
	fmt.Printf("%q\n", re.FindStringSubmatch("-axxxbyc-"))
	fmt.Printf("%q\n", re.FindStringSubmatch("-abzc-"))
	// Output:
	// ["axxxbyc" "xxx" "y"]
	// ["abzc" "" "z"]
}

func ExampleRegexp_FindAllString() {
	re := regexp.MustCompile(`a.`)
	fmt.Println(re.FindAllString("paranormal", -1))
	fmt.Println(re.FindAllString("paranormal", 2))
	fmt.Println(re.FindAllString("graal", -1))
	fmt.Println(re.FindAllString("none", -1))
	// Output:
	// [ar an al]
	// [ar an]
	// [aa]
	// []
}
