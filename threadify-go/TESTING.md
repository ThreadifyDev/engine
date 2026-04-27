# Testing & Mocks

## Run tests

From `threadify-go/`:

- Engine unit tests (main module): `make test-engine-unit`
- Engine integration tests (nested `tests/` module): `make test`
- Web API module (`api/`): `make test-api`
- Shared module (`shared/`): `make test-shared`
- Everything (engine + api + shared): `make test-all`

## Generate GoMock mocks

From `threadify-go/`:

- API mocks: `make mocks-api`
- Shared mocks: `make mocks-shared`
- Both: `make mocks-all`

## Shared mocks

Mocks for shared interfaces are generated into `threadify-go/shared/mocks` and can be imported from:

- `threadify-go/shared/mocks`
