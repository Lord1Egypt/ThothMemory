# Contributing

Open an issue to discuss substantial changes. Small fixes can go directly to a pull
request. Use Go 1.26.9+, Docker and Compose. No model key is needed for development.

```sh
go build -o bin/thothmemory ./cmd/thothmemory
./scripts/test-integration.sh
./scripts/check.sh
./setup.sh --build
```

Integration tests create a disposable PostgreSQL container. Unit tests explicitly
skip PostgreSQL tests when THOTH_TEST_DATABASE_URL is unset; CI sets it and executes
them. Keep production changes accompanied by tests for meaningful behavior, security
boundaries and recovery. New migrations must preserve existing history. Document
measured claims and record limitations. Retain dependency notices in third_party.

Use issues for public bugs and GitHub private vulnerability reporting for security
reports. Do not include secrets, project memories, database archives or personal
configuration in contributions. Contributions are licensed under MIT.
