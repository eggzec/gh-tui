package github

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// GraphQLError holds the errors of a GraphQL response. It unwraps to the
// core errors that match their types, so errors.Is(err, core.ErrNotFound)
// works for a query that names a missing repository: NOT_FOUND matches
// core.ErrNotFound, FORBIDDEN core.ErrForbidden, INSUFFICIENT_SCOPES a
// *core.ScopeError, which matches core.ErrUnauthorized, UNPROCESSABLE
// core.ErrConflict and RATE_LIMITED a *core.RateLimitError. A refusal to
// change a workflow file is a *core.ScopeError for the workflow scope, or
// core.ErrForbidden for a GitHub App, whatever its type.
type GraphQLError struct {
	Errors []GraphQLErrorItem
	causes []error
}

// GraphQLErrorItem is one entry of the errors list of a GraphQL response.
type GraphQLErrorItem struct {
	// Type is GitHub's error type, such as NOT_FOUND or FORBIDDEN.
	Type    string `json:"type"`
	Message string `json:"message"`
	// Path leads to the field that failed, as names and list indexes.
	Path []any `json:"path"`
	// Locations are where in the query an error of the query itself is,
	// such as a field the schema lacks, from line and column 1.
	Locations []struct {
		Line   int `json:"line"`
		Column int `json:"column"`
	} `json:"locations"`
	// Extensions say what such an error is, or are nil.
	Extensions *GraphQLErrorExtensions `json:"extensions"`
}

// GraphQLErrorExtensions say what an error of a query itself is: its code,
// such as undefinedField, and the type and field it is about.
type GraphQLErrorExtensions struct {
	Code      string `json:"code"`
	TypeName  string `json:"typeName"`
	FieldName string `json:"fieldName"`
}

func (e *GraphQLError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, item := range e.Errors {
		msgs[i] = item.Message
		if item.Type != "" {
			msgs[i] = item.Type + ": " + item.Message
		}
	}
	return "github: " + strings.Join(msgs, "; ")
}

// Reason returns GitHub's messages on one line, safe to show in a
// terminal. It isn't cut to any length; whatever shows it truncates it to
// fit.
func (e *GraphQLError) Reason() string {
	msgs := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		if m := oneLine(item.Message); m != "" {
			msgs = append(msgs, m)
		}
	}
	return strings.Join(msgs, "; ")
}

// Unwrap returns the core errors that match the error types.
func (e *GraphQLError) Unwrap() []error {
	return e.causes
}

// Query runs a GraphQL query or mutation and decodes its data into v. If
// GitHub returns errors along with data, v holds the partial data and the
// error is a *GraphQLError. A query that fails for a moment is sent again,
// and a mutation only if it never reached GitHub.
func (c *Client) Query(ctx context.Context, query string, vars map[string]any, v any) error {
	if err := c.query(ctx, query, vars, v); err != nil {
		return fmt.Errorf("graphql: %w", err)
	}
	return nil
}

// query is Query. On an Enterprise Server, it sends query without the
// fields that GitHub said its schema lacks, and when GitHub says so of
// more of them, it sends it once more without those too: GitHub refused
// the whole query before running any of it.
func (c *Client) query(ctx context.Context, query string, vars map[string]any, v any) error {
	sent := c.unsupported.rewrite(query)
	err := c.queryOnce(ctx, sent, vars, v)
	// github.com lacks no field the client selects, so one it says it
	// lacks is a bug to show, not to cut.
	if err == nil || !c.enterprise.Load() {
		return err
	}
	fewer, fields, ok := withoutMissing(sent, err)
	if !ok {
		return err
	}
	slog.WarnContext(ctx, "graphql fields unsupported", "span", "http", "op", operation(query), "fields", fields)
	c.unsupported.remember(query, fewer)
	return c.queryOnce(ctx, fewer, vars, v)
}

func (c *Client) queryOnce(ctx context.Context, query string, vars map[string]any, v any) error {
	cl := &call{op: operation(query), query: readOnly(query)}
	cl.shape = queryShape(cl.op, query)
	if owner, ok := vars["owner"].(string); ok {
		if name, ok := vars["name"].(string); ok {
			cl.repo = owner + "/" + name
		}
	}
	ctx = withCall(ctx, cl)
	b, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables,omitempty"`
	}{query, vars})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.send(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return c.httpError(resp)
	}

	var body struct {
		Data   json.RawMessage    `json:"data"`
		Errors []GraphQLErrorItem `json:"errors"`
	}
	if err := decode(ctx, resp.Body, &body); err != nil {
		return err
	}
	cl.rate = queryRate(body.Data)
	if cl.rate != nil && cl.query {
		c.budget.learnCost(cl.shape, cl.rate.Cost)
	}
	if v != nil && len(body.Data) > 0 && !bytes.Equal(body.Data, []byte("null")) {
		if err := json.Unmarshal(body.Data, v); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	if !slices.ContainsFunc(body.Errors, func(e GraphQLErrorItem) bool { return e.Type == "RATE_LIMITED" }) {
		c.budget.succeeded()
	}
	if len(body.Errors) > 0 {
		partial := len(body.Data) > 0 && !bytes.Equal(body.Data, []byte("null"))
		return c.graphqlError(ctx, resp.Header, cl.shape, body.Errors, partial)
	}
	return nil
}

// rateLimitField asks a query what it cost and what is left of the GraphQL
// quota, which the log records. It costs nothing itself.
const rateLimitField = "rateLimit { cost limit remaining used resetAt }"

