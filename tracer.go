package otelgqlgen

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/contrib"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	extensionName   = "CustomOpenTelemetryTracer"
	tracerName      = "github.com/aereal/otel-gqlgen"
	anonymousOpName = "anonymous-op"
)

type config struct {
	tracerProvider trace.TracerProvider
}

type Option func(c *config)

func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		c.tracerProvider = tp
	}
}

func New(opts ...Option) Tracer {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.tracerProvider == nil {
		cfg.tracerProvider = otel.GetTracerProvider()
	}
	t := Tracer{
		tracer: cfg.tracerProvider.Tracer(tracerName, trace.WithInstrumentationVersion(contrib.SemVersion())),
	}
	return t
}

type Tracer struct {
	tracer trace.Tracer
}

var _ interface {
	graphql.HandlerExtension
	graphql.ResponseInterceptor
	graphql.FieldInterceptor
} = Tracer{}

func (Tracer) ExtensionName() string {
	return extensionName
}

func (Tracer) Validate(_ graphql.ExecutableSchema) error {
	return nil
}

func (t Tracer) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	opCtx := graphql.GetOperationContext(ctx)
	opts := make([]trace.SpanStartOption, 0, 2)
	opts = append(opts, trace.WithSpanKind(trace.SpanKindServer))
	if !opCtx.Stats.OperationStart.IsZero() {
		opts = append(opts, trace.WithTimestamp(opCtx.Stats.OperationStart))
	}
	ctx, span := t.tracer.Start(ctx, operationName(ctx), opts...)
	defer span.End()
	if !span.IsRecording() {
		return next(ctx)
	}
	t.captureOperationTimings(ctx)

	attrs := make([]attribute.KeyValue, 0, len(opCtx.Variables)+2)
	for k, v := range opCtx.Variables {
		attrs = append(attrs, attrReqVariable(k, v))
	}
	if stats := extension.GetApqStats(ctx); stats != nil {
		attrs = append(attrs,
			attribute.String(apqPrefix.With("hash").Encode(), stats.Hash),
			attribute.Bool(apqPrefix.With("sent_query").Encode(), stats.SentQuery),
		)
	}
	span.SetAttributes(attrs...)
	resp := next(ctx)
	if resp != nil && len(resp.Errors) > 0 {
		recordGQLErrors(span, resp.Errors)
	}
	return resp
}

func (t Tracer) captureOperationTimings(ctx context.Context) {
	stats := graphql.GetOperationContext(ctx).Stats
	var (
		timing graphql.TraceTiming
		span   trace.Span
	)
	timing = stats.Parsing
	_, span = t.tracer.Start(ctx, "parsing", trace.WithTimestamp(timing.Start), trace.WithSpanKind(trace.SpanKindServer))
	span.End(trace.WithTimestamp(timing.End))
	timing = stats.Read
	_, span = t.tracer.Start(ctx, "read", trace.WithTimestamp(timing.Start), trace.WithSpanKind(trace.SpanKindServer))
	span.End(trace.WithTimestamp(timing.End))
	timing = stats.Validation
	_, span = t.tracer.Start(ctx, "validation", trace.WithTimestamp(timing.Start), trace.WithSpanKind(trace.SpanKindServer))
	span.End(trace.WithTimestamp(timing.End))
}

func (t Tracer) InterceptField(ctx context.Context, next graphql.Resolver) (any, error) {
	fieldCtx := graphql.GetFieldContext(ctx)
	field := fieldCtx.Field
	spanName := fmt.Sprintf("%s/%s", field.ObjectDefinition.Name, field.Name)
	ctx, span := t.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	if !span.IsRecording() {
		return next(ctx)
	}

	attrs := attrsField(field)
	attrs = append(attrs, attrPath(fieldCtx.Path().String()))
	span.SetAttributes(attrs...)
	span.SetAttributes(
		attribute.Bool(resolverPrefix.With("is_method").Encode(), fieldCtx.IsMethod),
		attribute.Bool(resolverPrefix.With("is_resolver").Encode(), fieldCtx.IsResolver),
	)

	resp, err := next(ctx)
	if errs := graphql.GetFieldErrors(ctx, fieldCtx); len(errs) > 0 {
		recordGQLErrors(span, errs)
	}
	return resp, err
}

