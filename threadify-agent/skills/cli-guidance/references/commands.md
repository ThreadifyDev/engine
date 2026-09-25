# Threadify CLI fallback

Download and verify a `threadify-cli` release from the official CLI repository.
It is installed separately from the `threadify` Engine binary.

```sh
threadify-cli config set api-url https://threadify.example.com
threadify-cli login
threadify-cli whoami
```

For remote terminals, use `threadify-cli login --no-browser`. For automation,
set `THREADIFY_API_KEY` in the process environment. Avoid passing keys as
command arguments.

```sh
threadify-cli contracts preview --file contract.feature
threadify-cli contracts create --file contract.feature
threadify-cli contracts list
threadify-cli contracts get --id CONTRACT_ID
threadify-cli contracts update --id CONTRACT_ID --file contract-v2.feature
threadify-cli threads list --status active
threadify-cli threads get --id THREAD_ID
threadify-cli profiles list --type Customers
threadify-cli ingestion-rules get
```

Use `threadify-cli help` or the public command guide for flags beyond these.
The CLI emits resource JSON to stdout and errors to stderr. It does not record
runtime step events; use a Threadify SDK or OTEL exporter for those.
