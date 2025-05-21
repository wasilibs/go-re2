package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/curioswitch/go-build"
	"github.com/google/go-github/github"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/boot"
	"github.com/goyek/x/cmd"
)

func main() {
	tags := buildTags()

	build.RegisterTestTask(goyek.Define(goyek.Task{
		Name:  "test-go",
		Usage: "Runs Go tests.",
		Action: func(a *goyek.A) {
			mode := strings.ToLower(os.Getenv("RE2_TEST_MODE"))
			race := "-race"
			if os.Getenv("TEST_NORACE") != "" {
				race = ""
			}
			cmd.Exec(a, fmt.Sprintf(`go test -v -timeout=20m %s -tags "%s" ./...`, race, strings.Join(tags, ",")))
			if mode == "" {
				cmd.Exec(a, fmt.Sprintf("go build -o %s ./internal/e2e", filepath.Join("out", "test.wasm")), cmd.Env("GOOS", "wasip1"), cmd.Env("GOARCH", "wasm"))
				// Could invoke wazero directly but the CLI has a simpler entry point.
				cmd.Exec(a, "go run github.com/tetratelabs/wazero/cmd/wazero@v1.8.2 run "+filepath.Join("out", "test.wasm"))
			}
		},
	}))

	goyek.Define(goyek.Task{
		Name:  "wasm",
		Usage: "Builds the WebAssembly module.",
		Action: func(a *goyek.A) {
			buildWasm(a)
		},
	})

	goyek.Define(goyek.Task{
		Name:  "update",
		Usage: "Checks upstream repo for new version and updates and builds if so.",
		Action: func(a *goyek.A) {
			currBytes, err := os.ReadFile(filepath.Join("buildtools", "wasm", "version.txt"))
			if err != nil {
				a.Fatal(err)
			}
			curr := strings.TrimSpace(string(currBytes))

			gh, err := api.DefaultRESTClient()
			if err != nil {
				a.Fatal(err)
			}

			var latest string
			var release *github.RepositoryRelease
			if err := gh.Get(fmt.Sprintf("repos/%s/releases/latest", "google/re2"), &release); err != nil {
				a.Log(err)
			}

			if release != nil {
				latest = release.GetTagName()
			} else {
				a.Log("could not find releases, falling back to tag")

				var tags []github.RepositoryTag
				if err := gh.Get(fmt.Sprintf("repos/%s/tags", "google/re2"), &tags); err != nil {
					a.Error(err)
				}
				if len(tags) == 0 {
					a.Fatal("could not find tags")
				}
				latest = tags[0].GetName()
			}

			if latest == curr {
				fmt.Println("up to date")
				return
			}

			fmt.Println("updating to", latest)
			if err := os.WriteFile(filepath.Join("buildtools", "wasm", "version.txt"), []byte(latest), 0o600); err != nil {
				a.Error(err)
			}

			buildWasm(a)
		},
	})

	defineBenchTasks("bench", "./...")
	defineBenchTasks("wafbench", "./wafbench")

	build.DefineTasks(
		build.Tags(tags...),
		build.ExcludeTasks("test-go"),
	)

	boot.Main()
}

func buildTags() []string {
	mode := strings.ToLower(os.Getenv("RE2_TEST_MODE"))
	exhaustive := os.Getenv("RE2_TEST_EXHAUSTIVE") == "1"

	var tags []string
	if mode == "cgo" {
		tags = append(tags, "re2_cgo")
	}
	if exhaustive {
		tags = append(tags, "re2_test_exhaustive")
	}

	return tags
}

func buildWasm(a *goyek.A) {
	if !cmd.Exec(a, fmt.Sprintf("docker build -t wasilibs-build -f %s .", filepath.Join("buildtools", "wasm", "Dockerfile"))) {
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		a.Fatal(err)
	}
	wasmDir := filepath.Join(wd, "internal", "wasm")
	if err := os.MkdirAll(wasmDir, 0o750); err != nil {
		a.Fatal(err)
	}
	cmd.Exec(a, fmt.Sprintf("docker run --rm -v %s:/out wasilibs-build", wasmDir))
}

type benchMode int

const (
	benchModeWazero benchMode = iota
	benchModeCGO
	benchModeSTDLib
)

func benchArgs(pkg string, count int, mode benchMode) string {
	args := []string{"test", "-bench=.", "-run=^$", "-v", "-timeout=60m"}
	if count > 0 {
		args = append(args, fmt.Sprintf("-count=%d", count))
	}
	switch mode {
	case benchModeCGO:
		args = append(args, "-tags=re2_cgo")
	case benchModeSTDLib:
		args = append(args, "-tags=re2_bench_stdlib")
	case benchModeWazero:
		// no args
	}
	args = append(args, pkg)

	return strings.Join(args, " ")
}

func defineBenchTasks(name string, pkg string) {
	goyek.Define(goyek.Task{
		Name:  name,
		Usage: "Runs benchmarks in the default configuration for a Go app, using wazero.",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "go "+benchArgs(pkg, 1, benchModeWazero))
		},
	})

	goyek.Define(goyek.Task{
		Name:  name + "-cgo",
		Usage: "Runs benchmarks with re2 accessed using cgo. A C++ toolchain and libre2 must be installed to run.",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "go "+benchArgs(pkg, 1, benchModeCGO))
		},
	})

	goyek.Define(goyek.Task{
		Name:  name + "-stdlib",
		Usage: "Runs benchmarks using the regexp library in the standard library for comparison.",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "go "+benchArgs(pkg, 1, benchModeSTDLib))
		},
	})

	goyek.Define(goyek.Task{
		Name:  name + "-all",
		Usage: "Runs all benchmark types and outputs with benchstat. A C++ toolchain and libre2 must be installed to run.",
		Action: func(a *goyek.A) {
			if err := os.MkdirAll("out", 0o750); err != nil {
				a.Errorf("create out directory: %v", err)
			}

			var stdout bytes.Buffer
			cmd.Exec(a, "go "+benchArgs(pkg, 5, benchModeWazero), cmd.Stdout(&stdout))
			if err := os.WriteFile(filepath.Join("out", name+".txt"), stdout.Bytes(), 0o600); err != nil {
				a.Errorf("write bench.txt: %v", err)
			}

			stdout.Reset()
			cmd.Exec(a, "go "+benchArgs(pkg, 5, benchModeCGO), cmd.Stdout(&stdout))
			if err := os.WriteFile(filepath.Join("out", name+"-cgo.txt"), stdout.Bytes(), 0o600); err != nil {
				a.Errorf("write bench-cgo.txt: %v", err)
			}

			stdout.Reset()
			cmd.Exec(a, "go "+benchArgs(pkg, 5, benchModeSTDLib), cmd.Stdout(&stdout))
			if err := os.WriteFile(filepath.Join("out", name+"-stdlib.txt"), stdout.Bytes(), 0o600); err != nil {
				a.Errorf("write bench-stdlib.txt: %v", err)
			}

			cmd.Exec(a, fmt.Sprintf("go run golang.org/x/perf/cmd/benchstat@%s %s %s %s", verBenchstat,
				filepath.Join("out", name+"-stdlib.txt"), filepath.Join("out", name+".txt"), filepath.Join("out", name+"-cgo.txt")))
		},
	})
}
