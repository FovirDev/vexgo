# Architecture

> **Explanation** — this page explains how VexGo is designed: the backend layout, how roles and permissions work, the moderation pipeline, theming, and SSO. It's background knowledge — read it to understand VexGo, not to accomplish a specific task.

## Overview

VexGo is a self-hosted blog CMS with two main parts:

- **A Go backend** (`backend/`) — an HTTP API built with Gin and GORM, serving the admin panel, the public site, and the REST API. It can run against SQLite, PostgreSQL, or MySQL.
- **A React frontend** (`frontend/`) — a TypeScript + Vite + Tailwind CSS SPA that talks to the API. Its build output is embedded into the backend binary.

A **theme system** lets the backend server-side-render public pages with uploaded themes, so visitors don't need JavaScript to read content.

## Backend Layout

The backend follows a domain-oriented layout under `backend/internal`:

```text
backend/
  main.go            # entry point: flags, config, storage, DB, router, static routes
  internal/
    auth/            # registration, login, JWT, profile, password reset
    comment/         # comments and AI-powered moderation
    config/          # flag / env / config-file parsing, JWT, S3, SSO setup (pure setup)
    database/        # connection, auto-migration, seeding
    home/            # site statistics
    mailer/          # SMTP mail building and sending
    message/         # in-app notifications
    middleware/      # JWT auth, role-based permissions, request logging
    model/           # GORM data models (post, user, tag, category, like, comment, ...)
    post/            # post CRUD, categories, tags, likes
    public/          # embedded frontend, themes, SSR renderer, static routes
    router/          # route registration (composes every domain)
    settings/        # admin configuration (SMTP, AI, general, theme)
    sso/             # GitHub / Google / OIDC login
    upload/          # file upload (local disk or S3)
    user/            # user management, roles, creator applications
    verification/    # email verification and sliding-puzzle captcha
```

Imports use the module path `vexgo/backend/internal/<package>`, for example:

```go
import (
    "vexgo/backend/internal/model"
    "vexgo/backend/internal/post"
    "vexgo/backend/internal/router"
)
```

### Dependency Rules

The package layout keeps the dependency graph **acyclic**:

- **Leaf packages** — `config/` and `model/` import no other backend module. `model` is imported by every domain package; `config` by `auth`, `database`, `middleware`, `sso`, and `upload`.
- **Shared layer** — `middleware/` (JWT auth, role permissions, request logging) depends only on `config` and `model`.
- **Cross-domain edges** — `auth` is used by `comment`, `post`, and `sso`; `auth` depends on `verification`; `settings` depends on `public` (themes) and `mailer` (SMTP); `database` depends on `config` and `model`.
- **Wiring** — `backend/main.go` is the single entry point: it opens the database, creates storage and the `public.Renderer`, then wires every domain together by calling `router.RegisterAPIRoutes(r, router.Deps{...})`.

This structure keeps packages testable in isolation and prevents circular imports as the codebase grows.

## Users, Roles, and Permissions

Authentication is JWT-based. Each user has exactly one role; permissions are checked against the role in the database on every request.

| Role          | Can do                                                                         |
| ------------- | ------------------------------------------------------------------------------ |
| `super_admin` | Everything. Bypasses all permission checks. Cannot be modified by other users. |
| `admin`       | Moderate content, manage users and settings, approve creator applications      |
| `author`      | Publish posts directly                                                         |
| `contributor` | Apply for a role upgrade (creator application)                                 |
| `guest`       | Newly registered users — limited access                                        |

Privilege checks are cumulative:

- **Is admin** = `admin` or `super_admin`
- **Is author** = `author` + admin roles
- **Is contributor** = `contributor` + higher roles

### Creator Applications

New users register as `guest`. They can submit a **creator application** (with a reason) to request an upgrade. Admins review the queue and approve or reject each application; approving moves the user up a role tier.

## Content Moderation

VexGo has two moderation pipelines — one for posts, one for comments. Both revolve around a `status` field:

- **Posts**: `draft` → `pending` → `published` / `rejected` (rejected posts can be resubmitted)
- **Comments**: `published`, `pending`, `rejected`

### Post moderation

When an author publishes a post, it can go straight to `published` (if the author has publishing rights) or to `pending` for admin review. Admins approve or reject it, optionally attaching a rejection reason.

### Comment moderation (AI-powered)

Comment moderation is configurable from the admin panel:

- **Keyword blocking** — comments containing blocked keywords are held or rejected
- **AI scoring** — a configured LLM (OpenAI-compatible API) scores each comment against a prompt; comments below the score threshold are held for review
- **Auto-approve** — when moderation is disabled, comments pass through immediately

The moderation configuration (prompt, keywords, thresholds, model) lives in the database and is managed via the admin panel or the `/moderation` API endpoints.

## Theme System

Public pages are rendered server-side. The embedded **default theme** is always available; admins can upload additional themes as ZIP archives from the admin panel.

A theme contains:

```text
theme.zip
└── theme-id/
    ├── vexgo-theme.json   # metadata (id, name, author, version, ...)
    ├── preview.png        # optional preview image
    └── dist/              # built frontend assets (index.html, JS, CSS)
```

Installed themes are extracted to `data/theme/<id>/` and served by the renderer. The active theme is stored in the database and can be switched at runtime without restarting the server.

## SSO

Login can be delegated to external identity providers:

- **GitHub** and **Google** OAuth
- Any **OpenID Connect (OIDC)** provider (Keycloak, Authentik, Authelia, Okta, Casdoor, ...)

SSO flows use the authorization-code grant with a popup window; the result is written to `localStorage` under `sso_callback_result` and the opener page picks it up via the `storage` event. When `allow_local_login` is `false`, password login is disabled entirely and SSO is the only way in.

The callback URLs are:

| Provider | Callback URL                                  |
| -------- | --------------------------------------------- |
| GitHub   | `https://your-domain/api/sso/github/callback` |
| Google   | `https://your-domain/api/sso/google/callback` |
| OIDC     | `https://your-domain/api/sso/oidc/callback`   |

`BASE_URL` must point to your public instance URL so these redirects are generated correctly.

## Storage

- **Uploads** go to the local data directory by default, or to any **S3-compatible object storage** (AWS S3, MinIO, Garage, ...) when S3 is enabled.
- **Metadata** (users, posts, comments, settings) lives in the database — SQLite by default, PostgreSQL/MySQL for production.

## Notifications

In-app notifications are stored per user. Events such as comments, likes, replies, post reviews, and role changes create messages in the recipient's inbox, exposed through the `/messages` API.

## Request Flow

A typical request looks like this:

```text
Browser/API client
      │  HTTP
      ▼
Gin router (internal/router)
      │
      ▼
Middleware chain: logger → optional JWT auth → role permission check
      │
      ▼
Domain handler (e.g. internal/post) → service → GORM → database
      │
      ▼
JSON response (or SSR-rendered HTML for theme pages)
```

The JWT middleware validates the token and sets the user in the Gin context; the permission middleware checks the database role against the endpoint's required roles. `super_admin` always passes.

## Related Reading

- [Configuration Reference](/reference/configuration) — every flag, variable, and config key
- [API Reference](/reference/api) — the REST endpoints exposed by this architecture
- [Configuration Guide](/guides/configuration) — practical setup recipes
