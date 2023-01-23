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
	"github.com/aereal/otelgqlgen/internal/execschema"
	"github.com/aereal/otelgqlgen/internal/resolvers"
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
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
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
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.String("graphql.operation.apq.hash", "bb1d493f173860f391c0358319c3a6b88c230c7bc5af8084c4082f45deb85437"),
						attribute.Bool("graphql.operation.apq.sent_query", true),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
			},
		},
		{
			name: "error from root field",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name isAdmin}}`,
				Variables: map[string]any{"name": "forbidden"},
			},
			spans: tracetest.SpanStubs{
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user forbidden\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.variables.name", "forbidden"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user"),
								semconv.ExceptionTypeKey.String("*gqlerror.Error"),
								semconv.ExceptionMessageKey.String("input: user forbidden"),
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
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user.name invalid name\ninput: user.age invalid age\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.variables.name", "invalid"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 4),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.name"),
								semconv.ExceptionTypeKey.String("*gqlerror.Error"),
								semconv.ExceptionMessageKey.String("input: user.name invalid name"),
								attrStacktrace,
							},
						},
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("graphql.errors.path", "user.age"),
								semconv.ExceptionTypeKey.String("*gqlerror.Error"),
								semconv.ExceptionMessageKey.String("input: user.age invalid age"),
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
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 1),
					},
				},
			},
		},
		{
			name: "nested input",
			params: &graphql.RawParams{
				Query: `query { root(rootInput: {nested: {val: "root"}}) }`,
			},
			spans: tracetest.SpanStubs{
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 1),
					},
				},
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
				{Name: "parsing", SpanKind: trace.SpanKindServer},
				{Name: "read", SpanKind: trace.SpanKindServer},
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
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("graphql.operation.variables.name", "aereal"),
						attribute.Int("graphql.operation.complexity.limit", 1000),
						attribute.Int("graphql.operation.complexity.calculated", 3),
					},
				},
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
			gqlsrv.Use(extension.AutomaticPersistedQuery{Cache: graphql.NoCache{}})
			options := tc.options[:]
			options = append(options, otelgqlgen.WithTracerProvider(tp))
			gqlsrv.Use(otelgqlgen.New(options...))
			gqlsrv.Use(extension.FixedComplexityLimit(1000))
			srv := httptest.NewServer(gqlsrv)
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

func cmpSpans(want, got tracetest.SpanStubs) string {
	opts := []cmp.Option{
		cmp.Transformer("attribute.KeyValue", transformKeyValue),
		cmpopts.IgnoreFields(sdktrace.Event{}, "Time"),
		cmpopts.IgnoreFields(tracetest.SpanStub{}, "Parent", "SpanContext", "StartTime", "EndTime", "Links", "DroppedAttributes", "DroppedEvents", "DroppedLinks", "ChildSpanCount", "Resource", "InstrumentationLibrary"),
	}
	return cmp.Diff(want, got, opts...)

}

func transformKeyValue(kv attribute.KeyValue) map[attribute.Key]any {
	if kv.Key == semconv.ExceptionStacktraceKey {
		return map[attribute.Key]any{semconv.ExceptionStacktraceKey: "stacktrace"}
	}
	return map[attribute.Key]any{kv.Key: kv.Value.AsInterface()}
}
