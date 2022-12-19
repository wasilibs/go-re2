//go:build !re2_bench_stdlib

package wafbench

import (
	"github.com/corazawaf/coraza/v3/operators"
	"github.com/corazawaf/coraza/v3/rules"

	"github.com/wasilibs/go-re2"
)

type rx struct {
	re *re2.Regexp
}

var _ rules.Operator = (*rx)(nil)

func newRX(options rules.OperatorOptions) (rules.Operator, error) {
	o := &rx{}
	data := options.Arguments

	re, err := re2.Compile(data)
	if err != nil {
		return nil, err
	}

	o.re = re
	return o, err
}

func (o *rx) Evaluate(tx rules.TransactionState, value string) bool {
	match := o.re.FindStringSubmatch(value)
	if len(match) == 0 {
		return false
	}

	if tx.Capturing() {
		for i, c := range match {
			if i == 9 {
				return true
			}
			tx.CaptureField(i, c)
		}
	}

	return true
}

func init() {
	operators.Register("rx", newRX)
}
