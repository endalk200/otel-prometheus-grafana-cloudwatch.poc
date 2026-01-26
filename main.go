package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/endalk200/user-api/handlers"
	"github.com/endalk200/user-api/storage"
)

const serviceName = "user-api"

func main() {
	ctx := context.Background()

	// Initialize OpenTelemetry
	otelShutdown, err := initOtel(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize OpenTelemetry: %v", err)
	}
	defer otelShutdown(ctx)

	// Create logger using OTel bridge
	logger := otelslog.NewLogger(serviceName)
	logger.Info("Starting User API server")

	// Initialize storage
	dataPath := getEnv("DATA_PATH", "./data/users.json")
	store, err := storage.NewJSONStore(dataPath)
	if err != nil {
		logger.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	logger.Info("Storage initialized", "path", dataPath)

	// Get meter for custom metrics
	meter := otel.Meter(serviceName)

	// Initialize Gin router
	gin.SetMode(getEnv("GIN_MODE", gin.DebugMode))
	router := gin.New()

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add OpenTelemetry middleware for automatic tracing
	router.Use(otelgin.Middleware(serviceName))

	// Add logging middleware
	router.Use(loggingMiddleware(logger))

	// Initialize handlers with meter for custom metrics
	userHandler := handlers.NewUserHandler(store, logger, meter)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// User routes
	users := router.Group("/users")
	{
		users.GET("", userHandler.GetAll)
		users.GET("/:id", userHandler.GetByID)
		users.POST("", userHandler.Create)
		users.PUT("/:id", userHandler.Update)
		users.DELETE("/:id", userHandler.Delete)
	}

	// Start server
	port := getEnv("PORT", "8080")
	logger.Info("Server starting", "port", port)

	if err := router.Run(":" + port); err != nil {
		logger.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}

// initOtel initializes OpenTelemetry with traces, metrics, and logs
func initOtel(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error

	// Create resource with service information
	// Use empty schema URL to avoid conflicts with resource.Default() which uses the SDK's schema
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Set up propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize trace provider
	traceShutdown, err := initTracerProvider(ctx, res)
	if err != nil {
		return nil, err
	}
	shutdownFuncs = append(shutdownFuncs, traceShutdown)

	// Initialize meter provider
	meterShutdown, err := initMeterProvider(ctx, res)
	if err != nil {
		return nil, err
	}
	shutdownFuncs = append(shutdownFuncs, meterShutdown)

	// Initialize logger provider
	loggerShutdown, err := initLoggerProvider(ctx, res)
	if err != nil {
		return nil, err
	}
	shutdownFuncs = append(shutdownFuncs, loggerShutdown)

	// Return combined shutdown function
	return func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			if shutdownErr := fn(ctx); shutdownErr != nil {
				err = shutdownErr
			}
		}
		return err
	}, nil
}

// initTracerProvider initializes the OpenTelemetry tracer provider
func initTracerProvider(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	// Create OTLP trace exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")),
	)
	if err != nil {
		return nil, err
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// initMeterProvider initializes the OpenTelemetry meter provider
func initMeterProvider(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	// Create OTLP metric exporter
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")),
	)
	if err != nil {
		return nil, err
	}

	// Create meter provider with periodic reader
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
	)

	// Set global meter provider
	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}

// initLoggerProvider initializes the OpenTelemetry logger provider
func initLoggerProvider(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	// Create OTLP log exporter
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")),
	)
	if err != nil {
		return nil, err
	}

	// Create logger provider
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	// Set global logger provider
	global.SetLoggerProvider(lp)

	return lp.Shutdown, nil
}

// loggingMiddleware logs incoming HTTP requests
func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Log request
		logger.InfoContext(c.Request.Context(), "Incoming request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
		)

		// Process request
		c.Next()

		// Log response
		logger.InfoContext(c.Request.Context(), "Request completed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	}
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
