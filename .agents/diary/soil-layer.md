# Soil layer plan

## Skipped refactor

`GISSourceConfig.BaseURL` is the full layer 215 query URL. A MapServer root plus layer ids would fit a second layer better. Left it. The parcel tests lock the full URL, and a third layer is the point where that split earns a change.

## Unrelated code smell

`internal/connector/shared/httpclient/http_client.go` still says the rate limiter should move into the shared package. The file is already there.

`Makefile` `build` compiles `./cmd/main.go`. `run` compiles `./main.go`.
