package comment

import (
	"errors"
	"strconv"
	"testing"

	"vexgo/backend/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// fakeNotifier records notification calls instead of touching the DB.
type fakeNotifier struct {
	calls []string
}

func (f *fakeNotifier) CreateNotification(userID uint, notificationType string, title string, content string, relatedID string, relatedType string) error {
	f.calls = append(f.calls, notificationType)
	return nil
}

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
	if err := db.AutoMigrate(&model.Comment{}, &model.Post{}, &model.User{}, &model.CommentModerationConfig{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func newTestService(t *testing.T) (*Service, *fakeNotifier) {
	t.Helper()
	notifier := &fakeNotifier{}
	svc := NewService(Deps{DB: newTestDB(t), Notifier: notifier})
	return svc, notifier
}

func seedUser(t *testing.T, db *gorm.DB, username string, role string) model.User {
	t.Helper()
	u := model.User{Username: username, Email: username + "@example.com", Role: role}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return u
}

func seedPost(t *testing.T, db *gorm.DB, authorID uint) model.Post {
	t.Helper()
	p := model.Post{Title: "Post", Content: "body", Category: "1", AuthorID: authorID, Status: "published"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("failed to seed post: %v", err)
	}
	return p
}

func TestCreate_AutoApproved(t *testing.T) {
	svc, notifier := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	comment, count, err := svc.Create(post.ID, commenter.ID, "nice post", nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if comment.Status != "published" {
		t.Errorf("expected published, got %s", comment.Status)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if len(notifier.calls) != 1 || notifier.calls[0] != "comment" {
		t.Errorf("expected one comment notification, got %v", notifier.calls)
	}

	// author commenting on own post → no notification
	notifier.calls = nil
	_, _, err = svc.Create(post.ID, author.ID, "self comment", nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Errorf("expected no notification for own post, got %v", notifier.calls)
	}
}

func TestCreate_ModerationDisabledManualApproval(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	// Disable auto-approve. Note: the *create* path of UpdateModerationConfig
	// cannot persist AutoApproveEnabled=false because the model carries
	// gorm:"default:true" and GORM omits zero-value fields on Create — a
	// pre-existing bug worth fixing separately. The *update* path (Save) writes
	// zero values correctly, so create the row first, then update it.
	if _, err := svc.UpdateModerationConfig(UpdateModerationConfigRequest{Enabled: true}); err != nil {
		t.Fatalf("UpdateModerationConfig error: %v", err)
	}
	if _, err := svc.UpdateModerationConfig(UpdateModerationConfigRequest{
		Enabled:            false,
		AutoApproveEnabled: false,
	}); err != nil {
		t.Fatalf("UpdateModerationConfig error: %v", err)
	}

	comment, _, err := svc.Create(post.ID, commenter.ID, "needs review", nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if comment.Status != "pending" {
		t.Errorf("expected pending, got %s", comment.Status)
	}
}

func TestCreate_ModerationRejectsBlockedKeyword(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	if _, err := svc.UpdateModerationConfig(UpdateModerationConfigRequest{
		Enabled:       true,
		BlockKeywords: "spam,ad",
	}); err != nil {
		t.Fatalf("UpdateModerationConfig error: %v", err)
	}

	comment, _, err := svc.Create(post.ID, commenter.ID, "buy now spam", nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if comment.Status != "rejected" {
		t.Errorf("expected rejected, got %s", comment.Status)
	}
}

func TestCreate_ReplyNotifiesParentAuthor(t *testing.T) {
	svc, notifier := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)
	replier := seedUser(t, svc.db, "replier", model.RoleGuest)

	parentID := uint(0)
	_, _, err := svc.Create(post.ID, commenter.ID, "first", nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	var parent model.Comment
	if err := svc.db.First(&parent).Error; err != nil {
		t.Fatalf("failed to load parent: %v", err)
	}
	parentID = parent.ID

	notifier.calls = nil
	_, _, err = svc.Create(post.ID, replier.ID, "reply", &parentID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(notifier.calls) != 2 {
		t.Errorf("expected post + parent notifications, got %v", notifier.calls)
	}
}

func TestListByPost_PublishedOnlyAndPrivacy(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	commenter.ProfileVisibility = "private"
	svc.db.Save(&commenter)

	if _, _, err := svc.Create(post.ID, commenter.ID, "published one", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if _, _, err := svc.Create(post.ID, author.ID, "pending one", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	var pending model.Comment
	if err := svc.db.Where("content = ?", "pending one").First(&pending).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	pending.Status = "pending"
	svc.db.Save(&pending)

	comments, err := svc.ListByPost("1", 0, "")
	if err != nil {
		t.Fatalf("ListByPost error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 published comment, got %d", len(comments))
	}
	if comments[0].User.Email != "" {
		t.Errorf("expected private email filtered for anonymous viewer")
	}

	// the author themselves sees their own comment privacy-wise; commenter is private for author
	comments, err = svc.ListByPost("1", author.ID, author.Role)
	if err != nil {
		t.Fatalf("ListByPost error: %v", err)
	}
	if comments[0].User.Email != "" {
		t.Errorf("expected email hidden from author too (not self), got %q", comments[0].User.Email)
	}

	// admin sees everything
	admin := seedUser(t, svc.db, "admin", model.RoleSuperAdmin)
	comments, err = svc.ListByPost("1", admin.ID, admin.Role)
	if err != nil {
		t.Fatalf("ListByPost error: %v", err)
	}
	if comments[0].User.Email == "" {
		t.Errorf("expected admin to see private email")
	}
}

func TestDelete_Permissions(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	if _, _, err := svc.Create(post.ID, commenter.ID, "to delete", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	var comment model.Comment
	if err := svc.db.First(&comment).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	id := comment.ID

	// another user cannot delete
	other := seedUser(t, svc.db, "other", model.RoleGuest)
	if _, err := svc.Delete(strconv.FormatUint(uint64(id), 10), other.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}

	// author of the comment can delete
	count, err := svc.Delete(strconv.FormatUint(uint64(id), 10), commenter.ID)
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after delete, got %d", count)
	}

	// not found
	if _, err := svc.Delete(strconv.FormatUint(uint64(id), 10), commenter.ID); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}
}

func TestSetStatus(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	if _, _, err := svc.Create(post.ID, commenter.ID, "moderate me", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	var comment model.Comment
	if err := svc.db.First(&comment).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}

	updated, err := svc.SetStatus(strconv.FormatUint(uint64(comment.ID), 10), "published")
	if err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}
	if updated.Status != "published" {
		t.Errorf("expected published, got %s", updated.Status)
	}

	if _, err := svc.SetStatus("99999", "published"); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}
}

func TestListModeration(t *testing.T) {
	svc, _ := newTestService(t)
	author := seedUser(t, svc.db, "author", model.RoleContributor)
	post := seedPost(t, svc.db, author.ID)
	commenter := seedUser(t, svc.db, "commenter", model.RoleGuest)

	if _, _, err := svc.Create(post.ID, commenter.ID, "one", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if _, _, err := svc.Create(post.ID, commenter.ID, "two", nil); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	list, total, err := svc.ListModeration("published", 1, 1)
	if err != nil {
		t.Fatalf("ListModeration error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 item per page, got %d", len(list))
	}

	list, total, err = svc.ListModeration("pending", 1, 10)
	if err != nil {
		t.Fatalf("ListModeration error: %v", err)
	}
	if total != 0 || len(list) != 0 {
		t.Errorf("expected empty pending queue, got total=%d len=%d", total, len(list))
	}
}

func TestUpdateModerationConfig_PreservesApiKey(t *testing.T) {
	svc, _ := newTestService(t)

	config, err := svc.UpdateModerationConfig(UpdateModerationConfigRequest{
		Enabled: true,
		ApiKey:  "secret-key",
	})
	if err != nil {
		t.Fatalf("UpdateModerationConfig error: %v", err)
	}
	if config.ApiKey != "" {
		t.Errorf("expected api key masked in response")
	}

	// verify stored key
	var stored model.CommentModerationConfig
	if err := svc.db.First(&stored).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if stored.ApiKey != "secret-key" {
		t.Errorf("expected stored api key, got %q", stored.ApiKey)
	}

	// update without api key → preserved
	_, err = svc.UpdateModerationConfig(UpdateModerationConfigRequest{
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateModerationConfig error: %v", err)
	}
	if err := svc.db.First(&stored).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if stored.ApiKey != "secret-key" {
		t.Errorf("expected api key preserved, got %q", stored.ApiKey)
	}
}
