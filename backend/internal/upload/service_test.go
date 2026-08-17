package upload

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"vexgo/backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.MediaFile{}, &model.User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dataDir := t.TempDir()
	svc := NewService(Deps{DB: db, Storage: NewLocalStorage(dataDir)})
	return svc, dataDir
}

func seedUser(t *testing.T, db *gorm.DB, username, role string) model.User {
	t.Helper()
	u := model.User{Username: username, Email: username + "@example.com", Role: role}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return u
}

func TestUpload_WritesFileAndRecord(t *testing.T) {
	svc, dataDir := newTestService(t)
	user := seedUser(t, svc.db, "uploader", model.RoleContributor)

	media, err := svc.Upload(user.ID, "photo.jpg", 42, strings.NewReader("jpeg-data"))
	if err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if media.URL == "" || !strings.HasPrefix(media.URL, "/uploads/") {
		t.Errorf("expected /uploads/ URL, got %q", media.URL)
	}
	if media.Size != 42 || media.UserID != user.ID {
		t.Errorf("unexpected media record: %+v", media)
	}

	filename := filepath.Base(media.URL)
	content, err := os.ReadFile(filepath.Join(dataDir, "media", filename))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(content) != "jpeg-data" {
		t.Errorf("unexpected file content %q", string(content))
	}

	// record persisted
	var stored model.MediaFile
	if err := svc.db.First(&stored, media.ID).Error; err != nil {
		t.Fatalf("media record not saved: %v", err)
	}
}

func TestListByUser(t *testing.T) {
	svc, _ := newTestService(t)
	u1 := seedUser(t, svc.db, "a", model.RoleContributor)
	u2 := seedUser(t, svc.db, "b", model.RoleContributor)

	if _, err := svc.Upload(u1.ID, "1.jpg", 1, strings.NewReader("a")); err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if _, err := svc.Upload(u2.ID, "2.jpg", 1, strings.NewReader("b")); err != nil {
		t.Fatalf("Upload error: %v", err)
	}

	files, err := svc.ListByUser(u1.ID)
	if err != nil {
		t.Fatalf("ListByUser error: %v", err)
	}
	if len(files) != 1 || files[0].UserID != u1.ID {
		t.Errorf("expected 1 file for user 1, got %+v", files)
	}
}

func TestDelete_PermissionsAndFileRemoval(t *testing.T) {
	svc, dataDir := newTestService(t)
	owner := seedUser(t, svc.db, "owner", model.RoleContributor)
	other := seedUser(t, svc.db, "other", model.RoleGuest)

	media, err := svc.Upload(owner.ID, "del.jpg", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Upload error: %v", err)
	}

	idStr := strconv.FormatUint(uint64(media.ID), 10)

	// other user cannot delete
	if err := svc.Delete(idStr, other.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}

	// owner can delete → file removed from disk and DB
	if err := svc.Delete(idStr, owner.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "media", filepath.Base(media.URL))); !os.IsNotExist(err) {
		t.Errorf("expected file removed from disk")
	}
	var count int64
	if err := svc.db.Model(&model.MediaFile{}).Count(&count).Error; err != nil {
		t.Fatalf("count error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected no media records left, got %d", count)
	}

	// not found
	if err := svc.Delete(idStr, owner.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
