package auth_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/test/testutil"
)

func TestHandler_Register_Success(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: check if user exists (should return false)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	// Mock: insert new user - use RETURNING clause to get ID
	mock.ExpectQuery(`INSERT INTO "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	reqBody := []byte(`{"username":"testuser","email":"test@example.com","password":"password123"}`)
	c, w := testutil.NewTestContext("POST", "/auth/register", reqBody)

	handler.Register(c)

	assert.Equal(t, 201, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())

	var resp sacv1.AuthResponse
	err := protojson.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "testuser", resp.User.Username)
	assert.Equal(t, "test@example.com", resp.User.Email)
}

func TestHandler_Register_MissingFields(t *testing.T) {
	db, _, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	reqBody := []byte(`{"username":"testuser"}`)
	c, w := testutil.NewTestContext("POST", "/auth/register", reqBody)

	handler.Register(c)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "required")
}

func TestHandler_Register_ShortPassword(t *testing.T) {
	db, _, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	reqBody := []byte(`{"username":"testuser","email":"test@example.com","password":"123"}`)
	c, w := testutil.NewTestContext("POST", "/auth/register", reqBody)

	handler.Register(c)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "at least 6 characters")
}

func TestHandler_Register_DuplicateUser(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: check if user exists (should return true)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	reqBody := []byte(`{"username":"testuser","email":"test@example.com","password":"password123"}`)
	c, w := testutil.NewTestContext("POST", "/auth/register", reqBody)

	handler.Register(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "already exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandler_Login_Success(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Hash the password "password123"
	hashedPassword, _ := auth.HashPassword("password123")

	// Mock: find user by username
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "password_hash", "role"}).
			AddRow(1, "testuser", "test@example.com", hashedPassword, "user"))

	reqBody := []byte(`{"username":"testuser","password":"password123"}`)
	c, w := testutil.NewTestContext("POST", "/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, 200, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())

	var resp sacv1.AuthResponse
	err := protojson.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "testuser", resp.User.Username)
}

func TestHandler_Login_MissingFields(t *testing.T) {
	db, _, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	reqBody := []byte(`{"username":"testuser"}`)
	c, w := testutil.NewTestContext("POST", "/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "required")
}

func TestHandler_Login_UserNotFound(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: user not found (return error)
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnError(sqlmock.ErrCancelled)

	reqBody := []byte(`{"username":"nonexistent","password":"password123"}`)
	c, w := testutil.NewTestContext("POST", "/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid credentials")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandler_Login_InvalidPassword(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	hashedPassword, _ := auth.HashPassword("correctpassword")

	// Mock: find user
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "password_hash", "role"}).
			AddRow(1, "testuser", "test@example.com", hashedPassword, "user"))

	reqBody := []byte(`{"username":"testuser","password":"wrongpassword"}`)
	c, w := testutil.NewTestContext("POST", "/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid credentials")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandler_GetCurrentUser_Success(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: find user by ID
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "role"}).
			AddRow(1, "testuser", "test@example.com", "user"))

	c, w := testutil.NewTestContext("GET", "/auth/me", nil)
	testutil.SetAuthContext(c, 1, "testuser", "user")

	handler.GetCurrentUser(c)

	assert.Equal(t, 200, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())

	var resp sacv1.User
	err := protojson.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Id)
	assert.Equal(t, "testuser", resp.Username)
}

func TestHandler_GetCurrentUser_NotFound(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: user not found
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnError(sqlmock.ErrCancelled)

	c, w := testutil.NewTestContext("GET", "/auth/me", nil)
	testutil.SetAuthContext(c, 999, "nonexistent", "user")

	handler.GetCurrentUser(c)

	assert.Equal(t, 404, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandler_SearchUsers_Success(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: search users
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name"}).
			AddRow(1, "testuser1", "Test User 1").
			AddRow(2, "testuser2", "Test User 2"))

	c, w := testutil.NewTestContext("GET", "/auth/users/search?q=test", nil)
	testutil.SetAuthContext(c, 1, "testuser", "user")
	testutil.SetQueryParam(c, "q", "test")

	handler.SearchUsers(c)

	assert.Equal(t, 200, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.Contains(t, w.Body.String(), "testuser1")
	assert.Contains(t, w.Body.String(), "testuser2")
}

func TestHandler_SearchUsers_ShortQuery(t *testing.T) {
	db, _, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	c, w := testutil.NewTestContext("GET", "/auth/users/search?q=", nil)
	testutil.SetAuthContext(c, 1, "testuser", "user")
	testutil.SetQueryParam(c, "q", "")

	handler.SearchUsers(c)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "too short")
}

func TestHandler_SearchUsers_NoResults(t *testing.T) {
	db, mock, cleanup := testutil.NewMockDB(t)
	defer cleanup()

	jwtService := auth.NewJWTService("test-secret")
	settingsService := admin.NewSettingsService(db)
	handler := auth.NewHandler(db, jwtService, settingsService)

	// Mock: no results
	mock.ExpectQuery(`SELECT .* FROM "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name"}))

	c, w := testutil.NewTestContext("GET", "/auth/users/search?q=nonexistent", nil)
	testutil.SetAuthContext(c, 1, "testuser", "user")
	testutil.SetQueryParam(c, "q", "nonexistent")

	handler.SearchUsers(c)

	assert.Equal(t, 200, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.Contains(t, w.Body.String(), "[]")
}
