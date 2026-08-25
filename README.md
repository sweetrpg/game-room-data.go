# shelf-data.go

Database access layer for the Shelf microservice: repository functions for library, wishlist,
and table, plus visibility resolution, translating between `shelf-objects.go` persistence
models and their API value objects.

## Dependencies

`api-core.go`, `shelf-objects.go`, `common.go`, `model-core.go`, `mongodb.go`. Depended on by
`shelf-api`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
