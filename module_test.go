package graphql_test

import (
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/go-modulus/graphql"
	"github.com/go-modulus/modulus/module"
	"github.com/stretchr/testify/require"
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
