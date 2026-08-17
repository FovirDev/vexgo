package handler

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"vexgo/backend/internal/config"
	"vexgo/backend/internal/mailer"
	"vexgo/backend/internal/verification"
	"vexgo/backend/model"

	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Login: issue JWT based on email and password
func Login(c *gin.Context) {
	logrus.Info("User login attempt started")

	var req struct {
		Email        string `json:"email" binding:"required"`
		Password     string `json:"password" binding:"required"`
		CaptchaID    string `json:"captcha_id"`
		CaptchaToken string `json:"captcha_token"`
		CaptchaX     int    `json:"captcha_x"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logrus.WithError(err).Warn("Failed to bind login request JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logrus.WithField("email", req.Email).Debug("Login request parsed successfully")

	// Check if captcha verification is enabled
	captchaEnabled, err := verification.NewService(verification.Deps{DB: db}).IsCaptchaEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check captcha settings"})
		return
	}

	// If captcha verification is enabled, verify captcha
	if captchaEnabled {
		if req.CaptchaID == "" || req.CaptchaToken == "" || req.CaptchaX == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please complete the captcha verification"})
			return
		}
		// Query captcha
		var captcha model.Captcha
		if err := db.Where("id = ? AND token = ?", req.CaptchaID, req.CaptchaToken).First(&captcha).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Captcha does not exist or has expired"})
			return
		}

		// Check if expired
		if time.Now().After(captcha.ExpiresAt) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Captcha has expired"})
			return
		}

		// Verify position (allow certain tolerance)
		tolerance := 10
		if math.Abs(float64(req.CaptchaX-captcha.X)) > float64(tolerance) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Verification failed, please try again"})
			return
		}

		// If captcha has not been used yet, mark it as used
		if !captcha.Used {
			captcha.Used = true
			if err := db.Save(&captcha).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Captcha verification failed"})
				return
			}
		}
		// If captcha already used, pre-verification successful, pass directly
	}

	var user model.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid email or password"})
		return
	}

	// Use bcrypt to compare hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid email or password"})
		return
	}

	// Check if SMTP is enabled, if so verify email status
	mailer := mailer.NewMailer(db)
	enabled, err := mailer.IsEmailEnabled()
	if err == nil && enabled && !user.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"message":        "Please verify your email address first. Check your inbox and click the verification link, or request to resend the verification email.",
			"email_verified": false,
		})
		return
	}

	// Generate token
	claims := jwt.MapClaims{
		"user_id":          user.ID,
		"username":         user.Username,
		"role":             user.Role,
		"password_version": user.PasswordVersion,
		"exp":              time.Now().Add(24 * time.Hour).Unix(),
		"iat":              time.Now().Unix(),                                                  // Timestamp
		"jti":              fmt.Sprintf("%d-%s", user.ID, time.Now().Format(time.RFC3339Nano)), // Unique identifier
	}

	// Update last login time to invalidate old tokens
	user.LastLoginAt = time.Now()
	if err := db.Save(&user).Error; err != nil {
		logrus.WithError(err).Warn("Failed to update last login time")
		// Don't fail the login, just log
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": ss,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
			"avatar":   user.Avatar,
			"bio":      user.Bio,
			"birthday": user.Birthday,
		},
	})
}

// Get current user information
func GetCurrentUser(c *gin.Context) {
	if uid, ok := c.Get("userID"); ok {
		var user model.User
		if err := db.First(&user, uid).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User does not exist"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": user})
		return
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Not logged in"})
}

// Update user profile
func UpdateProfile(c *gin.Context) {
	var req struct {
		Username *string `json:"username"`
		Avatar   *string `json:"avatar"`
		Birthday *string `json:"birthday"`
		Bio      *string `json:"bio"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _ := c.Get("userID")
	userID := uid.(uint)
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User does not exist"})
		return
	}

	// If updating avatar, delete old avatar
	if req.Avatar != nil && *req.Avatar != user.Avatar && user.Avatar != "" {
		// Delete old avatar file
		if err := deleteImageFile(user.Avatar); err != nil {
			// Log error but continue execution to avoid avatar update failure
			fmt.Printf("Failed to delete old avatar %s: %v\n", user.Avatar, err)
		}
		user.Avatar = *req.Avatar
	} else if req.Avatar != nil {
		user.Avatar = *req.Avatar
	}

	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Birthday != nil {
		user.Birthday = *req.Birthday
	}
	if req.Bio != nil {
		user.Bio = *req.Bio
	}
	db.Save(&user)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// Change password
func ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _ := c.Get("userID")
	userID := uid.(uint)
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User does not exist"})
		return
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
		return
	}

	// Increment password version to invalidate old tokens
	user.Password = string(hashed)
	user.PasswordVersion++
	db.Save(&user)
	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

