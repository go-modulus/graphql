package graphql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-modulus/graphql"
	"github.com/go-modulus/modulus/module"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

type TestObj struct {
	SomeField string
}

func TestDecorateServer(t *testing.T) {
	t.Run(
		"replaces the server produced by the container with the decorated one", func(t *testing.T) {
			original := handler.New(nil)
			decorated := handler.New(nil)

			var called bool
			var receivedDeps []any

			var srv *handler.Server

			testModule := module.NewModule("test/graphql").
				AddProviders(
					func() *handler.Server { return original },
					func() *TestObj { return &TestObj{} },
				).
				AddInvokes(
					func(s *handler.Server) { srv = s },
				)
			testModule.WithOptions(
				graphql.DecorateServer(
					func(srv *handler.Server, testObj *TestObj) *handler.Server {
						called = true
						receivedDeps = []any{testObj}
						require.Same(t, original, srv)
						return decorated
					},
				),
			)

			app := fxtest.New(
				t,
				module.BuildFx(testModule),
			)
			app.RequireStart()
			defer app.RequireStop()

			require.True(t, called, "decorator should be invoked")
			require.NotEmpty(
				t,
				receivedDeps,
				"variadic dependencies are stripped by the DI container when none are wired",
			)
			require.Same(t, decorated, srv, "the server resolved from the container should be the decorated instance")
		},
	)

	t.Run(
		"returns a module option that only registers a decorator when applied", func(t *testing.T) {
			called := false
			opt := graphql.DecorateServer(
				func(srv *handler.Server, dependencies ...any) *handler.Server {
					called = true
					return srv
				},
			)

			require.NotNil(t, opt)
			require.False(t, called, "the decorator must not run until the container resolves the server")
		},
	)
}

type initFuncCtxKey struct{}

// newInitFuncTestModule builds a bare test module with just what InitFunc
// needs to resolve: the registry it reads from and itself as a provider.
func newInitFuncTestModule() *module.Module {
	return module.NewModule("test/graphql").AddProviders(graphql.NewInitFuncRegistry, graphql.InitFunc)
}

func TestInitFunc(t *testing.T) {
	t.Run(
		"chains registered InitFuncs in rank order, threading ctx and payload", func(t *testing.T) {
			var order []string

			first := func(
				ctx context.Context, payload transport.InitPayload,
			) (context.Context, *transport.InitPayload, error) {
				order = append(order, "first")
				payload["seenBy"] = "first"
				ctx = context.WithValue(ctx, initFuncCtxKey{}, "first")
				return ctx, &payload, nil
			}
			second := func(
				ctx context.Context, payload transport.InitPayload,
			) (context.Context, *transport.InitPayload, error) {
				order = append(order, "second")
				require.Equal(
					t, "first", ctx.Value(initFuncCtxKey{}),
					"second must observe the context produced by first",
				)
				require.Equal(
					t, "first", payload["seenBy"],
					"second must observe the payload produced by first",
				)
				payload["seenBy"] = "second"
				return ctx, &payload, nil
			}

			testModule := newInitFuncTestModule()
			// Registered out of rank order on purpose: "second" is added
			// first but ranked after "first", to prove rank wins over
			// registration order.
			testModule.WithOptions(
				graphql.AddInitFunc(200, second),
				graphql.AddInitFunc(100, first),
			)

			var composed transport.WebsocketInitFunc
			app := fxtest.New(
				t,
				module.BuildFx(testModule),
				fx.Populate(&composed),
			)
			app.RequireStart()
			defer app.RequireStop()

			require.NotNil(t, composed)

			ctx, payload, err := composed(context.Background(), transport.InitPayload{})
			require.NoError(t, err)
			require.Equal(t, []string{"first", "second"}, order)
			require.Equal(t, "first", ctx.Value(initFuncCtxKey{}))
			require.NotNil(t, payload)
			require.Equal(t, "second", (*payload)["seenBy"])
		},
	)

	t.Run(
		"stops the chain and surfaces the error when an InitFunc rejects the connection", func(t *testing.T) {
			sentinel := errors.New("unauthenticated")

			var secondCalled bool
			first := func(
				ctx context.Context, payload transport.InitPayload,
			) (context.Context, *transport.InitPayload, error) {
				return ctx, &payload, sentinel
			}
			second := func(
				ctx context.Context, payload transport.InitPayload,
			) (context.Context, *transport.InitPayload, error) {
				secondCalled = true
				return ctx, &payload, nil
			}

			testModule := newInitFuncTestModule()
			testModule.WithOptions(
				graphql.AddInitFunc(100, first),
				graphql.AddInitFunc(200, second),
			)

			var composed transport.WebsocketInitFunc
			app := fxtest.New(
				t,
				module.BuildFx(testModule),
				fx.Populate(&composed),
			)
			app.RequireStart()
			defer app.RequireStop()

			_, _, err := composed(context.Background(), transport.InitPayload{})
			require.ErrorIs(t, err, sentinel)
			require.False(t, secondCalled, "the chain must stop at the first error")
		},
	)

	t.Run(
		"is a no-op pass-through when no InitFuncs are registered", func(t *testing.T) {
			testModule := newInitFuncTestModule()

			var composed transport.WebsocketInitFunc
			app := fxtest.New(
				t,
				module.BuildFx(testModule),
				fx.Populate(&composed),
			)
			app.RequireStart()
			defer app.RequireStop()

			ctx := context.Background()
			gotCtx, gotPayload, err := composed(ctx, transport.InitPayload{"a": 1})
			require.NoError(t, err)
			require.Equal(t, ctx, gotCtx)
			require.Equal(t, transport.InitPayload{"a": 1}, *gotPayload)
		},
	)
}

