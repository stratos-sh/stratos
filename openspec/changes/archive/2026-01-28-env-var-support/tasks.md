## 1. Dependencies

- [x] 1.1 Add godotenv dependency: `go get github.com/joho/godotenv`

## 2. Core Implementation

- [x] 2.1 Create config package with environment variable loading logic in `internal/config/config.go`
- [x] 2.2 Define Config struct with all flag fields and environment variable mapping
- [x] 2.3 Implement LoadConfig function that: loads .env file via godotenv, reads env vars with STRATOS_ prefix, supports legacy non-prefixed vars as fallbacks
- [x] 2.4 Refactor `cmd/stratos/main.go` to use the new config package while maintaining CLI flag precedence

## 3. Testing

- [x] 3.1 Add unit tests for config loading in `internal/config/config_test.go`
- [x] 3.2 Test environment variable parsing for all flag types (string, bool, duration)
- [x] 3.3 Test precedence: CLI flags > STRATOS_ env vars > legacy env vars > defaults
- [x] 3.4 Test .env file loading and missing file handling

## 4. Documentation

- [x] 4.1 Update CLAUDE.md with environment variable reference table
- [x] 4.2 Add sample `.env.example` file with all supported variables
- [x] 4.3 Update Helm chart values.yaml with comments documenting env var equivalents
- [x] 4.4 Update Helm chart deployment template to show env var configuration options
- [x] 4.5 Run docs-generator agent to update docs/ (cli.md, configuration.md)
