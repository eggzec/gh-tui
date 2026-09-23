package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// GraphQLError holds the errors of a GraphQL response. It unwraps to the
// core errors that match their types, so errors.Is(err, core.ErrNotFound)
// works for a query that names a missing repository.
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

// Unwrap returns the core errors that match the error types.
func (e *GraphQLError) Unwrap() []error {
	return e.causes
}

// Query runs a GraphQL query or mutation and decodes its data into v. If
// GitHub returns errors along with data, v holds the partial data and the
// error is a *GraphQLError.
func (c *Client) Query(ctx context.Context, query string, vars map[string]any, v any) error {
	if err := c.query(ctx, query, vars, v); err != nil {
		return fmt.Errorf("graphql: %w", err)
	}
	return nil
}

func (c *Client) query(ctx context.Context, query string, vars map[string]any, v any) error {
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
	if err := decode(resp.Body, &body); err != nil {
		return err
	}
	if v != nil && len(body.Data) > 0 && !bytes.Equal(body.Data, []byte("null")) {
		if err := json.Unmarshal(body.Data, v); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	if len(body.Errors) > 0 {
		return c.graphqlError(resp.Header, body.Errors)
	}
	return nil
}

func (c *Client) graphqlError(h http.Header, items []GraphQLErrorItem) *GraphQLError {
	e := &GraphQLError{Errors: items}
	for _, item := range items {
		switch item.Type {
		case "NOT_FOUND":
			e.causes = append(e.causes, core.ErrNotFound)
		case "RATE_LIMITED":
			e.causes = append(e.causes, &core.RateLimitError{Reset: c.graphqlReset(h)})
		}
	}
	return e
}

// graphqlReset is when the GraphQL quota refills, from the rate-limit
// headers GitHub sends with every GraphQL response.
func (c *Client) graphqlReset(h http.Header) time.Time {
	if rl, ok := parseRateLimit(h); ok {
		return rl.Reset
	}
	return c.now().Add(secondaryBackoff)
}
