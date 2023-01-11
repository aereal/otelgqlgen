package otelgqlgen

import (
	"context"
	"fmt"
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
	extensionName                  = "CustomOpenTelemetryTracer"
	tracerName                     = "github.com/aereal/otel-gqlgen"
	anonymousOpName                = "anonymous-op"
	defaultComplexityExtensionName = "ComplexityLimit"
)

type config struct {
	tracerProvider          trace.TracerProvider
	complexityExtensionName string
}

type Option func(c *config)

func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		c.tracerProvider = tp
	}
}

// WithComplexityLimitExtensionName creates an Option that tells Tracer to get complexity stats calculated by the extension identified by the given name.
func WithComplexityLimitExtensionName(extName string) Option {
	return func(c *config) {
		c.complexityExtensionName = extName
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
		tracer:                  cfg.tracerProvider.Tracer(tracerName, trace.WithInstrumentationVersion(contrib.SemVersion())),
		complexityExtensionName: cfg.complexityExtensionName,
	}
	if t.complexityExtensionName == "" {
		t.complexityExtensionName = defaultComplexityExtensionName
	}
	return t
}

type Tracer struct {
	tracer                  trace.Tracer
	complexityExtensionName string
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

	attrs := make([]attribute.KeyValue, 0, len(opCtx.Variables)+2+2)
	for k, v := range opCtx.Variables {
		attrs = append(attrs, attrReqVariable(k, v))
	}
	if stats := extension.GetApqStats(ctx); stats != nil {
		attrs = append(attrs,
			keyAPQHash.String(stats.Hash),
			keyAPQSendQuery.Bool(stats.SentQuery),
		)
	}
	if stats, ok := opCtx.Stats.GetExtension(t.complexityExtensionName).(*extension.ComplexityStats); stats != nil && ok {
		attrs = append(attrs,
			keyComplexityLimit.Int(stats.ComplexityLimit),
			keyComplexityCalculated.Int(stats.Complexity),
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
	attrs = append(attrs, keyResolverPath.String(fieldCtx.Path().String()))
	span.SetAttributes(attrs...)
	span.SetAttributes(
		keyFieldIsMethod.Bool(fieldCtx.IsMethod),
		keyFieldIsResolver.Bool(fieldCtx.IsResolver),
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
	for _, e := range errs {
		attrs := []attribute.KeyValue{
			keyErrorPath.String(e.Path.String()),
		}
		span.RecordError(e, trace.WithStackTrace(true), trace.WithAttributes(attrs...))
	}
}

func attrsField(field graphql.CollectedField) []attribute.KeyValue {
	max := 3 + len(field.Definition.Arguments)*2 + len(field.Directives)*2
	attrs := make([]attribute.KeyValue, 0, max)
	attrs = append(attrs,
		keyResolverObject.String(field.ObjectDefinition.Name),
		keyResolverFieldName.String(field.Name),
		keyResolverAlias.String(field.Alias),
	)
	for _, directive := range field.Directives {
		ns := directivePrefix.With(directive.Name)
		attrs = append(attrs, ns.With("location").asKey().String(string(directive.Location)))
		for _, arg := range directive.Arguments {
			current := ns.With("args", arg.Name)
			if len(arg.Value.Children) > 0 {
				attrs = append(attrs, childAttrs(arg.Value.Children, current)...)
			} else {
				attrs = append(attrs, current.asKey().String(arg.Value.String()))
			}
		}
	}
	for _, def := range field.Definition.Arguments {
		current := argsPrefix.With(def.Name)
		if arg := field.Arguments.ForName(def.Name); arg != nil {
			if arg.Value != nil && len(arg.Value.Children) > 0 {
				attrs = append(attrs, childAttrs(arg.Value.Children, current)...)
			} else {
				attrs = append(attrs,
					argsPrefix.With(arg.Name).asKey().String(arg.Value.String()),
				)
			}
		} else {
			if def.DefaultValue != nil && len(def.DefaultValue.Children) > 0 {
				attrs = append(attrs, childAttrs(def.DefaultValue.Children, current)...)
			} else {
				attrs = append(attrs, current.asKey().String(def.DefaultValue.String()))
			}
			attrs = append(attrs, current.With("default").asKey().Bool(true))
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
			attrs = append(attrs, current.asKey().String(child.Value.String()))
		}
	}
	return attrs
}

func attrReqVariable(key string, val any) attribute.KeyValue {
	return reqVarsPrefix.With(key).asKey().String(fmt.Sprintf("%+v", val))
}

var (
	ns              = "gql"
	nsResolver      = ns + ".resolver"
	nsReq           = ns + ".request"
	directivePrefix = attrNameHierarchy{nsResolver + ".directives"}
	argsPrefix      = attrNameHierarchy{nsResolver + ".args"}
	reqVarsPrefix   = attrNameHierarchy{nsReq + ".variables"}

	keyAPQHash              = attribute.Key(nsReq + ".apq.hash")
	keyAPQSendQuery         = attribute.Key(nsReq + ".apq.sent_query")
	keyComplexityLimit      = attribute.Key(nsReq + ".complexity.limit")
	keyComplexityCalculated = attribute.Key(nsReq + ".complexity.calculated")
	keyResolverObject       = attribute.Key(nsResolver + ".object")
	keyResolverFieldName    = attribute.Key(nsResolver + ".field")
	keyResolverAlias        = attribute.Key(nsResolver + ".alias")
	keyResolverPath         = attribute.Key(nsResolver + ".path")
	keyFieldIsResolver      = attribute.Key(nsResolver + ".is_resolver")
	keyFieldIsMethod        = attribute.Key(nsResolver + ".is_method")
	keyErrorPath            = attribute.Key(ns + ".errors.path")
)

type attrNameHierarchy []string

func (ns attrNameHierarchy) asKey() attribute.Key {
	return attribute.Key(strings.Join(ns, "."))
}

func (ns attrNameHierarchy) With(parts ...string) attrNameHierarchy {
	xs := ns[:]
	xs = append(xs, parts...)
	return xs
}
