package database

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	id := uuid.New()
	createdAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(createUser)).
		WithArgs("person@example.com", "hashed_secret").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password"}).
			AddRow(id.String(), createdAt, createdAt, "person@example.com", "hashed_secret"))

	user, err := New(db).CreateUser(context.Background(), CreateUserParams{
		Email:          "person@example.com",
		HashedPassword: "hashed_secret",
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if user.ID != id || user.CreatedAt != createdAt || user.UpdatedAt != createdAt || user.Email != "person@example.com" || user.HashedPassword != "hashed_secret" {
		t.Errorf("CreateUser returned %+v, want ID %s, email person@example.com, and password hashed_secret", user, id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database expectations: %v", err)
	}
}

func TestClearUsersTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec(regexp.QuoteMeta(clearUsersTable)).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := New(db).ClearUsersTable(context.Background()); err != nil {
		t.Fatalf("ClearUsersTable returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database expectations: %v", err)
	}
}
