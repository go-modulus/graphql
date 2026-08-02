package graphql

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	mHttp "github.com/go-modulus/modulus/http"
	"github.com/go-modulus/modulus/logger"
	"github.com/go-modulus/modulus/module"
)

func NewModule(options ...module.Option) *module.Module {
	return module.NewModule("modulus/graphql").
		AddDependencies(
			mHttp.NewModule(),
			logger.NewModule(),
		).
		AddProviders(
			NewGraphqlServer,
			NewLoadersInitializer,
			NewHandlerRoute,
			NewPlaygroundHandlerRoute,
			NewInitFuncRegistry,
			InitFunc,
		).
		SetOverriddenProvider("modulus/graphql.ErrorPresenter", NewErrorPresenter).
		InitConfig(Config{}).
		WithOptions(options...)
}

// OverrideErrorPresenter overrides the error presenter provider.
func OverrideErrorPresenter[T ErrorPresenterFactory](gqlModule *module.Module) *module.Module {
	return gqlModule.SetOverriddenProvider(
		"modulus/graphql.ErrorPresenter", func(factory T) graphql.ErrorPresenterFunc {
			return factory.NewErrorPresenter()
		},
	)
}

// DecorateServer - decorates GraphQL server with additional options.
// decorator is a function that gets server with additional dependencies and returns a server
// Example:
// DecorateServer(func(srv *handler.Server) *handler.Server)
func DecorateServer(decorator any) module.Option {
	return func(m *module.Module) *module.Module {
		m.Decorate(decorator)
		return m
	}
}

func DecorateWithDependency[T any](decorator func(srv *handler.Server, dep T) *handler.Server) module.Option {
	return func(m *module.Module) *module.Module {
		m.Decorate(decorator)
		return m
	}
}

// AddInitFunc registers a transport.WebsocketInitFunc that runs whenever a
// WebSocket subscription connection is initialized (i.e. on the client's
// "connection_init" message). InitFuncs are ranked the same way
// http.AddMiddlewareToPipeline ranks HTTP middlewares: lower ranks run
// first, same-rank entries run in registration order. Each one acts like a
// middleware, receiving the context and InitPayload produced by the previous
// one (see InitFunc), and can reject the connection by returning an error or
// thread values through the context for the next InitFunc and the resolvers.
//
// If your InitFunc needs its own dependencies (e.g. an authenticator
// resolved through DI), use AddInitFuncFactory instead.
func AddInitFunc(rank int, initFunc transport.WebsocketInitFunc) module.Option {
	return func(m *module.Module) *module.Module {
		return m.AddInvokes(
			func(registry *InitFuncRegistry) {
				registry.Add(rank, initFunc)
			},
		)
	}
}

// AddInitFuncFactory registers an InitFuncFactory whose own dependencies are
// resolved through DI: T is constructed by the container (so its
// constructor can request whatever it needs, e.g. an authenticator or a
// repository), and the transport.WebsocketInitFunc it returns from
// InitFunc() is added to the registry at rank, same as AddInitFunc. This
// mirrors how http.AddMiddlewareFactoryToPipeline builds an HTTP middleware
// with dependencies via http.MiddlewareFactory.
//
// The concrete type T must be registered as a provider elsewhere (typically
// in the consuming module's own AddProviders call), the same way a
// MiddlewareFactory implementation is.
func AddInitFuncFactory[T InitFuncFactory](rank int) module.Option {
	return func(m *module.Module) *module.Module {
		return m.AddInvokes(
			func(registry *InitFuncRegistry, factory T) {
				registry.Add(rank, factory.InitFunc())
			},
		)
	}
}

// NewManifestModule creates a new graphql module with the manifest module.
func NewManifesto() module.Manifesto {
	graphqlModule := module.NewManifesto(
		NewModule(),
		"github.com/go-modulus/graphql",
		"Graphql server and generator. It is based on the gqlgen library. It also provides a playground for the graphql server. You need to install the `chi http` module to use this module.",
		"1.0.0",
	)
	graphqlModule.Install.AppendFiles(
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/schema.graphql",
			DestFile:  "internal/graphql/schema.graphql",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/gqlgen.yaml",
			DestFile:  "gqlgen.yaml",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/time.go",
			DestFile:  "internal/graphql/types/time.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/time.graphql",
			DestFile:  "internal/graphql/types/time.graphql",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/uuid.go",
			DestFile:  "internal/graphql/types/uuid.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/uuid.graphql",
			DestFile:  "internal/graphql/types/uuid.graphql",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/void.go",
			DestFile:  "internal/graphql/types/void.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/types/void.graphql",
			DestFile:  "internal/graphql/types/void.graphql",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/gqlgen.mk",
			DestFile:  "mk/gqlgen.mk",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/module.go.tmpl",
			DestFile:  "internal/graphql/module.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/model/tools.go",
			DestFile:  "internal/graphql/model/tools.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/resolver/resolver.go.tmpl",
			DestFile:  "internal/graphql/resolver/resolver.go",
		},
		module.InstalledFile{
			SourceUrl: "https://raw.githubusercontent.com/go-modulus/graphql/refs/heads/main/install/resolver/schema.resolvers.go.tmpl",
			DestFile:  "internal/graphql/resolver/schema.resolvers.go",
		},
	).AppendPostInstallCommands(
		module.PostInstallCommand{
			CmdPackage: "github.com/99designs/gqlgen@latest",
			Params:     []string{"generate", "--config", "gqlgen.yaml"},
		},
	)

	graphqlModule.LocalPath = "internal/graphql"

	return graphqlModule
}
