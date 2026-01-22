package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/endalk200/user-api/models"
	"github.com/endalk200/user-api/storage"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	store  *storage.JSONStore
	logger *slog.Logger

	// Custom metrics
	userCounter    metric.Int64UpDownCounter // Current number of users
	usersCreated   metric.Int64Counter       // Total users created
	usersDeleted   metric.Int64Counter       // Total users deleted
	userOperations metric.Int64Counter       // Total user operations by type
}

// NewUserHandler creates a new user handler with metrics
func NewUserHandler(store *storage.JSONStore, logger *slog.Logger, meter metric.Meter) *UserHandler {
	// Initialize custom metrics
	userCounter, err := meter.Int64UpDownCounter(
		"user_api_users_total",
		metric.WithDescription("Current number of users in the system"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		logger.Error("Failed to create user counter metric", "error", err)
	}

	usersCreated, err := meter.Int64Counter(
		"user_api_users_created_total",
		metric.WithDescription("Total number of users created"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		logger.Error("Failed to create users created metric", "error", err)
	}

	usersDeleted, err := meter.Int64Counter(
		"user_api_users_deleted_total",
		metric.WithDescription("Total number of users deleted"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		logger.Error("Failed to create users deleted metric", "error", err)
	}

	userOperations, err := meter.Int64Counter(
		"user_api_operations_total",
		metric.WithDescription("Total number of user operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		logger.Error("Failed to create user operations metric", "error", err)
	}

	// Initialize user counter with current count from storage
	handler := &UserHandler{
		store:          store,
		logger:         logger,
		userCounter:    userCounter,
		usersCreated:   usersCreated,
		usersDeleted:   usersDeleted,
		userOperations: userOperations,
	}

	// Set initial user count
	if users, err := store.GetAll(); err == nil {
		userCounter.Add(context.Background(), int64(len(users)))
	}

	return handler
}

// GetAll returns all users
// GET /users
func (h *UserHandler) GetAll(c *gin.Context) {
	ctx := c.Request.Context()
	span := trace.SpanFromContext(ctx)

	h.logger.InfoContext(ctx, "Fetching all users")
	h.userOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "get_all")))

	users, err := h.store.GetAll()
	if err != nil {
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to fetch users", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	span.SetAttributes(attribute.Int("user_count", len(users)))
	h.logger.InfoContext(ctx, "Successfully fetched users", "count", len(users))
	c.JSON(http.StatusOK, users)
}

// GetByID returns a user by ID
// GET /users/:id
func (h *UserHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	span := trace.SpanFromContext(ctx)

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))
	h.logger.InfoContext(ctx, "Fetching user by ID", "user_id", id)
	h.userOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "get_by_id")))

	user, err := h.store.GetByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(ctx, "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to fetch user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully fetched user", "user_id", id)
	c.JSON(http.StatusOK, user)
}

// Create creates a new user
// POST /users
func (h *UserHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	span := trace.SpanFromContext(ctx)

	h.userOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "create")))

	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WarnContext(ctx, "Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Creating new user", "name", req.Name, "email", req.Email)

	now := time.Now()
	user := models.User{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: now,
		UpdatedAt: now,
	}

	span.SetAttributes(
		attribute.String("user.id", user.ID),
		attribute.String("user.email", user.Email),
	)

	if err := h.store.Create(user); err != nil {
		if errors.Is(err, storage.ErrUserExists) {
			h.logger.WarnContext(ctx, "User with email already exists", "email", req.Email)
			c.JSON(http.StatusConflict, gin.H{"error": "User with this email already exists"})
			return
		}
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Record metrics for successful user creation
	h.usersCreated.Add(ctx, 1)
	h.userCounter.Add(ctx, 1)

	h.logger.InfoContext(ctx, "Successfully created user", "user_id", user.ID)
	c.JSON(http.StatusCreated, user)
}

// Update updates an existing user
// PUT /users/:id
func (h *UserHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	span := trace.SpanFromContext(ctx)

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))
	h.userOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "update")))

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WarnContext(ctx, "Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Updating user", "user_id", id, "name", req.Name, "email", req.Email)

	// Get existing user to preserve created_at
	existingUser, err := h.store.GetByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(ctx, "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to fetch user for update", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	user := models.User{
		ID:        id,
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: existingUser.CreatedAt,
		UpdatedAt: time.Now(),
	}

	if err := h.store.Update(user); err != nil {
		if errors.Is(err, storage.ErrUserExists) {
			h.logger.WarnContext(ctx, "Email already in use", "email", req.Email)
			c.JSON(http.StatusConflict, gin.H{"error": "Email already in use by another user"})
			return
		}
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to update user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully updated user", "user_id", id)
	c.JSON(http.StatusOK, user)
}

// Delete removes a user
// DELETE /users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	span := trace.SpanFromContext(ctx)

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))
	h.logger.InfoContext(ctx, "Deleting user", "user_id", id)
	h.userOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "delete")))

	if err := h.store.Delete(id); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(ctx, "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		span.RecordError(err)
		h.logger.ErrorContext(ctx, "Failed to delete user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	// Record metrics for successful user deletion
	h.usersDeleted.Add(ctx, 1)
	h.userCounter.Add(ctx, -1)

	h.logger.InfoContext(ctx, "Successfully deleted user", "user_id", id)
	c.JSON(http.StatusNoContent, nil)
}