func operationName(ctx context.Context) string {
	opCtx := graphql.GetOperationContext(ctx)
	if name := opCtx.OperationName; name != "" {
		return name
	}
	return anonymousOpName
}

func recordGQLErrors(span trace.Span, errs gqlerror.List) {
	span.SetStatus(codes.Error, errs.Error())
	for i, e := range errs {
		ns := errPrefix.With(strconv.Itoa(i))
		attrs := []attribute.KeyValue{
			attribute.String(ns.With("message").Encode(), e.Message),
			attribute.Stringer(ns.With("path").Encode(), e.Path),
		}
		span.RecordError(e, trace.WithStackTrace(true), trace.WithAttributes(attrs...))
	}
}

func attrsField(field graphql.CollectedField) []attribute.KeyValue {
	max := 3 + len(field.Definition.Arguments)*2
	attrs := make([]attribute.KeyValue, 0, max)
	attrs = append(attrs,
		attrObject(field.ObjectDefinition.Name),
		attrFieldName(field.Name),
		attrAlias(field.Alias),
	)
	for _, def := range field.Definition.Arguments {
		current := argsPrefix.With(def.Name)
		if def.DefaultValue != nil && len(def.DefaultValue.Children) > 0 {
			attrs = append(attrs, childAttrs(def.DefaultValue.Children, current)...)
		}
		if arg := field.Arguments.ForName(def.Name); arg != nil {
			if arg.Value != nil && len(arg.Value.Children) > 0 {
				attrs = append(attrs, childAttrs(arg.Value.Children, current)...)
			} else {
				attrs = append(attrs,
					attribute.Stringer(argsPrefix.With(arg.Name).Encode(), arg.Value),
				)
			}
		} else {
			attrs = append(attrs, attribute.Bool(current.With("default").Encode(), true))
			if def.DefaultValue != nil && len(def.DefaultValue.Children) > 0 {
				attrs = append(attrs, childAttrs(def.DefaultValue.Children, current)...)
			} else {
				attrs = append(attrs, attribute.Stringer(current.Encode(), def.DefaultValue))
			}
		}
	}
	return attrs
}

func childAttrs(children ast.ChildValueList, ns attrNameHierarchy) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0)
	for _, child := range children {
		current := ns.With(child.Name)
		if len(child.Value.Children) > 0 {
			attrs = append(attrs, childAttrs(child.Value.Children, current)...)
		} else {
			attrs = append(attrs, attribute.Stringer(current.Encode(), child.Value))
		}
	}
	return attrs
}

func attrObject(v string) attribute.KeyValue {
	return attribute.String(resolverPrefix.With("object").Encode(), v)
}

func attrFieldName(v string) attribute.KeyValue {
	return attribute.String(resolverPrefix.With("field").Encode(), v)
}

func attrAlias(v string) attribute.KeyValue {
	return attribute.String(resolverPrefix.With("alias").Encode(), v)
}

func attrPath(v string) attribute.KeyValue {
	return attribute.String(resolverPrefix.With("path").Encode(), v)
}

func attrReqVariable(key string, val any) attribute.KeyValue {
	return attribute.String(requestPrefix.With("variables", key).Encode(), fmt.Sprintf("%+v", val))
}

var (
	attrPrefix     = attrNameHierarchy{"gql"}
	errPrefix      = attrPrefix.With("errors")
	resolverPrefix = attrPrefix.With("resolver")
	argsPrefix     = resolverPrefix.With("args")
	requestPrefix  = attrPrefix.With("request")
	apqPrefix      = requestPrefix.With("apq")
)

type attrNameHierarchy []string

func (ns attrNameHierarchy) Encode() string {
	return strings.Join(ns, ".")
}

func (ns attrNameHierarchy) With(parts ...string) attrNameHierarchy {
	xs := ns[:]
	xs = append(xs, parts...)
	return xs
}
