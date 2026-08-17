package main

import (
	"fmt"
	"strings"

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
	"github.com/sirupsen/logrus"
)

func main() {
	// 1. Parse command line arguments
	cfg := config.ParseFlags()

	// 2. Setup logging
	setupLogging(cfg.LogLevel)

	// 3. Initialize configuration (load JWT secret, etc., support config files and environment variables)
	config.Init(cfg.JWTSecret)

	// 3.1 Load SSO configuration from config file (overrides environment variables)
	config.LoadFromConfig(cfg)

	// 4. Initialize file storage: local disk by default, S3-compatible when enabled
	var storage upload.Storage = upload.NewLocalStorage(cfg.DataDir)
	if cfg.S3Enabled {
		s3Cfg := &config.S3Config{
			Enabled:                  cfg.S3Enabled,
			Endpoint:                 cfg.S3Endpoint,
			Region:                   cfg.S3Region,
			Bucket:                   cfg.S3Bucket,
			AccessKey:                cfg.S3AccessKey,
			SecretKey:                cfg.S3SecretKey,
			ForcePath:                cfg.S3ForcePath,
			CustomDomain:             cfg.S3CustomDomain,
			DisableBucketInCustomURL: cfg.S3DisableBucketInCustomURL,
		}
		logrus.WithFields(logrus.Fields{
			"enabled":                  s3Cfg.Enabled,
			"endpoint":                 s3Cfg.Endpoint,
			"region":                   s3Cfg.Region,
			"bucket":                   s3Cfg.Bucket,
			"customDomain":             s3Cfg.CustomDomain,
			"disableBucketInCustomURL": s3Cfg.DisableBucketInCustomURL,
		}).Info("S3 Config Loaded")
		if s3Storage, err := upload.NewS3Storage(s3Cfg); err != nil {
			logrus.WithError(err).Fatal("Failed to initialize S3 storage")
		} else if s3Storage != nil {
			storage = s3Storage
		}
		logrus.Info("S3 storage initialized")
	} else {
		logrus.Info("Using local file storage")
	}

	// 5. Initialize database connection (ensure database driver and connection string are configured correctly)
	db := database.Open(cfg, cfg.DataDir)
	database.AutoMigrate(db)
	database.Seed(db)

	// 6. Create Gin engine instance (includes Logger and Recovery middleware by default)
	r := gin.Default()

	// 6.1 Create the SSR renderer with the injected database, base URL and data dir
	renderer := public.NewRenderer(db, fmt.Sprintf("http://%s", cfg.GetListenAddr()), cfg.DataDir)
	logrus.WithField("baseURL", renderer.BaseURL()).Info("Base URL set for server-side rendering")

	// Configure trusted proxies based on environment/configuration
	// If BEHIND_REVERSE_PROXY=true, use TRUSTED_PROXIES list or common defaults
	// If BEHIND_REVERSE_PROXY=false, disable proxy trust (no warning)
	if cfg.BehindReverseProxy {
		if len(cfg.TrustedProxies) > 0 {
			// Use explicitly configured trusted proxies
			if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
				logrus.WithError(err).Fatal("Invalid trusted proxies configuration")
			}
			logrus.WithField("proxies", cfg.TrustedProxies).Info("Trusted proxies configured")
		} else {
			// Use common defaults: trust all private IP ranges and localhost
			// This is a reasonable default for self-hosted behind reverse proxy
			defaultProxies := []string{
				"127.0.0.1",
				"::1",
				"192.168.0.0/16",
				"10.0.0.0/8",
				"172.16.0.0/12",
			}
			if err := r.SetTrustedProxies(defaultProxies); err != nil {
				logrus.WithError(err).Fatal("Invalid default trusted proxies configuration")
			}
			logrus.WithField("proxies", defaultProxies).Info("Trusted proxies set to common private networks (behind reverse proxy)")
		}
	} else {
		// Not behind a reverse proxy, disable trust
		if err := r.SetTrustedProxies(nil); err != nil {
			logrus.WithError(err).Fatal("Failed to disable trusted proxies")
		}
		logrus.Info("No trusted proxies configured (not behind reverse proxy)")
	}

	// ===================== Core API routing group (all endpoints under /api) =====================
	// All API routing definitions have been moved to router.RegisterAPIRoutes to avoid cluttering main.go.
	router.RegisterAPIRoutes(r, router.Deps{
		DB:      db,
		Message: message.Deps{DB: db},
		Comment: comment.Deps{
			DB:       db,
			Notifier: message.NewService(message.Deps{DB: db}),
		},
		Post: post.Deps{
			DB:       db,
			Notifier: message.NewService(message.Deps{DB: db}),
			Files:    storage,
		},
		Upload: upload.Deps{
			DB:      db,
			Storage: storage,
		},
		User: user.Deps{
			DB:       db,
			Notifier: message.NewService(message.Deps{DB: db}),
			Files:    storage,
		},
		Verification: verification.Deps{
			DB: db,
		},
		Auth: auth.Deps{
			DB:        db,
			JWTSecret: config.JWTSecret,
			Files:     storage,
		},
		SSO: sso.Deps{
			DB:        db,
			SSO:       &config.SSO,
			JWTSecret: config.JWTSecret,
		},
		Home: home.Deps{
			DB: db,
		},
		Settings: settings.Deps{
			DB:     db,
			Themes: renderer,
		},
	})

	// ===================== Static file hosting =====================
	// Register all static routes (assets, uploads, SPA fallback) via the renderer
	renderer.RegisterStaticRoutes(r, cfg.S3Enabled)

	// 7. Start the server
	logrus.WithField("address", cfg.GetListenAddr()).Info("Starting server")
	if err := r.Run(cfg.GetListenAddr()); err != nil {
		logrus.WithError(err).Fatal("Failed to start server")
	}
}

// setupLogging configures the logging level based on the provided string
func setupLogging(levelStr string) {
	level, err := logrus.ParseLevel(strings.ToLower(levelStr))
	if err != nil {
		logrus.Warnf("Invalid log level '%s', defaulting to 'info'", levelStr)
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.Infof("Log level set to: %s", level)
}
