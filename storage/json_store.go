package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/endalk200/user-api/models"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user with this email already exists")
)

// JSONStore handles JSON file-based storage for users
type JSONStore struct {
	filePath string
	mu       sync.RWMutex
	users    map[string]models.User
}

// NewJSONStore creates a new JSON store instance
func NewJSONStore(filePath string) (*JSONStore, error) {
	store := &JSONStore{
		filePath: filePath,
		users:    make(map[string]models.User),
	}

	// Ensure the directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Load existing data if file exists
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return store, nil
}

// load reads users from the JSON file into memory
func (s *JSONStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var users []models.User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}

	s.users = make(map[string]models.User)
	for _, user := range users {
		s.users[user.ID] = user
	}

	return nil
}

// save writes all users from memory to the JSON file
func (s *JSONStore) save() error {
	users := make([]models.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// GetAll returns all users
func (s *JSONStore) GetAll() ([]models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]models.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	return users, nil
}

// GetByID returns a user by ID
func (s *JSONStore) GetByID(id string) (models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return models.User{}, ErrUserNotFound
	}

	return user, nil
}

// Create adds a new user
func (s *JSONStore) Create(user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if email already exists
	for _, existingUser := range s.users {
		if existingUser.Email == user.Email {
			return ErrUserExists
		}
	}

	s.users[user.ID] = user
	return s.save()
}

// Update modifies an existing user
func (s *JSONStore) Update(user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return ErrUserNotFound
	}

	// Check if email is being changed to one that already exists
	for id, existingUser := range s.users {
		if existingUser.Email == user.Email && id != user.ID {
			return ErrUserExists
		}
	}

	s.users[user.ID] = user
	return s.save()
}

// Delete removes a user by ID
func (s *JSONStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[id]; !exists {
		return ErrUserNotFound
	}

	delete(s.users, id)
	return s.save()
}
