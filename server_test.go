package graphql_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	gqlgraphql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-modulus/graphql"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// newWebsocketUpgradeRequest builds the minimal set of headers coder/websocket
// requires to consider a request a valid WebSocket handshake, so tests can
// focus on just the Origin check.
func newWebsocketUpgradeRequest(url, origin string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	key := make([]byte, 16)
	_, _ = rand.Read(key)

	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString(key))
	req.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req, nil
}

func TestNewGraphqlServer_SubscriptionOriginPatterns(t *testing.T) {
	newServer := func(originPatterns []string) http.Handler {
		return graphql.NewGraphqlServer(
			graphql.ServerParams{
				Config: graphql.Config{
					SubscriptionOriginPatterns: originPatterns,
				},
				Schema: nil,
				Logger: slog.Default(),
				ErrorPresenter: func(ctx context.Context, err error) *gqlerror.Error {
					return gqlgraphql.DefaultErrorPresenter(ctx, err)
				},
				InitFunc: func(
					ctx context.Context, payload transport.InitPayload,
				) (context.Context, *transport.InitPayload, error) {
					return ctx, &payload, nil
				},
			},
		)
	}

	t.Run(
		"rejects a cross-origin handshake when it doesn't match any configured pattern", func(t *testing.T) {
			server := httptest.NewServer(newServer([]string{"allowed.example.com"}))
			defer server.Close()

			req, err := newWebsocketUpgradeRequest(server.URL, "https://evil.example.com")
			require.NoError(t, err)

			resp, err := http.DefaultTransport.RoundTrip(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusForbidden, resp.StatusCode)
		},
	)

	t.Run(
		"accepts a cross-origin handshake that matches a configured pattern", func(t *testing.T) {
			server := httptest.NewServer(newServer([]string{"allowed.example.com"}))
			defer server.Close()

			req, err := newWebsocketUpgradeRequest(server.URL, "https://allowed.example.com")
			require.NoError(t, err)

			resp, err := http.DefaultTransport.RoundTrip(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		},
	)

	t.Run(
		"rejects any cross-origin handshake when no patterns are configured", func(t *testing.T) {
			server := httptest.NewServer(newServer(nil))
			defer server.Close()

			req, err := newWebsocketUpgradeRequest(server.URL, "https://not-configured.example.com")
			require.NoError(t, err)

			resp, err := http.DefaultTransport.RoundTrip(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusForbidden, resp.StatusCode)
		},
	)

	t.Run(
		"always allows a same-origin handshake regardless of configured patterns", func(t *testing.T) {
			server := httptest.NewServer(newServer(nil))
			defer server.Close()

			req, err := newWebsocketUpgradeRequest(server.URL, server.URL)
			require.NoError(t, err)

			resp, err := http.DefaultTransport.RoundTrip(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		},
	)
}
