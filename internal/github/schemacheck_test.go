package github

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/vektah/gqlparser/v2"
	gqlast "github.com/vektah/gqlparser/v2/ast"
	gqlparse "github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
	"go.yaml.in/yaml/v3"
)

// The schema check holds what the client sends against GitHub's published
// schemas: its GraphQL operations against the GraphQL schema, and its
// REST calls against the OpenAPI description, of the oldest supported
// GitHub Enterprise Server (core.MinEnterprise) and of github.com. The
// check itself, of downloaded schemas, runs with the schemacheck build
// tag (schema_ci_test.go); the tests here run it on small schemas, and
// hold the lists below to the code.

// graphqlOperations are every GraphQL query and mutation the client
// sends, by the name of the variable that holds it.
var graphqlOperations = map[string]string{
	"listReposQuery":           listReposQuery,
	"getRepoQuery":             getRepoQuery,
	"viewerOwnReposQuery":      viewerOwnReposQuery,
	"orgReposQuery":            orgReposQuery,
	"repoUsersQuery":           repoUsersQuery,
	"viewerLoginQuery":         viewerLoginQuery,
	"multiSearchQuery":         multiSearchQuery,
	"listPullsQuery":           listPullsQuery,
	"getPullQuery":             getPullQuery,
	"pullReviewsQuery":         pullReviewsQuery,
	"pullIDQuery":              pullIDQuery,
	"pullHeadQuery":            pullHeadQuery,
	"mergePullMutation":        mergePullMutation,
	"closePullMutation":        closePullMutation,
	"reopenPullMutation":       reopenPullMutation,
	"readyPullMutation":        readyPullMutation,
	"toDraftPullMutation":      toDraftPullMutation,
	"viewerHeaderQuery":        viewerHeaderQuery,
	"searchPullsQuery":         searchPullsQuery,
	"viewerWorkQuery":          viewerWorkQuery,
	"pullChecksQuery":          pullChecksQuery,
	"commitChecksQuery":        commitChecksQuery,
	"viewerContributionsQuery": viewerContributionsQuery,
}

// A field of a REST answer tagged schema:"optional" may be absent, as on
// an older GitHub, which its decoder takes for unknown: the check only
// holds it to what a schema that has it says of it.

// restCall is one REST call of the client: the method, the path template
// as GitHub's OpenAPI description names it, the query parameters it may
// send, a media type it asks for other than the default, and a value of
// the type it decodes the answer into, whose json tags say the fields it
// reads, or nil. Func is the function that makes the call.
type restCall struct {
	Func, Method, Path string
	Query              []string
	Accept             string
	Decode             any
}

