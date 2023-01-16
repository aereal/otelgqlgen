[![status][ci-status-badge]][ci-status]
[![PkgGoDev][pkg-go-dev-badge]][pkg-go-dev]

# otelgqlgen

**otelgqlgen** provides [99designs/gqlgen][gqlgen]'s extension that collects [OpenTelemetry traces][otel-traces].

## Synopsis

```go
import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/aereal/otelgqlgen"
)

func main() {
	var srv *handler.Server // handler.Server initialized with your executable schema.
	srv.Use(otelgqlgen.New())
	_ = (&http.Server{Handler: srv}).ListenAndServe()

	// also you must instrument [OpenTelemetry SDK](https://opentelemetry.io/docs/instrumentation/go/) and OpenTelemetry collector properly.
}
```

## Installation

```sh
go get github.com/aereal/otelgqlgen
```

## Prior arts and comparison

[ravilushqa/otelgqlgen][otelgqlgen] is registered in [the Registry][otel-registry].

It works enough for me, but I decide to create yet another instrumentation for below reasons:

- _studying_
	- _gqlgen extension study_: I created some gqlgen extension in past, but I had not known recent extension API changes.
	- _OpenTelemetry instrumentation study_: I've used OpenTelemetry for private and work.
- _customizing_
	- _additional support_: for example, APQ stats.

## License

See LICENSE file.

[pkg-go-dev]: https://pkg.go.dev/github.com/aereal/otelgqlgen
[pkg-go-dev-badge]: https://pkg.go.dev/badge/aereal/otelgqlgen
[ci-status-badge]: https://github.com/aereal/otelgqlgen/workflows/CI/badge.svg?branch=main
[ci-status]: https://github.com/aereal/otelgqlgen/actions/workflows/CI
[gqlgen]: https://github.com/99designs/gqlgen
[otel-traces]: https://opentelemetry.io/docs/concepts/signals/traces/
[otelgqlgen]: https://github.com/ravilushqa/otelgqlgen
[otel-registry]: https://opentelemetry.io/registry/?s=gqlgen&component=&language=
