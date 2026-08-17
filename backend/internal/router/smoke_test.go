package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vexgo/backend/internal/auth"
	"vexgo/backend/internal/comment"
	"vexgo/backend/internal/config"
	"vexgo/backend/internal/database"
	"vexgo/backend/internal/home"
	"vexgo/backend/internal/message"
	"vexgo/backend/internal/post"
	"vexgo/backend/internal/public"
	"vexgo/backend/internal/router"
	"vexgo/backend/internal/settings"
	"vexgo/backend/internal/sso"
	"vexgo/backend/internal/upload"
	"vexgo/backend/internal/user"
	"vexgo/backend/internal/verification"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestSmoke boots the entire application (mirroring main.go wiring) against an
// in-memory SQLite database and walks the core product journeys end to end:
// login, post lifecycle, likes/comments, profile & site settings, role
// promotion and the moderation queues.
func TestSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.Init("smoke-test-secret")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	database.AutoMigrate(db)
	database.Seed(db)

	dataDir := t.TempDir()
	storage := upload.NewLocalStorage(dataDir)
	renderer := public.NewRenderer(db, "http://localhost:3001", dataDir)

	r := gin.New()
	router.RegisterAPIRoutes(r, router.Deps{
		DB:      db,
		Message: message.Deps{DB: db},
		Comment: comment.Deps{DB: db, Notifier: message.NewService(message.Deps{DB: db})},
		Post: post.Deps{
			DB:       db,
			Notifier: message.NewService(message.Deps{DB: db}),
			Files:    storage,
		},
		Upload: upload.Deps{DB: db, Storage: storage},
		User: user.Deps{
			DB:       db,
			Notifier: message.NewService(message.Deps{DB: db}),
			Files:    storage,
		},
		Verification: verification.Deps{DB: db},
		Auth:         auth.Deps{DB: db, JWTSecret: config.JWTSecret, Files: storage},
		SSO:          sso.Deps{DB: db, SSO: &config.SSO, JWTSecret: config.JWTSecret},
		Home:         home.Deps{DB: db},
		Settings:     settings.Deps{DB: db, Themes: renderer},
	})
	renderer.RegisterStaticRoutes(r, false)

	// do performs a request and returns status, parsed JSON and raw body.
	do := func(method, url, accept, token string, body any) (int, map[string]any, string) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, url, &buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", accept)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var parsed map[string]any
		if json.Unmarshal(w.Body.Bytes(), &parsed) == nil {
			return w.Code, parsed, w.Body.String()
		}
		return w.Code, nil, w.Body.String()
	}

	// must performs a request and fails the test unless the status matches.
	must := func(method, url, accept, token string, body any, wantStatus int) map[string]any {
		t.Helper()
		code, parsed, raw := do(method, url, accept, token, body)
		if code != wantStatus {
			t.Fatalf("%s %s = %d, want %d: %s", method, url, code, wantStatus, raw)
		}
		return parsed
	}

	login := func(email, password string) string {
		t.Helper()
		parsed := must(http.MethodPost, "/api/auth/login", "application/json", "", map[string]string{
			"email": email, "password": password,
		}, http.StatusOK)
		token, _ := parsed["token"].(string)
		if token == "" {
			t.Fatal("login response has no token")
		}
		return token
	}

	createPost := func(token, status string) map[string]any {
		t.Helper()
		parsed := must(http.MethodPost, "/api/posts", "application/json", token, map[string]any{
			"title": "Smoke post", "content": "Smoke content", "category": "Default",
			"status": status, "tags": []string{"smoke"},
		}, http.StatusCreated)
		p, _ := parsed["post"].(map[string]any)
		if p == nil {
			t.Fatalf("create post: no post in response: %v", parsed)
		}
		return p
	}

	getPost := func(token string, id any) map[string]any {
		t.Helper()
		parsed := must(http.MethodGet, fmt.Sprintf("/api/posts/%v", id), "application/json", token, nil, http.StatusOK)
		p, _ := parsed["post"].(map[string]any)
		if p == nil {
			t.Fatalf("get post %v: no post in response", id)
		}
		return p
	}

	listIDs := func(token, url string) map[float64]bool {
		t.Helper()
		parsed := must(http.MethodGet, url, "application/json", token, nil, http.StatusOK)
		ids := map[float64]bool{}
		if arr, ok := parsed["posts"].([]any); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]any); ok {
					if id, ok := m["id"].(float64); ok {
						ids[id] = true
					}
				}
			}
		}
		return ids
	}

	assertIn := func(ids map[float64]bool, id float64, msg string) {
		t.Helper()
		if !ids[id] {
			t.Fatalf("%s: post %v not in list %v", msg, id, ids)
		}
	}
	assertNotIn := func(ids map[float64]bool, id float64, msg string) {
		t.Helper()
		if ids[id] {
			t.Fatalf("%s: post %v unexpectedly in list %v", msg, id, ids)
		}
	}

	// 1. Admin login
	t.Run("admin login", func(t *testing.T) {
		login("admin@example.com", "password")
	})
	admin := login("admin@example.com", "password")

	// 2. Save a post as draft, 3. publish it
	var adminPostID float64
	t.Run("create draft and publish", func(t *testing.T) {
		draft := createPost(admin, "draft")
		adminPostID = draft["id"].(float64)
		if draft["status"] != "draft" {
			t.Fatalf("expected draft, got %v", draft["status"])
		}
		ids := listIDs(admin, "/api/posts/drafts")
		assertIn(ids, adminPostID, "draft should be listed in /api/posts/drafts")

		published := must(http.MethodPut, fmt.Sprintf("/api/posts/%v", adminPostID), "application/json",
			admin, map[string]any{"status": "published"}, http.StatusOK)
		if p, _ := published["post"].(map[string]any); p["status"] != "published" {
			t.Fatalf("expected published, got %v", published)
		}
	})

	// 4. Viewing the post works (API + server-side rendered page)
	var viewsBefore float64
	t.Run("view post renders", func(t *testing.T) {
		p := getPost("", adminPostID)
		viewsBefore = p["viewCount"].(float64)
		if p["content"] != "Smoke content" {
			t.Fatalf("post content mismatch: %v", p["content"])
		}
		_, parsed, raw := do(http.MethodGet, fmt.Sprintf("/posts/%v", adminPostID), "text/html", "", nil)
		if !strings.Contains(raw, "Smoke post") {
			t.Fatalf("SSR page does not contain title, got %q", parsed)
		}
	})

	// 5. Comment on the post
	t.Run("comment on post", func(t *testing.T) {
		parsed := must(http.MethodPost, "/api/comments", "application/json", admin, map[string]any{
			"postId": adminPostID, "content": "Great post!",
		}, http.StatusCreated)
		c, _ := parsed["comment"].(map[string]any)
		if c["status"] != "published" {
			t.Fatalf("comment not auto-published: %v", parsed)
		}
	})

	// 6. Like the post
	t.Run("like post", func(t *testing.T) {
		parsed := must(http.MethodPost, fmt.Sprintf("/api/likes/%v", adminPostID), "application/json",
			admin, nil, http.StatusOK)
		if parsed["isLiked"] != true || parsed["likesCount"] != float64(1) {
			t.Fatalf("unexpected like result: %v", parsed)
		}
	})

	// 7. View count, comment count and like state all increased
	t.Run("counts increase", func(t *testing.T) {
		p := getPost(admin, adminPostID)
		if p["viewCount"].(float64) <= viewsBefore {
			t.Fatalf("viewCount did not increase: before=%v after=%v", viewsBefore, p["viewCount"])
		}
		if p["commentsCount"].(float64) < 1 {
			t.Fatalf("commentsCount not increased: %v", p["commentsCount"])
		}
		if p["likesCount"].(float64) != 1 || p["isLiked"] != true {
			t.Fatalf("like state mismatch: %v", p)
		}
	})

	// 8. Profile: every field is settable and comes back via /me
	t.Run("profile update and display", func(t *testing.T) {
		must(http.MethodPut, "/api/auth/profile", "application/json", admin, map[string]any{
			"username": "admin-x", "avatar": "/uploads/avatar.png",
			"birthday": "1990-01-01", "bio": "Hello world",
		}, http.StatusOK)
		me := must(http.MethodGet, "/api/auth/me", "application/json", admin, nil, http.StatusOK)
		u, _ := me["user"].(map[string]any)
		if u["username"] != "admin-x" || u["avatar"] != "/uploads/avatar.png" ||
			u["birthday"] != "1990-01-01" || u["bio"] != "Hello world" {
			t.Fatalf("profile fields not persisted: %v", u)
		}
	})

	// 9. User settings (privacy) take effect
	t.Run("user settings apply", func(t *testing.T) {
		must(http.MethodPut, "/api/auth/settings", "application/json", admin, map[string]any{
			"profile_visibility": "private", "hide_email": true, "hide_birthday": true, "hide_bio": true,
		}, http.StatusOK)
		me := must(http.MethodGet, "/api/auth/me", "application/json", admin, nil, http.StatusOK)
		u, _ := me["user"].(map[string]any)
		if u["profile_visibility"] != "private" || u["hide_email"] != true {
			t.Fatalf("privacy settings not persisted: %v", u)
		}
	})

	// 10. Admin general settings round-trip
	t.Run("general settings", func(t *testing.T) {
		must(http.MethodPut, "/api/config/general", "application/json", admin, map[string]any{
			"captchaEnabled": false, "registrationEnabled": true, "allowGuestViewPosts": true,
			"siteName": "SmokeSite", "siteDescription": "smoke desc", "siteIcon": "/icon.png",
			"itemsPerPage": 5,
		}, http.StatusOK)
		cfg := must(http.MethodGet, "/api/config/general", "application/json", admin, nil, http.StatusOK)
		if cfg["siteName"] != "SmokeSite" || cfg["siteDescription"] != "smoke desc" ||
			cfg["itemsPerPage"] != float64(5) || cfg["allowGuestViewPosts"] != true {
			t.Fatalf("general settings not persisted: %v", cfg)
		}
	})

	// 11. New registered guest: can only view posts
	var aliceToken string
	var aliceID float64
	t.Run("guest registration and read-only", func(t *testing.T) {
		reg := must(http.MethodPost, "/api/auth/register", "application/json", "", map[string]string{
			"email": "alice@example.com", "password": "alice-pass", "username": "alice",
		}, http.StatusCreated)
		u, _ := reg["user"].(map[string]any)
		aliceID = u["id"].(float64)
		aliceToken = login("alice@example.com", "alice-pass")

		ids := listIDs(aliceToken, "/api/posts")
		assertIn(ids, adminPostID, "guest should see published posts")
		must(http.MethodPost, "/api/posts", "application/json", aliceToken, map[string]any{
			"title": "nope", "content": "nope", "category": "Default",
		}, http.StatusForbidden)
	})

	// 12. Promote guest to contributor: posts need moderation
	var pendingID float64
	t.Run("contributor post requires moderation", func(t *testing.T) {
		must(http.MethodPut, fmt.Sprintf("/api/users/%v/role", aliceID), "application/json",
			admin, map[string]any{"role": "contributor"}, http.StatusOK)

		p := createPost(aliceToken, "")
		pendingID = p["id"].(float64)
		if p["status"] != "pending" {
			t.Fatalf("contributor post should be pending, got %v", p["status"])
		}

		// Invisible to everyone except the author, admins and the moderation queue
		assertNotIn(listIDs("", "/api/posts"), pendingID, "anonymous must not see pending post")
		assertIn(listIDs(aliceToken, "/api/posts"), pendingID, "author must see own pending post")
		assertIn(listIDs(admin, "/api/posts"), pendingID, "admin must see pending post")
		queue := must(http.MethodGet, "/api/moderation/pending", "application/json", admin, nil, http.StatusOK)
		if ids := listIDs(admin, "/api/moderation/pending"); !ids[pendingID] {
			t.Fatalf("pending post not in moderation queue: %v", queue)
		}

		// Super admin approves -> visible to everyone
		must(http.MethodPut, fmt.Sprintf("/api/moderation/approve/%v", pendingID), "application/json",
			admin, nil, http.StatusOK)
		assertIn(listIDs("", "/api/posts"), pendingID, "approved post must be public")
	})

	// 13. Super admin can reject a post
	var rejectedID float64
	t.Run("admin rejects post", func(t *testing.T) {
		rejected := createPost(aliceToken, "")
		rejectedID = rejected["id"].(float64)

		parsed := must(http.MethodPut, fmt.Sprintf("/api/moderation/reject/%v", rejectedID), "application/json",
			admin, map[string]any{"rejectionReason": "too short"}, http.StatusOK)
		p, _ := parsed["post"].(map[string]any)
		if p["status"] != "rejected" || p["rejectionReason"] != "too short" {
			t.Fatalf("reject result mismatch: %v", parsed)
		}
		assertNotIn(listIDs("", "/api/posts"), rejectedID, "rejected post must stay invisible")
		rejectedList := must(http.MethodGet, "/api/moderation/rejected", "application/json", admin, nil, http.StatusOK)
		if ids := listIDs(admin, "/api/moderation/rejected"); !ids[rejectedID] {
			t.Fatalf("rejected post not in rejected queue: %v", rejectedList)
		}
	})

	// 14. Contributor comment goes through the comment moderation queue
	var approvedCommentID float64
	t.Run("comment moderation", func(t *testing.T) {
		// Disable auto-approve so new comments land in the pending queue
		must(http.MethodPut, "/api/moderation/comments/config", "application/json", admin, map[string]any{
			"enabled": false, "autoApproveEnabled": false,
		}, http.StatusOK)

		parsed := must(http.MethodPost, "/api/comments", "application/json", aliceToken, map[string]any{
			"postId": adminPostID, "content": "needs review",
		}, http.StatusCreated)
		c, _ := parsed["comment"].(map[string]any)
		if c["status"] != "pending" {
			t.Fatalf("comment should be pending, got %v", parsed)
		}
		approvedCommentID = c["id"].(float64)

		pending := must(http.MethodGet, "/api/moderation/comments/pending", "application/json", admin, nil, http.StatusOK)
		found := false
		if arr, ok := pending["comments"].([]any); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]any); ok && m["id"] == approvedCommentID {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("pending comment not in moderation queue: %v", pending)
		}

		// Not visible until approved
		comments := must(http.MethodGet, fmt.Sprintf("/api/comments/post/%v", adminPostID), "application/json", "", nil, http.StatusOK)
		if len(comments["comments"].([]any)) != 1 {
			t.Fatalf("pending comment must not be public yet: %v", comments)
		}

		must(http.MethodPut, fmt.Sprintf("/api/moderation/comments/approve/%v", approvedCommentID), "application/json",
			admin, nil, http.StatusOK)
		comments = must(http.MethodGet, fmt.Sprintf("/api/comments/post/%v", adminPostID), "application/json", "", nil, http.StatusOK)
		if len(comments["comments"].([]any)) != 2 {
			t.Fatalf("approved comment should be visible: %v", comments)
		}

		// A second comment gets rejected
		parsed = must(http.MethodPost, "/api/comments", "application/json", aliceToken, map[string]any{
			"postId": adminPostID, "content": "reject me",
		}, http.StatusCreated)
		c, _ = parsed["comment"].(map[string]any)
		must(http.MethodPut, fmt.Sprintf("/api/moderation/comments/reject/%v", c["id"].(float64)), "application/json",
			admin, nil, http.StatusOK)
		comments = must(http.MethodGet, fmt.Sprintf("/api/comments/post/%v", adminPostID), "application/json", "", nil, http.StatusOK)
		if len(comments["comments"].([]any)) != 2 {
			t.Fatalf("rejected comment must stay invisible: %v", comments)
		}
	})

	// 15. Promote contributor to author: publishes directly without moderation
	t.Run("author publishes without moderation", func(t *testing.T) {
		must(http.MethodPut, fmt.Sprintf("/api/users/%v/role", aliceID), "application/json",
			admin, map[string]any{"role": "author"}, http.StatusOK)

		p := createPost(aliceToken, "")
		if p["status"] != "published" {
			t.Fatalf("author post should be published directly, got %v", p["status"])
		}
		assertIn(listIDs("", "/api/posts"), p["id"].(float64), "author post must be public immediately")
	})
}
