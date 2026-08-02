.PHONY: graphql-generate
graphql-generate: ## Generate public graphql schema
	go run github.com/99designs/gqlgen@latest generate --config gqlgen.yaml
