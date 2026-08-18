# 架构

> **原理讲解** —— 本页介绍 VexGo 的设计：后端结构、角色与权限的工作方式、内容审核管线、主题系统和 SSO。这是背景知识——用于理解 VexGo，而不是完成某个具体任务。

## 总览

VexGo 是一个自托管的博客 CMS，由两部分组成：

- **Go 后端**（`backend/`）—— 基于 Gin 和 GORM 构建的 HTTP API，提供管理面板、公开站点和 REST API。支持 SQLite、PostgreSQL 或 MySQL。
- **React 前端**（`frontend/`）—— 基于 TypeScript + Vite + Tailwind CSS 的 SPA，与 API 通信。构建产物嵌入后端二进制。

**主题系统**让后端能够以服务端渲染的方式，用上传的主题渲染公开页面——访客无需 JavaScript 即可阅读内容。

## 后端结构

后端采用领域导向的目录结构，位于 `backend/internal` 下：

```text
backend/
  main.go            # 入口：参数、配置、存储、数据库、路由、静态资源
  internal/
    auth/            # 注册、登录、JWT、个人资料、密码重置
    comment/         # 评论和 AI 内容审核
    config/          # 参数 / 环境变量 / 配置文件解析，JWT、S3、SSO 初始化（纯配置）
    database/        # 连接、自动迁移、种子数据
    home/            # 站点统计
    mailer/          # SMTP 邮件构建与发送
    message/         # 站内通知
    middleware/      # JWT 认证、角色权限、请求日志
    model/           # GORM 数据模型（文章、用户、标签、分类、点赞、评论等）
    post/            # 文章 CRUD、分类、标签、点赞
    public/          # 嵌入的前端、主题、SSR 渲染器、静态路由
    router/          # 路由注册（组合所有领域）
    settings/        # 管理员配置（SMTP、AI、通用、主题）
    sso/             # GitHub / Google / OIDC 登录
    upload/          # 文件上传（本地磁盘或 S3）
    user/            # 用户管理、角色、创作者申请
    verification/    # 邮箱验证和滑块验证码
```

导入使用模块路径 `vexgo/backend/internal/<package>`，例如：

```go
import (
    "vexgo/backend/internal/model"
    "vexgo/backend/internal/post"
    "vexgo/backend/internal/router"
)
```

### 依赖规则

包结构保证了依赖图**无环**：

- **叶子包** —— `config/` 和 `model/` 不导入任何其他后端模块。`model` 被所有领域包导入；`config` 被 `auth`、`database`、`middleware`、`sso` 和 `upload` 导入。
- **共享层** —— `middleware/`（JWT 认证、角色权限、请求日志）只依赖 `config` 和 `model`。
- **跨领域边** —— `auth` 被 `comment`、`post`、`sso` 使用；`auth` 依赖 `verification`；`settings` 依赖 `public`（主题）和 `mailer`（SMTP）；`database` 依赖 `config` 和 `model`。
- **装配** —— `backend/main.go` 是唯一入口：打开数据库、创建存储和 `public.Renderer`，然后通过调用 `router.RegisterAPIRoutes(r, router.Deps{...})` 将各领域组装起来。

这种结构使各包可以独立测试，并防止代码增长时产生循环导入。

## 用户、角色与权限

认证基于 JWT。每个用户只有一个角色；每次请求都会根据数据库中的角色检查权限。

| 角色          | 权限                                             |
| ------------- | ------------------------------------------------ |
| `super_admin` | 一切操作。绕过所有权限检查。不能被其他用户修改。 |
| `admin`       | 审核内容、管理用户和设置、审批创作者申请         |
| `author`      | 直接发布文章                                     |
| `contributor` | 申请角色升级（创作者申请）                       |
| `guest`       | 新注册用户——受限访问                             |

权限检查是累积的：

- **Is admin** = `admin` 或 `super_admin`
- **Is author** = `author` 及以上角色
- **Is contributor** = `contributor` 及以上角色

