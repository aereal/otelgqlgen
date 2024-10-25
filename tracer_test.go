package otelgqlgen_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/aereal/otelgqlgen"
	"github.com/aereal/otelgqlgen/internal/test/execschema"
	"github.com/aereal/otelgqlgen/internal/test/resolvers"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	attrStacktrace = semconv.ExceptionStacktraceKey.String("stacktrace")
)

func TestTracer(t *testing.T) {
	type testCase struct {
		name    string
		params  *graphql.RawParams
		spans   tracetest.SpanStubs
		options []otelgqlgen.Option
	}
	testCases := []testCase{
		{
			name: "ok",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) @include(if: true) {name @include(if: true) isAdmin}}`,
				Variables: map[string]any{"name": "aereal"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "ok/named",
			params: &graphql.RawParams{
				Query:     `query namedOp($name: String!) {user(name: $name) @include(if: true) {name @include(if: true) isAdmin}}`,
				Variables: map[string]any{"name": "aereal"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "namedOp",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "namedOp"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "ok/name from parameter",
			params: &graphql.RawParams{
				Query:         `query namedOp($name: String!) {user(name: $name) @include(if: true) {name @include(if: true) isAdmin}}`,
				Variables:     map[string]any{"name": "aereal"},
				OperationName: "namedOp",
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "namedOp",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "namedOp"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "automated persisted query",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name isAdmin}}`,
				Variables: map[string]any{"name": "aereal"},
				Extensions: map[string]any{
					"persistedQuery": map[string]any{
						"version":    1,
						"sha256Hash": "bb1d493f173860f391c0358319c3a6b88c230c7bc5af8084c4082f45deb85437",
					},
				},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.String("graphql.operation.apq.hash", "bb1d493f173860f391c0358319c3a6b88c230c7bc5af8084c4082f45deb85437"),
						attribute.Bool("graphql.operation.apq.sent_query", true),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "error from root field",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name isAdmin}}`,
				Variables: map[string]any{"name": "forbidden"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user forbidden\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "forbidden"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user"),
								semconv.ExceptionTypeKey.String("github.com/aereal/otelgqlgen/internal/test/resolvers.ForbiddenError"),
								semconv.ExceptionMessageKey.String("forbidden"),
								attrStacktrace,
							},
						},
					},
				},
				{
					Name:     "http_handler",
					SpanKind: trace.SpanKindInternal,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user forbidden\n"},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user"),
								semconv.ExceptionTypeKey.String("github.com/aereal/otelgqlgen/internal/test/resolvers.ForbiddenError"),
								semconv.ExceptionMessageKey.String("forbidden"),
								attrStacktrace,
							},
						},
					},
				},
			},
		},
		{
			name: "error from edge fields",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name age isAdmin}}`,
				Variables: map[string]any{"name": "invalid"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					},
				},
				{
					Name:     "User/age",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "age"),
						attribute.String("graphql.resolver.alias", "age"),
						attribute.String("graphql.resolver.path", "user.age"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					},
				},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user.name invalid name\ninput: user.age invalid age\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "invalid"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 4),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.name"),
								semconv.ExceptionTypeKey.String("*errors.errorString"),
								semconv.ExceptionMessageKey.String("invalid name"),
								attrStacktrace,
							},
						},
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.age"),
								semconv.ExceptionTypeKey.String("*errors.errorString"),
								semconv.ExceptionMessageKey.String("invalid age"),
								attrStacktrace,
							},
						},
					},
				},
				{
					Name:     "http_handler",
					SpanKind: trace.SpanKindInternal,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user.name invalid name\ninput: user.age invalid age\n"},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.name"),
								semconv.ExceptionTypeKey.String("*errors.errorString"),
								semconv.ExceptionMessageKey.String("invalid name"),
								attrStacktrace,
							},
						},
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.age"),
								semconv.ExceptionTypeKey.String("*errors.errorString"),
								semconv.ExceptionMessageKey.String("invalid age"),
								attrStacktrace,
							},
						},
					},
				},
			},
		},
		{
			name: "nested input default value",
			params: &graphql.RawParams{
				Query: `{ root }`,
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/root",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "root"),
						attribute.String("graphql.resolver.alias", "root"),
						attribute.String("graphql.resolver.args.num", "<nil>"),
						attribute.Bool("graphql.resolver.args.num.default", true),
						attribute.String("graphql.resolver.args.rootInput.nested", "{}"),
						attribute.Bool("graphql.resolver.args.rootInput.default", true),
						attribute.String("graphql.resolver.path", "root"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 1),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "nested input",
			params: &graphql.RawParams{
				Query: `query { root(rootInput: {nested: {val: "root"}}) }`,
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/root",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "root"),
						attribute.String("graphql.resolver.alias", "root"),
						attribute.String("graphql.resolver.args.num", "<nil>"),
						attribute.Bool("graphql.resolver.args.num.default", true),
						attribute.String("graphql.resolver.args.rootInput.nested.val", `"root"`),
						attribute.String("graphql.resolver.path", "root"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 1),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name:    "ok/TraceStructFields(true)",
			options: []otelgqlgen.Option{otelgqlgen.TraceStructFields(true)},
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) @include(if: true) {name @include(if: true) isAdmin}}`,
				Variables: map[string]any{"name": "aereal"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Query/user",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Query"),
						attribute.String("graphql.resolver.field", "user"),
						attribute.String("graphql.resolver.alias", "user"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "user"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/isAdmin",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "isAdmin"),
						attribute.String("graphql.resolver.alias", "isAdmin"),
						attribute.String("graphql.resolver.path", "user.isAdmin"),
						attribute.Bool("graphql.resolver.is_method", false),
						attribute.Bool("graphql.resolver.is_resolver", false),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "User"),
						attribute.String("graphql.resolver.field", "name"),
						attribute.String("graphql.resolver.alias", "name"),
						attribute.String("graphql.resolver.directives.include.location", "FIELD"),
						attribute.String("graphql.resolver.directives.include.args.if", "true"),
						attribute.String("graphql.resolver.path", "user.name"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					}},
				{
					Name:     "query",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "query"),
						attribute.String("graphql.operation.type", "query"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
		{
			name: "ok/mutation",
			params: &graphql.RawParams{
				Query:     `mutation($name: String!) {registerUser(name: $name)}`,
				Variables: map[string]any{"name": "aereal"},
			},
			spans: tracetest.SpanStubs{
				{Name: "read", SpanKind: trace.SpanKindServer},
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "validation", SpanKind: trace.SpanKindServer},
				{
					Name:     "Mutation/registerUser",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.resolver.object", "Mutation"),
						attribute.String("graphql.resolver.field", "registerUser"),
						attribute.String("graphql.resolver.alias", "registerUser"),
						attribute.String("graphql.resolver.args.name", "$name"),
						attribute.String("graphql.resolver.path", "registerUser"),
						attribute.Bool("graphql.resolver.is_method", true),
						attribute.Bool("graphql.resolver.is_resolver", true),
					},
				},
				{
					Name:     "mutation",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.name", "mutation"),
						attribute.String("graphql.operation.type", "mutation"),
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 1),
					},
				},
				{Name: "http_handler", SpanKind: trace.SpanKindInternal},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if deadline, ok := t.Deadline(); ok {
				ctx, cancel = context.WithDeadline(ctx, deadline)
			}
			defer cancel()
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
			gqlsrv := handler.New(execschema.NewExecutableSchema(execschema.Config{Resolvers: &resolvers.Resolver{}}))
			gqlsrv.AddTransport(transport.POST{})
			gqlsrv.Use(extension.AutomaticPersistedQuery{Cache: noCache{}})
			options := tc.options[:]
			options = append(options, otelgqlgen.WithTracerProvider(tp))
			gqlsrv.Use(otelgqlgen.New(options...))
			gqlsrv.Use(extension.FixedComplexityLimit(1000))
			testTracer := tp.Tracer("test")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, span := testTracer.Start(r.Context(), "http_handler")
				defer span.End()
				gqlsrv.ServeHTTP(w, r.WithContext(ctx))
			}))
			defer srv.Close()
			body, err := json.Marshal(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
			if err != nil {
				t.Fatalf("http.NewRequestWithContext: %+v", err)
			}
			req.Header.Set("content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("http.Client.Do: %+v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("http.Response.Status: %d %#v %s", resp.StatusCode, resp.Header, string(respBody))
			}
			if err := tp.ForceFlush(ctx); err != nil {
				t.Fatal(err)
			}
			spans := exporter.GetSpans()
			if diff := cmpSpans(tc.spans, spans); diff != "" {
				t.Errorf("-want, +got:\n%s", diff)
			}
		})
	}
}

func TestTracer_no_operation_provided(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()

	wantSpans := tracetest.SpanStubs{
		{Name: "read", SpanKind: trace.SpanKindServer},
		{Name: "parsing", SpanKind: trace.SpanKindServer},
		{Name: "validation", SpanKind: trace.SpanKindServer},
		{
			Name:     "GraphQL Operation",
			SpanKind: trace.SpanKindServer,
			Attributes: []attribute.KeyValue{
				attribute.String("graphql.operation.name", "GraphQL Operation"),
			},
			Events: []sdktrace.Event{
				{
					Name: semconv.ExceptionEventName,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.errors.path", ""),
						semconv.ExceptionTypeKey.String("*gqlerror.Error"),
						semconv.ExceptionMessageKey.String("input: no operation provided"),
						attrStacktrace,
					},
				},
			},
			Status: sdktrace.Status{
				Code:        codes.Error,
				Description: "input: no operation provided\n",
			},
		},
	}

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	gqlsrv := handler.New(execschema.NewExecutableSchema(execschema.Config{Resolvers: &resolvers.Resolver{}}))
	gqlsrv.AddTransport(transport.POST{})
	gqlsrv.Use(otelgqlgen.New(otelgqlgen.WithTracerProvider(tp)))
	srv := httptest.NewServer(gqlsrv)
	defer srv.Close()
	params := &graphql.RawParams{}
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequestWithContext: %+v", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Client.Do: %+v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("http.Response.Status: %d %#v %s", resp.StatusCode, resp.Header, string(respBody))
	}
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if diff := cmpSpans(wantSpans, spans); diff != "" {
		t.Errorf("-want, +got:\n%s", diff)
	}
}

func cmpSpans(want, got tracetest.SpanStubs) string {
	opts := []cmp.Option{
		cmp.Transformer("attribute.KeyValue", transformKeyValue),
		cmpopts.SortSlices(func(x, y tracetest.SpanStub) bool { return x.EndTime.Before(y.EndTime) }),
		cmpopts.IgnoreFields(sdktrace.Event{}, "Time"),
		cmpopts.IgnoreFields(tracetest.SpanStub{}, "Parent", "SpanContext", "StartTime", "EndTime", "Links", "DroppedAttributes", "DroppedEvents", "DroppedLinks", "ChildSpanCount", "Resource", "InstrumentationLibrary", "InstrumentationScope"),
	}
	return cmp.Diff(want, got, opts...)

}

func transformKeyValue(kv attribute.KeyValue) map[attribute.Key]any {
	if kv.Key == semconv.ExceptionStacktraceKey {
		return map[attribute.Key]any{semconv.ExceptionStacktraceKey: "stacktrace"}
	}
	return map[attribute.Key]any{kv.Key: kv.Value.AsInterface()}
}

type noCache struct{}

var _ graphql.Cache[string] = (*noCache)(nil)

func (noCache) Get(_ context.Context, _ string) (string, bool) { return "", false }

func (noCache) Add(_ context.Context, _ string, _ string) {}
