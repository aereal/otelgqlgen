package otelgqlgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	extensionName                  = "CustomOpenTelemetryTracer"
	tracerName                     = "github.com/aereal/otelgqlgen"
	anonymousOpName                = "anonymous-op"
	defaultComplexityExtensionName = "ComplexityLimit"
)

type config struct {
	tracerProvider          trace.TracerProvider
	errorSelector           ErrorSelector
	complexityExtensionName string
	traceStructFields       bool
}

type Option func(c *config)

// WithTracerProvider creates an Optoin that tells Tracer to use given TracerProvider.
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

// TraceStructFields creates an Option that enforces Tracer to struct fields resolver.
//
// default value: false
// The false means the Tracer only traces the resolvers runs against struct methods or resolver methods.
func TraceStructFields(v bool) Option {
	return func(c *config) {
		c.traceStructFields = v
	}
}

// ErrorSelector is a predicate that the error should be recorded.
//
// The span records only errors that the function returns true.
// If the function returns false against all of the errors in the gqlgen response, the span status will be Unset instead of Error.
type ErrorSelector func(err error) bool

// WithErrorSelector creates an Option that tells Tracer uses the given selector.
func WithErrorSelector(fn ErrorSelector) Option { return func(c *config) { c.errorSelector = fn } }

// New returns a new Tracer with given options.
func New(opts ...Option) Tracer {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.tracerProvider == nil {
		cfg.tracerProvider = otel.GetTracerProvider()
	}
	t := Tracer{
		tracer:                  cfg.tracerProvider.Tracer(tracerName),
		complexityExtensionName: cfg.complexityExtensionName,
		traceStructFields:       cfg.traceStructFields,
		errorSelector:           cfg.errorSelector,
	}
	if t.complexityExtensionName == "" {
		t.complexityExtensionName = defaultComplexityExtensionName
	}
	if t.errorSelector == nil {
		t.errorSelector = func(_ error) bool { return true }
	}
	return t
}

// Tracer is a gqlgen extension to collect traces from the resolver.
type Tracer struct {
	tracer                  trace.Tracer
	errorSelector           ErrorSelector
	complexityExtensionName string
	traceStructFields       bool
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

func (t Tracer) startResponseSpan(ctx context.Context) (context.Context, trace.Span) {
	opCtx := graphql.GetOperationContext(ctx)
	name := operationName(ctx)
	opts := make([]trace.SpanStartOption, 0, 3)
	attrs := make([]attribute.KeyValue, 0, 2)
	attrs = append(attrs, semconv.GraphqlOperationNameKey.String(name))
	if op := opCtx.Operation; op != nil {
		attrs = append(attrs, semconv.GraphqlOperationTypeKey.String(string(op.Operation)))
	}
	opts = append(opts,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...))
	if !opCtx.Stats.OperationStart.IsZero() {
		opts = append(opts, trace.WithTimestamp(opCtx.Stats.OperationStart))
	}
	return t.tracer.Start(ctx, name, opts...)
}

func (t Tracer) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	parentSpan := trace.SpanFromContext(ctx)
	ctx, span := t.startResponseSpan(ctx)
	defer span.End()
	if !span.IsRecording() {
		return next(ctx)
	}
	t.captureOperationTimings(ctx)

	opCtx := graphql.GetOperationContext(ctx)
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
		recordGQLErrors(span, resp.Errors, t.errorSelector)
		if parentSpan.SpanContext().IsValid() {
			recordGQLErrors(parentSpan, resp.Errors, t.errorSelector)
		}
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

func fieldSpanName(fc *graphql.FieldContext) string {
	w := &strings.Builder{}
	fmt.Fprint(w, fc.Field.ObjectDefinition.Name)
	if fc.Parent != nil && fc.Parent.Index != nil {
		fmt.Fprintf(w, "/%d", *fc.Parent.Index)
	}
	fmt.Fprintf(w, "/%s", fc.Field.Name)
	if fc.Index != nil {
		fmt.Fprintf(w, "/%d", *fc.Index)
	}
	return w.String()
}

func (t Tracer) InterceptField(ctx context.Context, next graphql.Resolver) (any, error) {
	fieldCtx := graphql.GetFieldContext(ctx)
	if !t.traceStructFields && (!fieldCtx.IsMethod && !fieldCtx.IsResolver) {
		return next(ctx)
	}
	field := fieldCtx.Field
	ctx, span := t.tracer.Start(ctx, fieldSpanName(fieldCtx), trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	if !span.IsRecording() {
		return next(ctx)
	}

	attrs := attrsField(field)
	attrs = append(attrs,
		keyResolverPath.String(fieldCtx.Path().String()),
		keyFieldIsMethod.Bool(fieldCtx.IsMethod),
		keyFieldIsResolver.Bool(fieldCtx.IsResolver),
	)
	span.SetAttributes(attrs...)

	resp, err := next(ctx)
	if errs := graphql.GetFieldErrors(ctx, fieldCtx); len(errs) > 0 {
		recordGQLErrors(span, errs, t.errorSelector)
	}
	return resp, err
}

func operationName(ctx context.Context) string {
	opCtx := graphql.GetOperationContext(ctx)
	if name := opCtx.OperationName; name != "" {
		return name
	}
	op := opCtx.Operation
	if op == nil {
		return "GraphQL Operation"
	}
	if op.Name != "" {
		return op.Name
	}
	return string(op.Operation)
}

func recordGQLErrors(span trace.Span, errs gqlerror.List, selector ErrorSelector) {
	var recorded bool
	for _, gqlErr := range errs {
		if !selector(gqlErr) {
			continue
		}
		recorded = true
		attrErrorPath := keyErrorPath.String(gqlErr.Path.String())
		span.RecordError(unwrapErr(gqlErr), trace.WithStackTrace(true), trace.WithAttributes(attrErrorPath))
	}
	if !recorded {
		return
	}
	span.SetStatus(codes.Error, errs.Error())
}

func unwrapErr(err error) error {
	underlying := err
	for {
		wrapped, ok := underlying.(interface{ Unwrap() error })
		if !ok {
			return underlying
		}
		unwrapped := wrapped.Unwrap()
		if unwrapped == nil {
			return underlying
		}
		underlying = unwrapped
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
	ns              = "graphql"
	nsResolver      = ns + ".resolver"
	nsReq           = ns + ".operation"
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
