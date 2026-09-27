package github

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
// core.ErrNotFound, FORBIDDEN core.ErrForbidden, INSUFFICIENT_SCOPES
// core.ErrUnauthorized, UNPROCESSABLE core.ErrConflict and RATE_LIMITED a
// *core.RateLimitError.
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

func (c *Client) query(ctx context.Context, query string, vars map[string]any, v any) error {
	cl := &call{op: operation(query), query: readOnly(query)}
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
	if v != nil && len(body.Data) > 0 && !bytes.Equal(body.Data, []byte("null")) {
		if err := json.Unmarshal(body.Data, v); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	if len(body.Errors) > 0 {
		partial := len(body.Data) > 0 && !bytes.Equal(body.Data, []byte("null"))
		return c.graphqlError(ctx, resp.Header, body.Errors, partial)
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

// graphqlError builds the error of a response with errors. partial says
// that the response has data too, as a search across organizations does
// when some of them keep the token out of their results. Then a FORBIDDEN
// or INSUFFICIENT_SCOPES error on a field below the root, such as one
// node of a search, refuses that node only, not the query, so it isn't
// tagged as a refusal, which would drop what was kept of the query.
func (c *Client) graphqlError(ctx context.Context, h http.Header, items []GraphQLErrorItem, partial bool) *GraphQLError {
	e := &GraphQLError{Errors: items}
	var below []string
	for _, item := range items {
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
				e.causes = append(e.causes, core.ErrUnauthorized)
			}
		case "UNPROCESSABLE":
			e.causes = append(e.causes, core.ErrConflict)
		case "RATE_LIMITED":
			e.causes = append(e.causes, &core.RateLimitError{Reset: c.graphqlReset(h)})
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

// graphqlReset is when the GraphQL quota refills, from the rate-limit
// headers GitHub sends with every GraphQL response.
func (c *Client) graphqlReset(h http.Header) time.Time {
	if rl, ok := parseRateLimit(h); ok {
		return rl.Reset
	}
	return c.now().Add(secondaryBackoff)
}
