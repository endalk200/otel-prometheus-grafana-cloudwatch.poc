package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/endalk200/user-api/handlers"
	"github.com/endalk200/user-api/storage"
)

func main() {
	// Initialize OpenTelemetry logging
	logger, cleanup, err := initLogger()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer cleanup()

	logger.Info("Starting User API server")

	// Initialize storage
	dataPath := getEnv("DATA_PATH", "./data/users.json")
	store, err := storage.NewJSONStore(dataPath)
	if err != nil {
		logger.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	logger.Info("Storage initialized", "path", dataPath)

	// Initialize Gin router
	gin.SetMode(getEnv("GIN_MODE", gin.DebugMode))
	router := gin.New()

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add logging middleware
	router.Use(loggingMiddleware(logger))

	// Initialize handlers
	userHandler := handlers.NewUserHandler(store, logger)

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

// initLogger initializes OpenTelemetry logging with stdout exporter
func initLogger() (*slog.Logger, func(), error) {
	// Create stdout exporter for logs
	exporter, err := stdoutlog.New()
	if err != nil {
		return nil, nil, err
	}

	// Create log processor and provider
	processor := sdklog.NewSimpleProcessor(exporter)
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))

	// Set the global logger provider
	global.SetLoggerProvider(provider)

	// Create slog logger using OTel bridge
	logger := otelslog.NewLogger("user-api")

	// Cleanup function to shutdown the provider
	cleanup := func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down logger provider: %v", err)
		}
	}

	return logger, cleanup, nil
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
