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
		name   string
		params *graphql.RawParams
		spans  tracetest.SpanStubs
	}
	testCases := []testCase{
		{
			name: "ok",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) @include(if: true) {name @include(if: true)}}`,
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "user"),
						attribute.String("gql.resolver.alias", "user"),
						attribute.String("gql.resolver.directives.include.location", "FIELD"),
						attribute.String("gql.resolver.directives.include.args.if", "true"),
						attribute.String("gql.resolver.args.name", "$name"),
						attribute.String("gql.resolver.path", "user"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.resolver.object", "User"),
						attribute.String("gql.resolver.field", "name"),
						attribute.String("gql.resolver.alias", "name"),
						attribute.String("gql.resolver.directives.include.location", "FIELD"),
						attribute.String("gql.resolver.directives.include.args.if", "true"),
						attribute.String("gql.resolver.path", "user.name"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.request.variables.name", "aereal"),
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 2),
					},
				},
			},
		},
		{
			name: "automated persisted query",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name}}`,
				Variables: map[string]any{"name": "aereal"},
				Extensions: map[string]any{
					"persistedQuery": map[string]any{
						"version":    1,
						"sha256Hash": "d27e6805f86ffaf5b1c96ad70fe044580f6454a7731b30f0e93494afe25294d6",
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "user"),
						attribute.String("gql.resolver.alias", "user"),
						attribute.String("gql.resolver.args.name", "$name"),
						attribute.String("gql.resolver.path", "user"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.resolver.object", "User"),
						attribute.String("gql.resolver.field", "name"),
						attribute.String("gql.resolver.alias", "name"),
						attribute.String("gql.resolver.path", "user.name"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.request.variables.name", "aereal"),
						attribute.String("gql.request.apq.hash", "d27e6805f86ffaf5b1c96ad70fe044580f6454a7731b30f0e93494afe25294d6"),
						attribute.Bool("gql.request.apq.sent_query", true),
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 2),
					},
				},
			},
		},
		{
			name: "error from root field",
			params: &graphql.RawParams{
				Query:     `query($name: String!) {user(name: $name) {name}}`,
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "user"),
						attribute.String("gql.resolver.alias", "user"),
						attribute.String("gql.resolver.args.name", "$name"),
						attribute.String("gql.resolver.path", "user"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user forbidden\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("gql.request.variables.name", "forbidden"),
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 2),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("gql.errors.path", "user"),
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
				Query:     `query($name: String!) {user(name: $name) {name age}}`,
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "user"),
						attribute.String("gql.resolver.alias", "user"),
						attribute.String("gql.resolver.args.name", "$name"),
						attribute.String("gql.resolver.path", "user"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "User/name",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.resolver.object", "User"),
						attribute.String("gql.resolver.field", "name"),
						attribute.String("gql.resolver.alias", "name"),
						attribute.String("gql.resolver.path", "user.name"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					},
				},
				{
					Name:     "User/age",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.String("gql.resolver.object", "User"),
						attribute.String("gql.resolver.field", "age"),
						attribute.String("gql.resolver.alias", "age"),
						attribute.String("gql.resolver.path", "user.age"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					},
				},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Status:   sdktrace.Status{Code: codes.Error, Description: "input: user.name invalid name\ninput: user.age invalid age\n"},
					Attributes: []attribute.KeyValue{
						attribute.String("gql.request.variables.name", "invalid"),
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 3),
					},
					Events: []sdktrace.Event{
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("gql.errors.path", "user.name"),
								semconv.ExceptionTypeKey.String("*gqlerror.Error"),
								semconv.ExceptionMessageKey.String("input: user.name invalid name"),
								attrStacktrace,
							},
						},
						{
							Name: semconv.ExceptionEventName,
							Attributes: []attribute.KeyValue{
								attribute.String("gql.errors.path", "user.age"),
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "root"),
						attribute.String("gql.resolver.alias", "root"),
						attribute.String("gql.resolver.args.num", "<nil>"),
						attribute.Bool("gql.resolver.args.num.default", true),
						attribute.String("gql.resolver.args.rootInput.nested", "{}"),
						attribute.Bool("gql.resolver.args.rootInput.default", true),
						attribute.String("gql.resolver.path", "root"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 1),
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
						attribute.String("gql.resolver.object", "Query"),
						attribute.String("gql.resolver.field", "root"),
						attribute.String("gql.resolver.alias", "root"),
						attribute.String("gql.resolver.args.num", "<nil>"),
						attribute.Bool("gql.resolver.args.num.default", true),
						attribute.String("gql.resolver.args.rootInput.nested.val", `"root"`),
						attribute.String("gql.resolver.path", "root"),
						attribute.Bool("gql.resolver.is_method", true),
						attribute.Bool("gql.resolver.is_resolver", true),
					}},
				{
					Name:     "anonymous-op",
					SpanKind: trace.SpanKindServer,
					Attributes: []attribute.KeyValue{
						attribute.Int("gql.request.complexity.limit", 1000),
						attribute.Int("gql.request.complexity.calculated", 1),
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
			gqlsrv.Use(otelgqlgen.New(otelgqlgen.WithTracerProvider(tp)))
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
