# Testing & Mocks

## Run tests

From `threadify-go/`:

- Engine unit tests (main module): `make test-engine-unit`
- Engine integration tests (nested `tests/` module): `make test`
- Stateless AI gateway module (`api/`): `make test-api`
- Shared module (`shared/`): `make test-shared`
- Everything (engine + api + shared): `make test-all`

The gateway uses standard-library HTTP test servers and needs no database or
external model for its tests. `make test-api-integration` runs its streaming,
authentication and TLS checks with the race detector. Engine management coverage
remains in `tests/e2e`; the retired management Web API suite has been removed.

## Generate GoMock mocks

From `threadify-go/`:

- Shared mocks: `make mocks-shared`
- Engine and shared mocks: `make mocks-all`

## Shared mocks

Mocks for shared interfaces are generated into `threadify-go/shared/mocks` and can be imported from:

- `threadify-go/shared/mocks`