### 创作者申请

新用户注册为 `guest`。他们可以提交**创作者申请**（附理由）请求升级。管理员审核队列，批准或拒绝每份申请；批准后用户升入更高角色层级。

## 内容审核

VexGo 有两条审核管线——文章和评论各一条。两者都围绕 `status` 字段：

- **文章**：`draft` → `pending` → `published` / `rejected`（被拒文章可重新提交）
- **评论**：`published`、`pending`、`rejected`

### 文章审核

作者发布文章时，可直接进入 `published`（若作者有发布权限），或进入 `pending` 等待管理员审核。管理员批准或拒绝，可附加拒绝原因。

### 评论审核（AI 驱动）

评论审核可在管理面板配置：

- **关键词拦截** —— 包含拦截关键词的评论被扣留或拒绝
- **AI 评分** —— 配置的 LLM（兼容 OpenAI API）根据提示词为每条评论打分；低于阈值的评论被扣留待审
- **自动通过** —— 审核关闭时，评论直接通过

审核配置（提示词、关键词、阈值、模型）存储在数据库中，通过管理面板或 `/moderation` API 端点管理。

## 主题系统

公开页面由服务端渲染。内置的**默认主题**始终可用；管理员可通过管理面板上传 ZIP 格式的主题。

主题包含：

```text
theme.zip
└── theme-id/
    ├── vexgo-theme.json   # 元数据（id、name、author、version 等）
    ├── preview.png        # 可选预览图
    └── dist/              # 构建好的前端资源（index.html、JS、CSS）
```

安装的主题解压到 `data/theme/<id>/`，由渲染器提供。当前主题存储在数据库中，无需重启即可在运行时切换。

## SSO

登录可以委托给外部身份提供商：

- **GitHub** 和 **Google** OAuth
- 任意 **OpenID Connect (OIDC)** 提供商（Keycloak、Authentik、Authelia、Okta、Casdoor 等）

SSO 流程使用授权码模式 + 弹窗；结果写入 `localStorage` 的 `sso_callback_result` 键，打开方页面通过 `storage` 事件获取。当 `allow_local_login` 为 `false` 时，密码登录被完全禁用，SSO 成为唯一入口。

回调地址：

| 提供商 | 回调地址                                      |
| ------ | --------------------------------------------- |
| GitHub | `https://your-domain/api/sso/github/callback` |
| Google | `https://your-domain/api/sso/google/callback` |
| OIDC   | `https://your-domain/api/sso/oidc/callback`   |

`BASE_URL` 必须指向你的公网实例地址，以便正确生成这些重定向。

## 存储

- **上传文件** 默认存到本地数据目录，启用 S3 后存到任意 **S3 兼容对象存储**（AWS S3、MinIO、Garage 等）。
- **元数据**（用户、文章、评论、设置）存储在数据库中——默认 SQLite，生产环境用 PostgreSQL/MySQL。

## 通知

站内通知按用户存储。评论、点赞、回复、文章审核和角色变更等事件都会在接收者的收件箱中创建消息，通过 `/messages` API 暴露。

## 请求流程

一个典型请求的流程：

```text
浏览器 / API 客户端
      │  HTTP
      ▼
Gin 路由器（internal/router）
      │
      ▼
中间件链：日志 → 可选 JWT 认证 → 角色权限检查
      │
      ▼
领域处理器（如 internal/post）→ service → GORM → 数据库
      │
      ▼
JSON 响应（主题页面则为 SSR 渲染的 HTML）
```

JWT 中间件验证令牌并将用户写入 Gin 上下文；权限中间件将数据库中的角色与端点要求的角色进行比对。`super_admin` 始终通过。

## 相关阅读

- [配置参考](/zh-cn/reference/configuration) —— 全部参数、变量和配置键
- [API 参考](/zh-cn/reference/api) —— 该架构暴露的 REST 端点
- [配置指南](/zh-cn/guides/configuration) —— 实操配置方法