// restCalls are every REST call the client makes.
var restCalls = []restCall{
	{Func: "ListAnnotations", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/check-runs/{check_run_id}/annotations", Query: []string{"per_page"}, Decode: []checkAnnotation{}},
	{Func: "ListBranches", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/branches", Query: []string{"per_page"}, Decode: []restBranch{}},
	{Func: "ListRuns", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/runs", Query: []string{"branch", "event", "status", "actor", "head_sha", "per_page"}, Decode: restRuns{}},
	{Func: "ListRuns", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", Query: []string{"branch", "event", "status", "actor", "head_sha", "per_page"}, Decode: restRuns{}},
	{Func: "GetRun", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}", Decode: restRun{}},
	{Func: "ListWorkflows", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/workflows", Query: []string{"per_page"}, Decode: restWorkflows{}},
	{Func: "ListJobs", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}/jobs", Query: []string{"filter", "per_page"}, Decode: restJobs{}},
	{Func: "ListJobs", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}/jobs", Query: []string{"per_page"}, Decode: restJobs{}},
	{Func: "GetJob", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/jobs/{job_id}", Decode: restJob{}},
	{Func: "runAction", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}/rerun"},
	{Func: "runAction", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs"},
	{Func: "runAction", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/actions/jobs/{job_id}/rerun"},
	{Func: "runAction", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/actions/runs/{run_id}/cancel"},
	{Func: "logLocationOf", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/actions/jobs/{job_id}/logs"},
	{Func: "FilterIssues", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/issues", Query: []string{"sort", "direction", "state", "labels", "assignee", "creator", "mentioned", "milestone", "per_page"}, Decode: []restIssue{}},
	{Func: "GetIssue", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/issues/{issue_number}", Decode: restIssue{}},
	{Func: "GetIssueKind", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/issues/{issue_number}", Decode: restIssue{}},
	{Func: "ListIssueComments", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/issues/{issue_number}/comments", Query: []string{"per_page"}, Decode: []issueComment{}},
	{Func: "ProbeIssues", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/issues", Query: []string{"state", "sort", "direction", "per_page"}},
	{Func: "ProbePullRequests", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/pulls", Query: []string{"state", "sort", "direction", "per_page"}},
	{Func: "SetIssueState", Method: http.MethodPatch, Path: "/repos/{owner}/{repo}/issues/{issue_number}", Decode: restIssue{}},
	{Func: "AddIssueLabels", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/issues/{issue_number}/labels", Decode: []label{}},
	{Func: "RemoveIssueLabel", Method: http.MethodDelete, Path: "/repos/{owner}/{repo}/issues/{issue_number}/labels/{name}", Decode: []label{}},
	{Func: "CreateIssueComment", Method: http.MethodPost, Path: "/repos/{owner}/{repo}/issues/{issue_number}/comments", Decode: issueComment{}},
	{Func: "ListCommits", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/commits", Query: []string{"sha", "per_page"}, Decode: []restCommit{}},
	{Func: "GetCommit", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/commits/{ref}", Decode: restCommitDetail{}},
	{Func: "ListCommitFiles", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/commits/{ref}", Decode: restCommitFiles{}},
	{Func: "Compare", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/compare/{basehead}", Query: []string{"per_page", "page"}, Decode: restCompare{}},
	{Func: "ListLabels", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/labels", Query: []string{"per_page"}, Decode: []label{}},
	{Func: "ListMilestones", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/milestones", Query: []string{"state", "sort", "direction", "per_page"}, Decode: []restMilestone{}},
	{Func: "rateLimits", Method: http.MethodGet, Path: "/rate_limit", Decode: restRateLimits{}},
	{Func: "ListNotifications", Method: http.MethodGet, Path: "/notifications", Query: []string{"per_page", "all", "participating"}, Decode: []notification{}},
	{Func: "MarkThreadRead", Method: http.MethodPatch, Path: "/notifications/threads/{thread_id}"},
	{Func: "MarkThreadDone", Method: http.MethodDelete, Path: "/notifications/threads/{thread_id}"},
	{Func: "MarkNotificationsRead", Method: http.MethodPut, Path: "/notifications"},
	{Func: "GetRelease", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/releases/{release_id}", Decode: restRelease{}},
	{Func: "GetRepo", Method: http.MethodGet, Path: "/repos/{owner}/{repo}", Decode: restRepoFlags{}},
	{Func: "Star", Method: http.MethodPut, Path: "/user/starred/{owner}/{repo}"},
	{Func: "Unstar", Method: http.MethodDelete, Path: "/user/starred/{owner}/{repo}"},
	{Func: "search", Method: http.MethodGet, Path: "/search/repositories", Query: []string{"q", "per_page"}, Decode: searchResults[searchRepo]{}},
	{Func: "search", Method: http.MethodGet, Path: "/search/issues", Query: []string{"q", "per_page"}, Decode: searchResults[searchIssue]{}},
	{Func: "SearchCode", Method: http.MethodGet, Path: "/search/code", Query: []string{"q", "per_page"}, Accept: codeSearchAccept, Decode: codeSearchResults{}},
	{Func: "getTree", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/git/trees/{tree_sha}", Query: []string{"recursive"}, Decode: restTree{}},
	{Func: "GetBlob", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/git/blobs/{file_sha}", Accept: rawAccept},
}

// graphqlOptional are the fields, as Type.field, that the client selects
// though an Enterprise Server it supports may lack them: it leaves them
// out of what it sends to such a server, and reads them as unsaid.
var graphqlOptional = []string{
	// Came in GitHub Enterprise Server 3.15.
	"PullRequest.viewerCanLabel",
}

// missingField matches gqlparser's message for a field the schema lacks.
var missingField = regexp.MustCompile(`^Cannot query field "(\w+)" on type "(\w+)"`)

// checkGraphQL returns what is wrong with ops, GraphQL operations by name,
// against schema: what doesn't validate, but for a missing field that
// optional lists, and what they use that is deprecated, or that upcoming,
// the locations of GitHub's upcoming breaking changes such as Type.field,
// says will change.
func checkGraphQL(schema *gqlast.Schema, optional, upcoming []string, ops map[string]string) []string {
	var out []string
	for _, name := range slicesSorted(ops) {
		doc, err := gqlparse.ParseQuery(&gqlast.Source{Name: name, Input: ops[name]})
		if err != nil {
			out = append(out, name+": "+err.Error())
			continue
		}
		// Validation also ties each field to its definition, which the
		// walk below reads even of an operation that doesn't validate.
		for _, e := range validator.ValidateWithRules(schema, doc, nil) {
			if m := missingField.FindStringSubmatch(e.Message); len(m) == 3 && slices.Contains(optional, m[2]+"."+m[1]) {
				continue
			}
			out = append(out, name+": "+e.Message)
		}
		used := func(what, loc string, deprecated bool) {
			if deprecated {
				out = append(out, name+": "+what+" "+loc+" is deprecated")
			}
			if slices.Contains(upcoming, loc) {
				out = append(out, name+": "+what+" "+loc+" is to change")
			}
		}
		var walk func(gqlast.SelectionSet)
		walk = func(set gqlast.SelectionSet) {
			for _, sel := range set {
				switch s := sel.(type) {
				case *gqlast.Field:
					if d := s.Definition; d != nil && s.ObjectDefinition != nil && !strings.HasPrefix(s.Name, "__") {
						field := s.ObjectDefinition.Name + "." + s.Name
						used("field", field, d.Directives.ForName("deprecated") != nil)
						for _, a := range s.Arguments {
							def := d.Arguments.ForName(a.Name)
							if def == nil {
								continue
							}
							used("argument", field+"."+a.Name, def.Directives.ForName("deprecated") != nil)
							if a.Value != nil && a.Value.Kind == gqlast.EnumValue {
								if t := schema.Types[def.Type.Name()]; t != nil {
									if v := t.EnumValues.ForName(a.Value.Raw); v != nil {
										used("value", t.Name+"."+v.Name, v.Directives.ForName("deprecated") != nil)
									}
								}
							}
						}
					}
					walk(s.SelectionSet)
				case *gqlast.InlineFragment:
					walk(s.SelectionSet)
				case *gqlast.FragmentSpread:
					if s.Definition != nil {
						walk(s.Definition.SelectionSet)
					}
				}
			}
		}
		for _, op := range doc.Operations {
			walk(op.SelectionSet)
		}
	}
	return out
}

// upcomingChanges returns the locations of GitHub's upcoming breaking
// changes, from its graphql_upcoming_changes YAML.
func upcomingChanges(b []byte) ([]string, error) {
	var doc struct {
		Changes []struct {
			Location string `yaml:"location"`
		} `yaml:"upcoming_changes"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	out := make([]string, len(doc.Changes))
	for i, c := range doc.Changes {
		out[i] = c.Location
	}
	return out, nil
}

// checkREST returns what is wrong with calls against doc: a path, method
// or parameter it doesn't declare, a media type it doesn't name, a field
// of what a call decodes that its answer lacks, or anything of these
// that is deprecated.
func checkREST(doc *openapi3.T, calls []restCall) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		at := c.Func + ": " + c.Method + " " + c.Path
		item := doc.Paths.Find(c.Path)
		if item == nil {
			out = append(out, at+": no such path")
			continue
		}
		op := item.Operations()[c.Method]
		if op == nil {
			out = append(out, at+": no such method")
			continue
		}
		if op.Deprecated {
			out = append(out, at+": deprecated")
		}
		params := slices.Concat(item.Parameters, op.Parameters)
		for _, name := range pathParams(c.Path) {
			if p := param(params, openapi3.ParameterInPath, name); p == nil {
				out = append(out, at+": no path parameter "+name)
			}
		}
		for _, name := range c.Query {
			switch p := param(params, openapi3.ParameterInQuery, name); {
			case p == nil:
				out = append(out, at+": no query parameter "+name)
			case p.Deprecated:
				out = append(out, at+": query parameter "+name+" is deprecated")
			}
		}
		res := success(op)
		if c.Accept != "" && (res == nil || res.Content.Get(c.Accept) == nil) && !mediaNamed(op.Description, c.Accept) {
			out = append(out, at+": media type "+c.Accept+" not named")
		}
		if c.Decode == nil {
			continue
		}
		var schema *openapi3.SchemaRef
		if res != nil {
			if mt := res.Content.Get("application/json"); mt != nil {
				schema = mt.Schema
			}
		}
		if schema == nil {
			out = append(out, at+": no JSON answer")
			continue
		}
		out = append(out, checkType(at+": ", reflect.TypeOf(c.Decode), schema, "")...)
	}
	return out
}

// mediaNamed reports whether description, of an operation, names media,
// one of GitHub's own media types, as GitHub does: whole, or by its short
// name, as the `text-match` media type of application/vnd.github.text-match+json.
func mediaNamed(description, media string) bool {
	short := strings.TrimSuffix(strings.TrimPrefix(media, "application/vnd.github."), "+json")
	return strings.Contains(description, media) || strings.Contains(description, "`"+short+"`")
}

// pathParams returns the names of the parameters of a path template.
func pathParams(path string) []string {
	matches := regexp.MustCompile(`\{(\w+)\}`).FindAllStringSubmatch(path, -1)
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m[1]
	}
	return out
}

func param(params openapi3.Parameters, in, name string) *openapi3.Parameter {
	for _, p := range params {
		if p.Value != nil && p.Value.In == in && p.Value.Name == name {
			return p.Value
		}
	}
	return nil
}

// success returns the first answer of op that means success, or nil.
func success(op *openapi3.Operation) *openapi3.Response {
	for _, code := range []int{200, 201, 202, 204, 205} {
		if r := op.Responses.Status(code); r != nil && r.Value != nil {
			return r.Value
		}
	}
	return nil
}

var (
	timeType    = reflect.TypeFor[time.Time]()
	rawJSONType = reflect.TypeFor[json.RawMessage]()
)

// checkType returns the json fields of t, at path, that schema lacks or
// deprecates.
func checkType(at string, t reflect.Type, schema *openapi3.SchemaRef, path string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if schema == nil || schema.Value == nil || t == timeType || t == rawJSONType {
		return nil
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		if items := schemaItems(schema); items != nil {
			return checkType(at, t.Elem(), items, path+"[]")
		}
	case reflect.Map:
		var out []string
		if ap := schema.Value.AdditionalProperties.Schema; ap != nil {
			return checkType(at, t.Elem(), ap, path+".*")
		}
		for name, p := range properties(schema) {
			out = append(out, checkType(at, t.Elem(), p, path+"."+name)...)
		}
		return out
	case reflect.Struct:
		var out []string
		props := properties(schema)
		for f := range t.Fields() {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			switch {
			case name == "-" || !f.IsExported() && !f.Anonymous:
				continue
			case f.Anonymous && name == "":
				out = append(out, checkType(at, f.Type, schema, path)...)
				continue
			case name == "":
				name = f.Name
			}
			p, ok := props[name]
			if !ok {
				if len(props) > 0 && f.Tag.Get("schema") != "optional" {
					out = append(out, at+"no field "+strings.TrimPrefix(path+"."+name, "."))
				}
				continue
			}
			if p.Value != nil && p.Value.Deprecated {
				out = append(out, at+"field "+strings.TrimPrefix(path+"."+name, ".")+" is deprecated")
			}
			out = append(out, checkType(at, f.Type, p, path+"."+name)...)
		}
		return out
	default:
	}
	return nil
}

// properties returns the properties of schema, with those of the schemas
// it is made of: all of allOf, and any of anyOf and oneOf.
func properties(schema *openapi3.SchemaRef) map[string]*openapi3.SchemaRef {
	out := map[string]*openapi3.SchemaRef{}
	var add func(*openapi3.SchemaRef)
	add = func(s *openapi3.SchemaRef) {
		if s == nil || s.Value == nil {
			return
		}
		for name, p := range s.Value.Properties {
			if _, ok := out[name]; !ok {
				out[name] = p
			}
		}
		for _, sub := range slices.Concat(s.Value.AllOf, s.Value.AnyOf, s.Value.OneOf) {
			add(sub)
		}
	}
	add(schema)
	return out
}

// schemaItems returns the schema of the items of an array schema, or of
// one it is made of.
func schemaItems(schema *openapi3.SchemaRef) *openapi3.SchemaRef {
	if schema == nil || schema.Value == nil {
		return nil
	}
	if schema.Value.Items != nil {
		return schema.Value.Items
	}
	for _, sub := range slices.Concat(schema.Value.AllOf, schema.Value.AnyOf, schema.Value.OneOf) {
		if items := schemaItems(sub); items != nil {
			return items
		}
	}
	return nil
}

func slicesSorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// packageFiles parses the package's own files, not its tests.
func packageFiles(t *testing.T) []*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}

// TestSchemaCheckListsEveryCall holds graphqlOperations and restCalls to
// the code: every package-level query or mutation is listed, and so is
// every function that makes a REST call. It matches REST calls by the
// function that makes them, not by each call, so a new call in a listed
// function needs a new row by hand.
func TestSchemaCheckListsEveryCall(t *testing.T) {
	// The plumbing that every call goes through.
	plumbing := []string{
		"Get", "Do", "rest", "roundTrip", "probeList", "getRaw", "rawRoundTrip", "codeSearchGet",
		"resolve", "send", "sendWith", "Query", "query", "queryOnce",
	}
	sends := []string{"Get", "Do", "rest", "probeList", "getRaw", "codeSearchGet", "resolve", "send", "sendWith"}
	var ops, callers []string
	files := packageFiles(t)
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					// A query is a value; a constant of iota, such as
					// classMutation, has none of its own.
					if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Values) > 0 {
						for _, n := range vs.Names {
							if strings.HasSuffix(n.Name, "Query") || strings.HasSuffix(n.Name, "Mutation") {
								ops = append(ops, n.Name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if d.Body == nil || slices.Contains(plumbing, d.Name.Name) {
					continue
				}
				ast.Inspect(d.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name == "c" && slices.Contains(sends, sel.Sel.Name) {
							callers = append(callers, d.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
	slices.Sort(ops)
	if listed := slicesSorted(graphqlOperations); !slices.Equal(ops, listed) {
		t.Errorf("GraphQL operations in the code %v, listed %v", ops, listed)
	}
	listed := make([]string, len(restCalls))
	for i, c := range restCalls {
		listed[i] = c.Func
	}
	slices.Sort(callers)
	slices.Sort(listed)
	if callers, listed = slices.Compact(callers), slices.Compact(listed); !slices.Equal(callers, listed) {
		t.Errorf("functions that make REST calls %v, listed %v", callers, listed)
	}
	for _, p := range unlistedQueries(files, plumbing) {
		t.Error(p)
	}
}

// unlistedQueries returns the GraphQL queries sent with c.Query or c.query
// that graphqlOperations doesn't list: a query argument that is no listed
// variable, such as a literal, or a parameter that a call of its function
// passes no listed variable for.
func unlistedQueries(files []*ast.File, plumbing []string) []string {
	var out []string
	listed := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		if !ok {
			return false
		}
		_, known := graphqlOperations[id.Name]
		return known
	}
	// passed are the parameters, by function and position, that are sent
	// as queries, so each call of the function must pass a listed one.
	type param struct {
		fn string
		at int
	}
	var passed []param
	for _, f := range files {
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok || d.Body == nil || slices.Contains(plumbing, d.Name.Name) {
				continue
			}
			var params []string
			for _, field := range d.Type.Params.List {
				for _, n := range field.Names {
					params = append(params, n.Name)
				}
			}
			ast.Inspect(d.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Query" && sel.Sel.Name != "query" {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "c" || listed(call.Args[1]) {
					return true
				}
				if id, ok := call.Args[1].(*ast.Ident); ok && slices.Contains(params, id.Name) {
					passed = append(passed, param{d.Name.Name, slices.Index(params, id.Name)})
					return true
				}
				out = append(out, d.Name.Name+" sends a query graphqlOperations doesn't list")
				return true
			})
		}
	}
	for _, p := range passed {
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if name == p.fn && (len(call.Args) <= p.at || !listed(call.Args[p.at])) {
					out = append(out, fmt.Sprintf("a call of %s passes a query graphqlOperations doesn't list", p.fn))
				}
				return true
			})
		}
	}
	return out
}

func TestUnlistedQueries(t *testing.T) {
	const src = `package github
func (c *Client) Listed() { c.Query(ctx, getRepoQuery, nil, nil) }
func (c *Client) Inline() { c.Query(ctx, "query { viewer { login } }", nil, nil) }
func (c *Client) Named() { q := getRepoQuery; c.query(ctx, q, nil, nil) }
func (c *Client) send(ctx context.Context, what, query string) { c.Query(ctx, query, nil, nil) }
func (c *Client) A() { c.send(ctx, "a", pullIDQuery) }
func (c *Client) B() { c.send(ctx, "b", otherQuery) }
`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := unlistedQueries([]*ast.File{f}, nil)
	want := []string{
		"Inline sends a query graphqlOperations doesn't list",
		"Named sends a query graphqlOperations doesn't list",
		"a call of send passes a query graphqlOperations doesn't list",
	}
	if !slices.Equal(got, want) {
		t.Errorf("unlistedQueries = %q, want %q", got, want)
	}
}

// TestGraphQLOptionalSelected holds graphqlOptional to the operations:
// each field it lists is still selected by one of them.
func TestGraphQLOptionalSelected(t *testing.T) {
	for _, f := range graphqlOptional {
		_, field, _ := strings.Cut(f, ".")
		selected := false
		for _, op := range graphqlOperations {
			selected = selected || regexp.MustCompile(`\b`+field+`\b`).MatchString(op)
		}
		if !selected {
			t.Errorf("%s is listed optional but no operation selects it", f)
		}
	}
}

func TestCheckGraphQL(t *testing.T) {
	schema := gqlparser.MustLoadSchema(&gqlast.Source{Input: `
type Query { viewer: User!, repository(owner: String!, name: String!): Repository }
type User { login: String!, name: String @deprecated(reason: "gone"), repos(order: Order): [Repository!]! }
type Repository { name: String!, stars: Int! }
enum Order { NAME, AGE @deprecated(reason: "old") }
`})
	ops := map[string]string{
		"ok":         `query A { viewer { login repos(order: NAME) { name } } }`,
		"missing":    `query B { viewer { login novel } }`,
		"optional":   `query E { viewer { login fresh } }`,
		"deprecated": `query C { viewer { name repos(order: AGE) { name } } }`,
		"upcoming":   `query D($o: String!) { repository(owner: $o, name: "x") { stars } }`,
	}
	got := checkGraphQL(schema, []string{"User.fresh"}, []string{"Repository.stars"}, ops)
	want := []string{
		"deprecated: field User.name is deprecated",
		"deprecated: value Order.AGE is deprecated",
		`missing: Cannot query field "novel" on type "User". Did you mean "name"?`,
		"upcoming: field Repository.stars is to change",
	}
	if !slices.Equal(got, want) {
		t.Errorf("checkGraphQL =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestUpcomingChanges(t *testing.T) {
	got, err := upcomingChanges([]byte("---\nupcoming_changes:\n  - location: Issue.stateReason.enableDuplicate\n    date: '2025-10-01'\n  - location: A.b\n"))
	if err != nil || !slices.Equal(got, []string{"Issue.stateReason.enableDuplicate", "A.b"}) {
		t.Errorf("upcomingChanges = %v, %v", got, err)
	}
}

// tinyREST is a small OpenAPI description in GitHub's style: a repository
// made of a base and more by allOf, a deprecated field, and a media type
// named in a description.
const tinyREST = `{
  "openapi": "3.0.3",
  "info": {"title": "tiny", "version": "1"},
  "paths": {
    "/repos/{owner}/{repo}": {
      "get": {
        "parameters": [
          {"$ref": "#/components/parameters/owner"},
          {"name": "repo", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "fields", "in": "query", "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/full-repository"}}}}}
      }
    },
    "/repos/{owner}/{repo}/git/blobs/{file_sha}": {
      "get": {
        "description": "Ask for application/vnd.github.raw+json for the raw content, or the \u0060object\u0060 media type for JSON.",
        "parameters": [
          {"$ref": "#/components/parameters/owner"},
          {"name": "repo", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "file_sha", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object"}}}}}
      }
    }
  },
  "components": {
    "parameters": {"owner": {"name": "owner", "in": "path", "required": true, "schema": {"type": "string"}}},
    "schemas": {
      "repository": {"type": "object", "properties": {"name": {"type": "string"}, "owner": {"type": "object", "properties": {"login": {"type": "string"}}}}},
      "full-repository": {"allOf": [
        {"$ref": "#/components/schemas/repository"},
        {"type": "object", "properties": {
          "topics": {"type": "array", "items": {"type": "string"}},
          "master_branch": {"type": "string", "deprecated": true}
        }}
      ]}
    }
  }
}`

func TestCheckREST(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData([]byte(tinyREST))
	if err != nil {
		t.Fatal(err)
	}
	type owner struct {
		Login string `json:"login"`
		Kind  string `json:"kind"`
	}
	type repo struct {
		Name   string    `json:"name"`
		Owner  *owner    `json:"owner"`
		Topics []string  `json:"topics"`
		Master string    `json:"master_branch"`
		Pushed time.Time `json:"pushed_at"`
		Newer  string    `json:"newer" schema:"optional"`
	}
	calls := []restCall{
		{Func: "ok", Method: http.MethodGet, Path: "/repos/{owner}/{repo}", Query: []string{"fields"}, Decode: struct {
			Name string `json:"name"`
		}{}},
		{Func: "fields", Method: http.MethodGet, Path: "/repos/{owner}/{repo}", Decode: repo{}},
		{Func: "query", Method: http.MethodGet, Path: "/repos/{owner}/{repo}", Query: []string{"sort"}},
		{Func: "method", Method: http.MethodDelete, Path: "/repos/{owner}/{repo}"},
		{Func: "path", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/pulls"},
		{Func: "raw", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/git/blobs/{file_sha}", Accept: "application/vnd.github.raw+json"},
		{Func: "short", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/git/blobs/{file_sha}", Accept: "application/vnd.github.object+json"},
		{Func: "media", Method: http.MethodGet, Path: "/repos/{owner}/{repo}/git/blobs/{file_sha}", Accept: "application/vnd.github.v3.diff"},
	}
	got := checkREST(doc, calls)
	want := []string{
		"fields: GET /repos/{owner}/{repo}: no field owner.kind",
		"fields: GET /repos/{owner}/{repo}: field master_branch is deprecated",
		"fields: GET /repos/{owner}/{repo}: no field pushed_at",
		"query: GET /repos/{owner}/{repo}: no query parameter sort",
		"method: DELETE /repos/{owner}/{repo}: no such method",
		"path: GET /repos/{owner}/{repo}/pulls: no such path",
		"media: GET /repos/{owner}/{repo}/git/blobs/{file_sha}: media type application/vnd.github.v3.diff not named",
	}
	if !slices.Equal(got, want) {
		t.Errorf("checkREST =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
