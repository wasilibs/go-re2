package wafbench

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
	txhttp "github.com/corazawaf/coraza/v3/http"
	"github.com/corazawaf/coraza/v3/types"
)

//go:embed coraza.conf-recommended
var confRecommended string

func BenchmarkWAF(b *testing.B) {
	conf := coraza.NewWAFConfig()
	customTestingConfig := `
SecResponseBodyMimeType text/plain
SecDefaultAction "phase:3,log,auditlog,pass"
SecDefaultAction "phase:4,log,auditlog,pass"
SecDefaultAction "phase:5,log,auditlog,pass"

# Rule 900005 from https://github.com/coreruleset/coreruleset/blob/v4.0/dev/tests/regression/README.md#requirements
SecAction "id:900005,\
  phase:1,\
  nolog,\
  pass,\
  ctl:ruleEngine=DetectionOnly,\
  ctl:ruleRemoveById=910000,\
  setvar:tx.blocking_paranoia_level=4,\
  setvar:tx.crs_validate_utf8_encoding=1,\
  setvar:tx.arg_name_length=100,\
  setvar:tx.arg_length=400,\
  setvar:tx.total_arg_length=64000,\
  setvar:tx.max_num_args=255,\
  setvar:tx.max_file_size=64100,\
  setvar:tx.combined_file_sizes=65535"

# Write the value from the X-CRS-Test header as a marker to the log
# Requests with X-CRS-Test header will not be matched by any rule. See https://github.com/coreruleset/go-ftw/pull/133
SecRule REQUEST_HEADERS:X-CRS-Test "@rx ^.*$" \
  "id:999999,\
  phase:1,\
  pass,\
  t:none,\
  log,\
  msg:'X-CRS-Test %{MATCHED_VAR}',\
  ctl:ruleRemoveById=1-999999"
`
	// Configs are loaded with a precise order:
	// 1. Coraza config
	// 2. Custom Rules for testing and eventually overrides of the basic Coraza config
	// 3. CRS basic config
	// 4. CRS rules (on top of which are applied the previously defined SecDefaultAction)
	conf = conf.
		WithRootFS(coreruleset.FS).
		WithDirectives(confRecommended).
		WithDirectives(customTestingConfig).
		WithDirectives("Include @crs-setup.conf.example").
		WithDirectives("Include @owasp_crs/*.conf")

	errorPath := filepath.Join(b.TempDir(), "error.log")
	errorFile, err := os.Create(errorPath)
	if err != nil {
		b.Fatalf("failed to create error log: %v", err)
	}
	errorWriter := bufio.NewWriter(errorFile)
	conf = conf.WithErrorCallback(func(rule types.MatchedRule) {
		msg := rule.ErrorLog() + "\n"
		if _, err := io.WriteString(errorWriter, msg); err != nil {
			b.Fatal(err)
		}
		if err := errorWriter.Flush(); err != nil {
			b.Fatal(err)
		}
	})

	waf, err := coraza.NewWAF(conf)
	if err != nil {
		b.Fatal(err)
	}

	s := httptest.NewServer(txhttp.WrapHandler(waf, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Emulated httpbin behaviour: /anything endpoint acts as an echo server, writing back the request body
		if r.URL.Path == "/anything" {
			defer r.Body.Close()
			w.Header().Set("Content-Type", "text/plain")
			_, err = io.Copy(w, r.Body)
			if err != nil {
				b.Fatalf("handler can not read request body: %v", err)
			}
		} else {
			fmt.Fprintf(w, "Hello!")
		}
	})))
	defer s.Close()

	for _, size := range []int{1, 1000, 10000, 100000} {
		payload := strings.Repeat("a", size)
		b.Run(fmt.Sprintf("POST/%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resp, err := http.Post(s.URL+"/anything", "text/plain", strings.NewReader(payload))
				if err != nil {
					b.Error(err)
				}
				if resp.Body != nil {
					resp.Body.Close()
				}
			}
		})
	}
}
