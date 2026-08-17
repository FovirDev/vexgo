package message

import (
	"testing"

	"vexgo/backend/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newTestDB opens an isolated in-memory SQLite database with a single
// connection so all queries hit the same database instance.
func newTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Notification{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func newTestService(t *testing.T) *Service {
	return NewService(Deps{DB: newTestDB(t)})
}

func TestCreateNotification(t *testing.T) {
	svc := newTestService(t)

	err := svc.CreateNotification(1, "comment", "New comment", "someone commented", "42", "post")
	if err != nil {
		t.Fatalf("CreateNotification error: %v", err)
	}

	var n model.Notification
	if err := svc.db.First(&n).Error; err != nil {
		t.Fatalf("notification not saved: %v", err)
	}
	if n.UserID != 1 || n.Type != "comment" || n.Title != "New comment" {
		t.Errorf("unexpected notification: %+v", n)
	}
	if n.IsRead {
		t.Errorf("expected new notification to be unread")
	}
}

func TestList_PaginationAndFilters(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < 5; i++ {
		if err := svc.CreateNotification(1, "comment", "c", "content", "", ""); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}
	}
	// one of them read
	var first model.Notification
	if err := svc.db.First(&first).Error; err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	first.IsRead = true
	if err := svc.db.Save(&first).Error; err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// page 1, limit 2
	list, total, err := svc.List(1, 1, 2, "", "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items on page 1, got %d", len(list))
	}

	// filter unread only
	list, total, err = svc.List(1, 1, 10, "", "false")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if total != 4 {
		t.Errorf("expected 4 unread, got %d", total)
	}
	if len(list) != 4 {
		t.Errorf("expected 4 unread items, got %d", len(list))
	}

	// filter by type
	list, total, err = svc.List(1, 1, 10, "comment", "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected 5 comment notifications, got %d", total)
	}
}

func TestMarkAsRead(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CreateNotification(1, "comment", "t", "c", "", ""); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	var n model.Notification
	if err := svc.db.First(&n).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}

	rows, err := svc.MarkAsRead(1, int(n.ID))
	if err != nil {
		t.Fatalf("MarkAsRead error: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	var updated model.Notification
	if err := svc.db.First(&updated, n.ID).Error; err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if !updated.IsRead {
		t.Errorf("expected notification to be read")
	}

	// other user cannot mark it read
	rows, err = svc.MarkAsRead(2, int(n.ID))
	if err != nil {
		t.Fatalf("MarkAsRead error: %v", err)
	}
	if rows != 0 {
		t.Errorf("expected 0 rows for foreign user, got %d", rows)
	}
}

func TestMarkAllAsRead(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CreateNotification(1, "comment", "t", "c", "", ""); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if err := svc.CreateNotification(2, "comment", "t", "c", "", ""); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := svc.MarkAllAsRead(1); err != nil {
		t.Fatalf("MarkAllAsRead error: %v", err)
	}

	count, err := svc.UnreadCount(1)
	if err != nil {
		t.Fatalf("UnreadCount error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread for user 1, got %d", count)
	}
	// user 2 unaffected
	count, err = svc.UnreadCount(2)
	if err != nil {
		t.Fatalf("UnreadCount error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 unread for user 2, got %d", count)
	}
}

func TestDelete(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CreateNotification(1, "comment", "t", "c", "", ""); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	var n model.Notification
	if err := svc.db.First(&n).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}

	rows, err := svc.Delete(2, int(n.ID))
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if rows != 0 {
		t.Errorf("expected 0 rows for foreign user, got %d", rows)
	}

	rows, err = svc.Delete(1, int(n.ID))
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	var count int64
	if err := svc.db.Model(&model.Notification{}).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 notifications left, got %d", count)
	}
}