// authenticatorStub stands in for a real dependency (e.g. a token verifier)
// that an InitFuncFactory would need, to prove AddInitFuncFactory resolves
// it through DI rather than requiring it to already exist at call time.
type authenticatorStub struct {
	allowedToken string
}

func (a *authenticatorStub) Authenticate(token string) bool {
	return token == a.allowedToken
}

type tokenCtxKey struct{}

// authInitFuncFactory is an InitFuncFactory whose constructor depends on
// *authenticatorStub, resolved by the container - this is exactly the
// dependency-injection ability AddInitFuncFactory adds on top of AddInitFunc.
type authInitFuncFactory struct {
	authenticator *authenticatorStub
}

func newAuthInitFuncFactory(authenticator *authenticatorStub) *authInitFuncFactory {
	return &authInitFuncFactory{authenticator: authenticator}
}

func (f *authInitFuncFactory) InitFunc() transport.WebsocketInitFunc {
	return func(
		ctx context.Context, payload transport.InitPayload,
	) (context.Context, *transport.InitPayload, error) {
		token, _ := payload["token"].(string)
		if !f.authenticator.Authenticate(token) {
			return ctx, nil, errors.New("unauthenticated")
		}
		return context.WithValue(ctx, tokenCtxKey{}, token), &payload, nil
	}
}

func TestAddInitFuncFactory(t *testing.T) {
	t.Run(
		"builds the InitFunc with dependencies resolved through DI", func(t *testing.T) {
			testModule := newInitFuncTestModule().
				AddProviders(
					func() *authenticatorStub { return &authenticatorStub{allowedToken: "s3cret"} },
					newAuthInitFuncFactory,
				)
			testModule.WithOptions(
				graphql.AddInitFuncFactory[*authInitFuncFactory](100),
			)

			var composed transport.WebsocketInitFunc
			app := fxtest.New(
				t,
				module.BuildFx(testModule),
				fx.Populate(&composed),
			)
			app.RequireStart()
			defer app.RequireStop()

			ctx, payload, err := composed(context.Background(), transport.InitPayload{"token": "s3cret"})
			require.NoError(t, err)
			require.Equal(t, "s3cret", ctx.Value(tokenCtxKey{}))
			require.NotNil(t, payload)

			_, _, err = composed(context.Background(), transport.InitPayload{"token": "wrong"})
			require.EqualError(t, err, "unauthenticated")
		},
	)

	t.Run(
		"composes with plain AddInitFunc entries, ordered by rank across both", func(t *testing.T) {
			var order []string

			testModule := newInitFuncTestModule().
				AddProviders(
					func() *authenticatorStub { return &authenticatorStub{allowedToken: "s3cret"} },
					newAuthInitFuncFactory,
				)
			testModule.WithOptions(
				graphql.AddInitFuncFactory[*authInitFuncFactory](200),
				graphql.AddInitFunc(
					100, func(
						ctx context.Context, payload transport.InitPayload,
					) (context.Context, *transport.InitPayload, error) {
						order = append(order, "plain")
						return ctx, &payload, nil
					},
				),
			)

			var composed transport.WebsocketInitFunc
			app := fxtest.New(
				t,
				module.BuildFx(testModule),
				fx.Populate(&composed),
			)
			app.RequireStart()
			defer app.RequireStop()

			_, _, err := composed(context.Background(), transport.InitPayload{"token": "s3cret"})
			require.NoError(t, err)
			require.Equal(t, []string{"plain"}, order, "the rank-100 plain InitFunc must run before the rank-200 factory one")
		},
	)
}
