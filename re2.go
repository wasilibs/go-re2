package re2

type Regexp struct {
	ptr uintptr
	// Find methods seem to require the pattern to be enclosed in parentheses, so we keep a second
	// regex for them.
	parensPtr uintptr
	numGroups int
}

func MustCompile(str string) *Regexp {
	re, err := Compile(str)
	if err != nil {
		panic(err)
	}
	return re
}

func Compile(str string) (*Regexp, error) {
	return newRE(str)
}

func (r *Regexp) MatchString(s string) bool {
	return matchString(r, s)
}

func (r *Regexp) Match(s []byte) bool {
	return match(r, s)
}

func (r *Regexp) FindAllString(s string, n int) []string {
	return findAllString(r, s, n)
}

func (r *Regexp) FindStringSubmatch(s string) []string {
	return findStringSubmatch(r, s)
}
