package graphql

import (
	"html/template"
	"net/http"
)

// graphiqlPage renders GraphiQL 5, which dropped the old UMD CDN bundle in
// favor of ESM-only distribution (github.com/graphql/graphiql/tree/main/examples/graphiql-cdn).
// The importmap below pins the exact dependency versions/integrity hashes
// from that official example, resolved through esm.sh.
var graphiqlPage = template.Must(
	template.New("graphiql").Parse(
		`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{.Title}}</title>
    <style>
      body {
        margin: 0;
      }

      #graphiql {
        height: 100dvh;
      }

      .loading {
        height: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 4rem;
      }
    </style>
    <link
      rel="stylesheet"
      href="https://esm.sh/graphiql@5.2.4/dist/style.css"
      integrity="sha384-TFpQQKp325U5sd3PddH4cS0KOB3Gz/aqdEe12Mqkkq3wm2MGcDhRX5WhWf+o8akh"
      crossorigin="anonymous"
    />
    <link
      rel="stylesheet"
      href="https://esm.sh/@graphiql/plugin-explorer@5.1.3/dist/style.css"
      integrity="sha384-vTFGj0krVqwFXLB7kq/VHR0/j2+cCT/B63rge2mULaqnib2OX7DVLUVksTlqvMab"
      crossorigin="anonymous"
    />
    <script type="importmap">
      {
        "imports": {
          "react": "https://esm.sh/react@19.2.8",
          "react/": "https://esm.sh/react@19.2.8/",
          "react-dom": "https://esm.sh/react-dom@19.2.8",
          "react-dom/": "https://esm.sh/react-dom@19.2.8/",
          "graphiql": "https://esm.sh/graphiql@5.2.4?standalone&external=react,react-dom,@graphiql/react,graphql",
          "graphiql/": "https://esm.sh/graphiql@5.2.4/",
          "@graphiql/plugin-explorer": "https://esm.sh/@graphiql/plugin-explorer@5.1.3?standalone&external=react,@graphiql/react,graphql",
          "@graphiql/react": "https://esm.sh/@graphiql/react@0.37.7?standalone&external=react,react-dom,graphql,@graphiql/toolkit,@emotion/is-prop-valid",
          "@graphiql/toolkit": "https://esm.sh/@graphiql/toolkit@0.12.1?standalone&external=graphql",
          "graphql": "https://esm.sh/graphql@17.0.2",
          "@emotion/is-prop-valid": "data:text/javascript,"
        },
        "integrity": {
          "https://esm.sh/react@19.2.8": "sha384-ZLbEMZxxxJSWKr0slZZsXGR6UNzfnEk4+qYI9PLH9QSxvWWQjs3UgkzrGuo33roA",
          "https://esm.sh/react-dom@19.2.8": "sha384-jssD0f2tUCDrNPAwM/2fFwybg7wF0K+9oVfFE0a7r0n7wb2UVP4/uYyWIZKGxG6L",
          "https://esm.sh/graphiql@5.2.4": "sha384-3feoWeu5QZYyfhHQyP8i+VBW+tYf58tTwgBb8Fsrw2QlP/YRBR2tscNGcYyRDtHC",
          "https://esm.sh/graphiql@5.2.4?standalone&external=react,react-dom,@graphiql/react,graphql": "sha384-n1sWmquV8wXH/vbn5Q8BaQAw8iAFku5zAs2fPBrht0L/OP4/qgZWL/v/WhMLFPBH",
          "https://esm.sh/@graphiql/plugin-explorer@5.1.3": "sha384-aDt72jaNBi2he5K4f47qh+xnS6Za54L6vuoNt6KtToLAJfebp23zCaAl3zXGl7dV",
          "https://esm.sh/@graphiql/react@0.37.7?standalone&external=react,react-dom,graphql,@graphiql/toolkit,@emotion/is-prop-valid": "sha384-U8awo9eG6M8scx4fjis/pNfYja4d5EtxOFYcmvDGG8K4Rt/bGB6Km1hxbQXZr9qH",
          "https://esm.sh/@graphiql/toolkit@0.12.1?standalone&external=graphql": "sha384-+cNTwZgIW33q7A4E+ZoCMqzcXdfVIc2VthQvJ0uDpRXERBWYuDKPVMzvdQU8x48o",
          "https://esm.sh/graphql@17.0.2": "sha384-IPvlcmnvWT91sKqwbg9MxPXJUuPmSdwXCp22x6hcbBkAEhlfSKkMLR3v19K9iRUp"
        }
      }
    </script>
    <script type="module">
      import React from 'react';
      import ReactDOM from 'react-dom/client';
      import { GraphiQL, HISTORY_PLUGIN } from 'graphiql';
      import { createGraphiQLFetcher } from '@graphiql/toolkit';
      import { explorerPlugin } from '@graphiql/plugin-explorer';
      import 'graphiql/setup-workers/esm.sh';

      const httpUrl = location.protocol + '//' + location.host + {{.Endpoint}};
{{- if .SubscriptionsEnabled}}
      const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const subscriptionUrl = wsProto + '//' + location.host + {{.Endpoint}};
{{- end}}

      const fetcher = createGraphiQLFetcher({
        url: httpUrl,
{{- if .SubscriptionsEnabled}}
        subscriptionUrl: subscriptionUrl,
{{- end}}
      });
      const plugins = [HISTORY_PLUGIN, explorerPlugin()];

      function App() {
        return React.createElement(GraphiQL, {
          fetcher,
          plugins,
          defaultEditorToolsVisibility: true,
        });
      }

      const container = document.getElementById('graphiql');
      const root = ReactDOM.createRoot(container);
      root.render(React.createElement(App));
    </script>
  </head>
  <body>
    <div id="graphiql">
      <div class="loading">Loading…</div>
    </div>
  </body>
</html>
`,
	),
)

type playgroundData struct {
	Title                string
	Endpoint             string
	SubscriptionsEnabled bool
}

// NewGraphiQLHandler renders GraphiQL 5 (the current major version - it
// dropped the UMD CDN bundle gqlgen's own playground.Handler relies on, so
// this ships its own ESM-based page instead of using that helper).
//
// subscriptionsEnabled should be true only when the server's subscription
// transport is graphql-ws-compatible (i.e. config.SubscriptionTransport ==
// "ws"); GraphiQL's fetcher wires subscriptions over graphql-ws, which
// doesn't speak the SSE-based subscription protocol.
func NewGraphiQLHandler(title, endpoint string, subscriptionsEnabled bool) http.HandlerFunc {
	data := playgroundData{
		Title:                title,
		Endpoint:             endpoint,
		SubscriptionsEnabled: subscriptionsEnabled,
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html; charset=UTF-8")

		if err := graphiqlPage.Execute(w, data); err != nil {
			panic(err)
		}
	}
}
