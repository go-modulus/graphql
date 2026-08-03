package graphql

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	coderws "github.com/coder/websocket"
	"github.com/go-modulus/modulus/http/errhttp"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/fx"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	infraErrors "github.com/go-modulus/modulus/errors"
	"github.com/ravilushqa/otelgqlgen"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type PlaygroundConfig struct {
	Enabled bool   `env:"GQL_PLAYGROUND_ENABLED, default=true"`
	Path    string `env:"GQL_PLAYGROUND_URL, default=/playground"`
}

type Config struct {
	ComplexityLimit            int           `env:"GQL_COMPLEXITY_LIMIT, default=200"`
	Path                       string        `env:"GQL_API_URL, default=/graphql"`
	IntrospectionEnabled       bool          `env:"GQL_INTROSPECTION_ENABLED, default=true"`
	TracingEnabled             bool          `env:"GQL_TRACING_ENABLED, default=false"`
	ReturnCause                bool          `env:"GQL_RETURN_CAUSE, default=false"`
	SubscriptionTransport      string        `env:"GQL_SUBSCRIPTION_TRANSPORT, default=ws" comment:"Transport for GraphQL subscriptions. Allowed values: ws, sse"`
	SubscriptionPingInterval   time.Duration `env:"GQL_SUBSCRIPTION_PING_INTERVAL, default=10s" comment:"Keepalive ping interval connection"`
	SubscriptionOriginPatterns []string      `env:"GQL_SUBSCRIPTION_ORIGIN_PATTERNS, default=" comment:"Comma-separated list of allowed origin host patterns for WebSocket subscription connections (see coder/websocket AcceptOptions.OriginPatterns). The request host is always authorized. Leave empty to only allow same-origin connections."`
	Playground                 PlaygroundConfig
}

type ErrorPresenterParams struct {
	fx.In

	ErrorPipeline *errhttp.ErrorPipeline `optional:"true"`
	Config        Config
}

type ServerParams struct {
	fx.In

	Config             Config
	Schema             graphql.ExecutableSchema
	LoadersInitializer *LoadersInitializer `optional:"true"`
	Logger             *slog.Logger
	ErrorPresenter     graphql.ErrorPresenterFunc
	InitFunc           transport.WebsocketInitFunc
}

// InitFuncFactory lets a WebSocket InitFunc be built with its own
// dependencies resolved through DI (see AddInitFuncFactory), the same way
// http.MiddlewareFactory lets an HTTP middleware be built with dependencies
// for http.AddMiddlewareFactoryToPipeline.
type InitFuncFactory interface {
	InitFunc() transport.WebsocketInitFunc
}

// InitFuncRegistry accumulates transport.WebsocketInitFunc entries added via
// AddInitFunc/AddInitFuncFactory, ranked the same way http.Pipeline ranks
// HTTP middlewares: lower ranks run first, same-rank entries run in the
// order they were added. It's a shared mutable singleton (like http.Pipeline)
// populated by fx.Invoke calls before InitFunc ever reads from it.
type InitFuncRegistry struct {
	mu      sync.Mutex
	entries map[int][]transport.WebsocketInitFunc
	cache   []transport.WebsocketInitFunc
}

func NewInitFuncRegistry() *InitFuncRegistry {
	return &InitFuncRegistry{}
}

// Add appends fn at the given rank.
func (r *InitFuncRegistry) Add(rank int, fn transport.WebsocketInitFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries == nil {
		r.entries = make(map[int][]transport.WebsocketInitFunc)
	}
	r.entries[rank] = append(r.entries[rank], fn)
	r.cache = nil
}

// List returns the flat, rank-sorted slice of registered InitFuncs.
func (r *InitFuncRegistry) List() []transport.WebsocketInitFunc {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) == 0 {
		return nil
	}
	if len(r.cache) > 0 {
		return r.cache
	}

	ranks := make([]int, 0, len(r.entries))
	for rank := range r.entries {
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)

	result := make([]transport.WebsocketInitFunc, 0, len(r.entries))
	for _, rank := range ranks {
		result = append(result, r.entries[rank]...)
	}
	r.cache = result
	return result
}

