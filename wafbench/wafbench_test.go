package wafbench

import (
	"archive/zip"
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/corazawaf/coraza/v3"
	txhttp "github.com/corazawaf/coraza/v3/http"
	"github.com/corazawaf/coraza/v3/types"
)

//go:embed coreruleset-32e6d80419d386a330ddaf5e60047a4a1c38a160.zip
var crsZip []byte

//go:embed coraza.conf-recommended
var confRecommended string

func BenchmarkWAF(b *testing.B) {
	crsReader, err := zip.NewReader(bytes.NewReader(crsZip), int64(len(crsZip)))
	if err != nil {
		b.Fatal(err)
	}

	crs, err := fs.Sub(crsReader, "coreruleset-32e6d80419d386a330ddaf5e60047a4a1c38a160")
	if err != nil {
		b.Fatal(err)
	}

	conf := coraza.NewWAFConfig()
	customTestingConfig := `
SecResponseBodyMimeType text/plain
SecDefaultAction "phase:3,log,auditlog,pass"
SecDefaultAction "phase:4,log,auditlog,pass"
SecAction "id:900005,\
  phase:1,\
  nolog,\
  pass,\
  ctl:ruleEngine=DetectionOnly,\
  ctl:ruleRemoveById=910000,\
  # Interferes with ftw log scanning
  ctl:ruleRemoveById=920250,\
  setvar:tx.paranoia_level=4,\
  setvar:tx.crs_validate_utf8_encoding=1,\
  setvar:tx.arg_name_length=100,\
  setvar:tx.arg_length=400,\
  setvar:tx.total_arg_length=64000,\
  setvar:tx.max_num_args=255,\
  setvar:tx.max_file_size=64100,\
  setvar:tx.combined_file_sizes=65535"
# Write the value from the X-CRS-Test header as a marker to the log
SecRule REQUEST_HEADERS:X-CRS-Test "@rx ^.*$" \
  "id:999999,\
  phase:1,\
  log,\
  msg:'X-CRS-Test %{MATCHED_VAR}',\
  pass,\
  t:none"
`
	// Configs are loaded with a precise order:
	// 1. Coraza config
	// 2. Custom Rules for testing and eventually overrides of the basic Coraza config
	// 3. CRS basic config
	// 4. CRS rules (on top of which are applied the previously defined SecDefaultAction)
	conf = conf.
		WithRootFS(crs).
		WithDirectives(confRecommended).
		WithDirectives(customTestingConfig).
		WithDirectives("Include crs-setup.conf.example").
		WithDirectives("Include rules/*.conf")

	errorPath := filepath.Join(b.TempDir(), "error.log")
	errorFile, err := os.Create(errorPath)
	if err != nil {
		b.Fatalf("failed to create error log: %v", err)
	}
	errorWriter := bufio.NewWriter(errorFile)
	conf = conf.WithErrorLogger(func(rule types.MatchedRule) {
		msg := rule.ErrorLog(0)
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

	s := httptest.NewServer(txhttp.WrapHandler(waf, b.Logf, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				_, err := http.Post(s.URL+"/anything", "text/plain", strings.NewReader(payload))
				if err != nil {
					b.Error(err)
				}
			}
		})
	}
}
