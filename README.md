# game-room-data.go

Database access layer for the Game Room microservice: repository functions for library, wishlist,
and table, plus visibility resolution, translating between `game-room-objects.go` persistence
models and their API value objects.

## Dependencies

`api-core.go`, `game-room-objects.go`, `common.go`, `model-core.go`, `mongodb.go`. Depended on by
`game-room-api`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