// InitFunc composes all registered transport.WebsocketInitFunc (see
// AddInitFunc/AddInitFuncFactory) into a single one, used as the Websocket
// transport's InitFunc. Registered functions run in rank order, acting like
// a middleware chain: each one receives the context and InitPayload produced
// by the previous one, and can reject the connection by returning an error,
// or thread values through the context for the next InitFunc and for the
// resolvers. The registry is read on every call rather than once here,
// since fx.Invoke registrations that populate it (see AddInitFunc) must all
// have already run by the time a real connection triggers this - reading it
// once at provide-time could race a registration that hasn't happened yet.
func InitFunc(registry *InitFuncRegistry) transport.WebsocketInitFunc {
	return func(ctx context.Context, initPayload transport.InitPayload) (
		context.Context,
		*transport.InitPayload,
		error,
	) {
		payload := initPayload
		for _, next := range registry.List() {
			nextCtx, nextPayload, err := next(ctx, payload)
			if err != nil {
				return nextCtx, nil, err
			}
			ctx = nextCtx
			if nextPayload != nil {
				payload = *nextPayload
			}
		}
		return ctx, &payload, nil
	}
}

func NewGraphqlServer(
	params ServerParams,
) *handler.Server {
	var mb int64 = 1 << 20

	config := params.Config
	srv := handler.New(params.Schema)

	if config.SubscriptionTransport == "sse" {
		srv.AddTransport(
			transport.SSE{
				KeepAlivePingInterval: config.SubscriptionPingInterval,
			},
		)
	} else {
		srv.AddTransport(
			transport.Websocket{
				KeepAlivePingInterval: config.SubscriptionPingInterval,
				InitFunc:              params.InitFunc,
				Implementation: transport.CoderWebsocketImplementation{
					AcceptOptions: coderws.AcceptOptions{
						OriginPatterns: config.SubscriptionOriginPatterns,
					},
				},
			},
		)
	}
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(
		transport.MultipartForm{
			MaxUploadSize: mb * 5,
			MaxMemory:     mb * 5,
		},
	)
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](1000)})
	if config.IntrospectionEnabled {
		srv.Use(extension.Introspection{})
	} else {
		srv.SetDisableSuggestion(true)
	}

	srv.Use(extension.FixedComplexityLimit(config.ComplexityLimit))
	if params.LoadersInitializer != nil {
		srv.Use(params.LoadersInitializer)
	}
	srv.Use(otelgqlgen.Middleware())

	if config.TracingEnabled {
		srv.Use(apollotracing.Tracer{})
	}

	srv.SetRecoverFunc(
		func(ctx context.Context, p any) error {
			return fmt.Errorf("panic: %v", p)
		},
	)

	srv.SetErrorPresenter(
		params.ErrorPresenter,
	)

	return srv
}

type ErrorPresenterFactory interface {
	NewErrorPresenter() graphql.ErrorPresenterFunc
}

func NewErrorPresenter(params ErrorPresenterParams) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		var gqlErr *gqlerror.Error
		path := graphql.GetPath(ctx)
		if errors.As(err, &gqlErr) {
			if gqlErr.Path == nil {
				gqlErr.Path = path
			} else {
				path = gqlErr.Path
			}

			originalErr := gqlErr.Unwrap()
			if originalErr == nil {
				return gqlErr
			}

			err = originalErr
		}

		config := params.Config

		if params.ErrorPipeline != nil {
			err = params.ErrorPipeline.Process(ctx, err)
		}

		code := err.Error()
		message := infraErrors.Hint(err)
		if message == "" {
			message = code
		}

		extra := make(map[string]any)

		meta := infraErrors.Meta(err)
		if meta != nil {
			extra["meta"] = infraErrors.Meta(err)
		}

		if config.ReturnCause {
			cause := infraErrors.Cause(err)
			if cause != nil {
				causeMap := map[string]interface{}{
					"code": cause.Error(),
				}
				hint := infraErrors.Hint(cause)
				if hint != "" {
					causeMap["message"] = hint
				}
				metaCause := infraErrors.Meta(cause)
				if metaCause != nil {
					causeMap["meta"] = infraErrors.Meta(cause)
				}

				extra["cause"] = causeMap
			}
		}

		extra["code"] = code

		return &gqlerror.Error{
			Message:    message,
			Path:       path,
			Extensions: extra,
		}
	}
}
