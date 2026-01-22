package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/endalk200/user-api/models"
	"github.com/endalk200/user-api/storage"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	store  *storage.JSONStore
	logger *slog.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(store *storage.JSONStore, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		store:  store,
		logger: logger,
	}
}

// GetAll returns all users
// GET /users
func (h *UserHandler) GetAll(c *gin.Context) {
	h.logger.InfoContext(c.Request.Context(), "Fetching all users")

	users, err := h.store.GetAll()
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "Failed to fetch users", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Successfully fetched users", "count", len(users))
	c.JSON(http.StatusOK, users)
}

// GetByID returns a user by ID
// GET /users/:id
func (h *UserHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	h.logger.InfoContext(c.Request.Context(), "Fetching user by ID", "user_id", id)

	user, err := h.store.GetByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(c.Request.Context(), "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "Failed to fetch user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Successfully fetched user", "user_id", id)
	c.JSON(http.StatusOK, user)
}

// Create creates a new user
// POST /users
func (h *UserHandler) Create(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WarnContext(c.Request.Context(), "Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Creating new user", "name", req.Name, "email", req.Email)

	now := time.Now()
	user := models.User{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.Create(user); err != nil {
		if errors.Is(err, storage.ErrUserExists) {
			h.logger.WarnContext(c.Request.Context(), "User with email already exists", "email", req.Email)
			c.JSON(http.StatusConflict, gin.H{"error": "User with this email already exists"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "Failed to create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Successfully created user", "user_id", user.ID)
	c.JSON(http.StatusCreated, user)
}

// Update updates an existing user
// PUT /users/:id
func (h *UserHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WarnContext(c.Request.Context(), "Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Updating user", "user_id", id, "name", req.Name, "email", req.Email)

	// Get existing user to preserve created_at
	existingUser, err := h.store.GetByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(c.Request.Context(), "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "Failed to fetch user for update", "user_id", id, "error", err)
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
			h.logger.WarnContext(c.Request.Context(), "Email already in use", "email", req.Email)
			c.JSON(http.StatusConflict, gin.H{"error": "Email already in use by another user"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "Failed to update user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Successfully updated user", "user_id", id)
	c.JSON(http.StatusOK, user)
}

// Delete removes a user
// DELETE /users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	h.logger.InfoContext(c.Request.Context(), "Deleting user", "user_id", id)

	if err := h.store.Delete(id); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			h.logger.WarnContext(c.Request.Context(), "User not found", "user_id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "Failed to delete user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "Successfully deleted user", "user_id", id)
	c.JSON(http.StatusNoContent, nil)
}