// Update user settings (privacy settings, etc.)
func UpdateSettings(c *gin.Context) {
	var req struct {
		ProfileVisibility *string `json:"profile_visibility"`
		HideEmail         *bool   `json:"hide_email"`
		HideBirthday      *bool   `json:"hide_birthday"`
		HideBio           *bool   `json:"hide_bio"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _ := c.Get("userID")
	userID := uid.(uint)
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User does not exist"})
		return
	}

	if req.ProfileVisibility != nil {
		user.ProfileVisibility = *req.ProfileVisibility
	}
	if req.HideEmail != nil {
		user.HideEmail = *req.HideEmail
	}
	if req.HideBirthday != nil {
		user.HideBirthday = *req.HideBirthday
	}
	if req.HideBio != nil {
		user.HideBio = *req.HideBio
	}

	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Settings updated successfully",
		"user":    user,
	})
}

// Update email
func UpdateEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _ := c.Get("userID")
	userID := uid.(uint)
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User does not exist"})
		return
	}

	// Check if new email is the same as current email
	if req.Email == user.Email {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New email cannot be the same as current email"})
		return
	}

	// Check if new email is already used by another user
	var existingUser model.User
	if err := db.Where("email = ? AND id != ?", req.Email, userID).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This email is already used by another user"})
		return
	}

	// Check if SMTP is enabled
	mailer := mailer.NewMailer(db)
	enabled, err := mailer.IsEmailEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check mail configuration"})
		return
	}

	if enabled {
		// If SMTP enabled, generate email change verification token and send confirmation email
		token, err := mailer.GenerateEmailChangeToken(userID, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate verification token"})
			return
		}

		// Build verification link
		protocol := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			protocol = "https"
		}
		host := c.Request.Host
		verificationLink := fmt.Sprintf("%s://%s/verify-email?token=%s", protocol, host, token)

		// Send confirmation email
		if err := mailer.SendEmailChangeEmail(user.Email, user.Username, req.Email, verificationLink); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Verification email sent. Please check your inbox and click the link to complete email change.",
			"pending": true,
		})
	} else {
		// If SMTP not enabled, update email directly
		if err := db.Model(&user).Update("email", req.Email).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update email"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Email updated successfully",
			"pending": false,
			"user": gin.H{
				"email": req.Email,
			},
		})
	}
}

// Request password reset
func RequestPasswordReset(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user
	var user model.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// For security reasons, return success even if user doesn't exist to avoid information leakage
		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, reset link has been sent"})
		return
	}

	// Check if SMTP is enabled
	mailer := mailer.NewMailer(db)
	enabled, err := mailer.IsEmailEnabled()
	if err != nil || !enabled {
		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, reset link has been sent"})
		return
	}

	// Generate password reset token
	token, err := mailer.GeneratePasswordResetToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate reset token"})
		return
	}

	// Build reset link - use request protocol and hostname
	protocol := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		protocol = "https"
	}
	host := c.Request.Host
	resetLink := fmt.Sprintf("%s://%s/reset-password?token=%s", protocol, host, token)

	// Send email
	if err := mailer.SendPasswordResetEmail(user.Email, user.Username, resetLink); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "If the email exists, reset link has been sent"})
}

// Reset password
func ResetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user with this token
	var user model.User
	if err := db.Where("verification_token = ?", req.Token).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reset token"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed"})
		return
	}

	// Check if token has expired
	if user.TokenExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reset token has expired"})
		return
	}

	// Generate hash for new password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
		return
	}

	// Update password and clear reset token
	if err := db.Model(&user).Updates(map[string]interface{}{
		"password":           string(hashed),
		"verification_token": "",
		"token_expires_at":   time.Time{},
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}