// graphqlRate is the rateLimit field of a query.
type graphqlRate struct {
	Cost      int       `json:"cost"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Used      int       `json:"used"`
	ResetAt   time.Time `json:"resetAt"`
}

func (r *graphqlRate) obs() obs.Rate {
	return obs.Rate{Resource: "graphql", Limit: r.Limit, Remaining: r.Remaining, Used: r.Used, Reset: r.ResetAt}
}

// queryRate returns the rateLimit field of the data of a query, or nil if
// it has none, as mutations don't.
func queryRate(data json.RawMessage) *graphqlRate {
	if !bytes.Contains(data, []byte(`"rateLimit"`)) {
		return nil
	}
	var d struct {
		RateLimit *graphqlRate `json:"rateLimit"`
	}
	if json.Unmarshal(data, &d) != nil {
		return nil
	}
	return d.RateLimit
}

// operation returns the name of the operation of query, such as GetPull in
// "query GetPull($owner: String!) {…}", or its type if it has no name.
func operation(query string) string {
	q := strings.TrimSpace(query)
	for _, typ := range []string{"query", "mutation"} {
		rest, ok := strings.CutPrefix(q, typ)
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " \t\r\n")
		end := strings.IndexFunc(rest, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
		})
		if end < 0 {
			end = len(rest)
		}
		return cmp.Or(rest[:end], typ)
	}
	// A query may leave out its type.
	return "query"
}

// queryShape returns what names the cost of query, whose operation is op:
// op, unless the query has no name, and then its text, hashed.
func queryShape(op, query string) string {
	if op != "query" {
		return op
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(query)))
	return "query " + strconv.FormatUint(h.Sum64(), 16)
}

// readOnly reports whether the GraphQL document query is a query, which
// only reads: its operation type is query, or it is the shorthand {…}.
// Whatever else it is, such as a mutation, counts as a write, so that it
// is never sent twice.
func readOnly(query string) bool {
	q := strings.TrimLeft(query, " \t\r\n")
	if strings.HasPrefix(q, "{") {
		return true
	}
	rest, ok := strings.CutPrefix(q, "query")
	if !ok {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// graphqlError builds the error of a response with errors to a query of
// shape (call.shape). partial says that the response has data too, as a
// search across organizations does when some of them keep the token out
// of their results. Then a FORBIDDEN or INSUFFICIENT_SCOPES error on a
// field below the root, such as one node of a search, refuses that node
// only, not the query, so it isn't tagged as a refusal, which would drop
// what was kept of the query.
func (c *Client) graphqlError(ctx context.Context, h http.Header, shape string, items []GraphQLErrorItem, partial bool) *GraphQLError {
	e := &GraphQLError{Errors: items}
	var below []string
	for _, item := range items {
		if err := workflowRefusal(item.Message); err != nil {
			e.causes = append(e.causes, err)
			continue
		}
		switch item.Type {
		case "NOT_FOUND":
			e.causes = append(e.causes, core.ErrNotFound)
		case "FORBIDDEN", "INSUFFICIENT_SCOPES":
			if partial && len(item.Path) > 1 {
				below = append(below, item.Type+" "+graphqlPath(item.Path))
				break
			}
			if item.Type == "FORBIDDEN" {
				e.causes = append(e.causes, core.ErrForbidden)
			} else {
				e.causes = append(e.causes, &core.ScopeError{Scopes: scopesRequired(item.Message)})
			}
		case "UNPROCESSABLE":
			e.causes = append(e.causes, core.ErrConflict)
		case "RATE_LIMITED":
			e.causes = append(e.causes, &core.RateLimitError{Reset: c.graphqlReset(ctx, h, shape, item.Message)})
		default:
			// An Enterprise Server may lack what github.com has, which
			// no query of github.com lacks.
			if item.schemaError() && c.enterprise.Load() {
				e.causes = append(e.causes, core.ErrUnsupported)
			}
		}
	}
	if len(below) > 0 {
		slog.WarnContext(ctx, "graphql partial refusal", "span", "http", "errors", below)
	}
	return e
}

// graphqlPath writes the path of an error as GitHub gives it, such as
// search.nodes.3.
func graphqlPath(path []any) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = fmt.Sprint(p)
	}
	return strings.Join(parts, ".")
}

// graphqlReset is when a query of shape (call.shape) that GitHub refused
// as RATE_LIMITED with msg may be sent again, from the headers h of the
// response. The GraphQL quota is spent when nothing is left of it, when
// msg is GitHub's for a spent quota, or, unless msg is GitHub's for a
// secondary limit, when the query costs more than is left, and then the
// query may be sent at the release of the quota. Otherwise the limit is a
// secondary one, which lasts as Retry-After says, or a minute, and holds
// every request until then.
func (c *Client) graphqlReset(ctx context.Context, h http.Header, shape, msg string) time.Time {
	rl, ok := parseRateLimit(h)
	if ok && (rl.Remaining == 0 || quotaSpent(msg) ||
		!secondaryMessage(msg) && c.budget.costsMore(shape, rl.Remaining)) {
		return c.limitedUntil(resourceGraphQL, rl)
	}
	at, said := c.budget.retryAfter(h)
	return c.budget.limitSecondary(ctx, at, said)
}

// quotaSpent reports whether msg is GitHub's for a spent quota, such as
// "API rate limit exceeded for user ID 1.", rather than for a secondary
// limit.
func quotaSpent(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "api rate limit exceeded")
}

// secondaryMessage reports whether msg is GitHub's for a secondary limit,
// such as "You have exceeded a secondary rate limit.", which holds even
// if the query costs more than is left.
func secondaryMessage(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "secondary rate limit")
}
