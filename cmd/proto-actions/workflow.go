package main

import (
	"fmt"
	"strings"
)

// workflowPath is where the run's workflow file lives in the repository.
func (r run) workflowPath() string {
	return ".github/workflows/" + strings.ToLower(r.workflow) + ".yml"
}

// workflowYAML is a fake workflow file that matches the run's jobs, needs and
// matrix.
func (r run) workflowYAML() string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("name: %s", r.workflow)
	w("")
	w("on:")
	switch r.event {
	case "schedule":
		w("  schedule:")
		w("    - cron: \"0 3 * * *\"")
	case "pull_request":
		w("  pull_request:")
		w("    branches: [main]")
	case "workflow_dispatch":
		w("  workflow_dispatch:")
	default:
		w("  push:")
		w("    branches: [main]")
		w("  pull_request:")
	}
	w("")
	w("permissions:")
	w("  contents: read")
	w("")
	w("jobs:")
	seen := map[string]bool{}
	for _, j := range r.jobs {
		id := j.name
		if j.group != "" {
			id = j.group
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		w("  %s:", id)
		if len(j.needs) > 0 {
			w("    needs: [%s]", strings.Join(j.needs, ", "))
		}
		if j.group != "" {
			goV, oses := r.matrixAxes(j.group)
			w("    runs-on: ${{ matrix.os }}-latest")
			w("    strategy:")
			w("      fail-fast: false")
			w("      matrix:")
			w("        go: [%s]", strings.Join(goV, ", "))
			w("        os: [%s]", strings.Join(oses, ", "))
		} else {
			w("    runs-on: ubuntu-latest")
		}
		w("    steps:")
		w("      - uses: actions/checkout@v4")
		if id != "lint" && id != "release" && id != "deploy" && id != "package" {
			w("      - uses: actions/setup-go@v5")
			w("        with:")
			if j.group != "" {
				w("          go-version: ${{ matrix.go }}")
			} else {
				w("          go-version: \"1.24\"")
			}
		}
		switch id {
		case "lint":
			w("      - uses: golangci/golangci-lint-action@v8")
		case "vet":
			w("      - run: go vet ./...")
		case "setup":
			w("      - run: go mod download")
		case "build":
			w("      - run: go build ./...")
		case "test":
			w("      - run: go test -race ./...")
		case "e2e":
			w("      - run: make e2e")
		case "package":
			w("      - run: make package")
			w("      - uses: actions/upload-artifact@v4")
			w("        with:")
			w("          name: gh-tui-linux-amd64")
			w("          path: dist/")
		case "release", "deploy":
			w("      - run: gh release create ${{ github.ref_name }} dist/*")
			w("        env:")
			w("          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}")
		default:
			w("      - run: make %s", id)
		}
	}
	return b.String()
}

// matrixAxes are the go versions and systems of a matrix parent's jobs, from
// names such as "test (go 1.23, ubuntu)".
func (r run) matrixAxes(group string) (goV, oses []string) {
	add := func(s *[]string, v string) {
		for _, x := range *s {
			if x == v {
				return
			}
		}
		*s = append(*s, v)
	}
	for _, j := range r.jobs {
		if j.group != group {
			continue
		}
		_, in, ok := strings.Cut(j.name, " (")
		if !ok {
			continue
		}
		for _, p := range strings.Split(strings.TrimSuffix(in, ")"), ", ") {
			if v, ok := strings.CutPrefix(p, "go "); ok {
				add(&goV, "\""+v+"\"")
			} else {
				add(&oses, p)
			}
		}
	}
	return goV, oses
}
