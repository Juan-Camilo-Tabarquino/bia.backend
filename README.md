# bia.backend

## Prerequisites

- **Go version:** 1.22 or newer (run `go version` to verify)

## Setup

After cloning the repository, ensure the module dependencies are up‑to‑date:

```sh
go mod tidy
```

This will resolve and download any missing packages and clean the `go.mod` and `go.sum` files.

## Build & Run

```sh
go build ./cmd/api
./cmd/api/api
```


