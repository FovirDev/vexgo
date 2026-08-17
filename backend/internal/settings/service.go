// Package settings implements the admin configuration domain: SMTP, AI,
// general site settings and theme configuration.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vexgo/backend/model"
	"vexgo/backend/public"

	"gorm.io/gorm"
)

// Sentinel errors mapped to HTTP responses by the handler. Each error carries
// the exact message of the original handler response it replaces.
var (
	// SMTP config errors.
	ErrSMTPNotConfigured = errors.New("Please configure SMTP settings first")
	ErrSMTPDisabled      = errors.New("SMTP is not enabled, please enable and save configuration first")
	ErrSMTPIncomplete    = errors.New("Please fill in all SMTP configuration fields")
	ErrSMTPNoRecipient   = errors.New("Please fill in test email address first")

	// AI config errors.
	ErrAINotConfigured    = errors.New("Please configure AI settings first")
	ErrAIDisabled         = errors.New("AI is not enabled, please enable and save configuration first")
	ErrAIIncomplete       = errors.New("Please fill in all AI configuration fields (endpoint, API key, model name)")
	ErrAIIncompleteModels = errors.New("Please fill in all AI configuration fields (endpoint, API key)")

	// Theme errors.
	ErrThemeNotFound       = errors.New("Theme not found")
	ErrPreviewNotSpecified = errors.New("Preview not specified")
	ErrPreviewNotFound     = errors.New("Preview image not found")
)

// Deps holds the dependencies required by the settings domain.
type Deps struct {
	DB *gorm.DB
}

// Service contains the business logic of the settings domain.
type Service struct {
	db *gorm.DB
}

// NewService creates a settings service with the given dependencies.
func NewService(deps Deps) *Service {
	return &Service{db: deps.DB}
}

// SMTPConfigRequest carries the fields accepted when updating the SMTP config.
type SMTPConfigRequest struct {
	Enabled   bool
	Host      string
	Port      int
	Username  string
	Password  string // if empty, don't update password
	FromEmail string
	FromName  string
	TestEmail string
}

// GetSMTPConfig returns the stored SMTP configuration with the password
// masked, or the default configuration when no row exists.
func (s *Service) GetSMTPConfig() (model.SMTPConfig, error) {
	var config model.SMTPConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return default configuration
			return model.SMTPConfig{
				Enabled:   false,
				Host:      "",
				Port:      587,
				Username:  "",
				Password:  "",
				FromEmail: "",
				FromName:  "VexGo",
			}, nil
		}
		return config, err
	}

	// Don't return password field
	config.Password = ""
	return config, nil
}

// UpdateSMTPConfig creates or updates the SMTP configuration and returns it
// with the password masked.
func (s *Service) UpdateSMTPConfig(req SMTPConfigRequest) (model.SMTPConfig, error) {
	var config model.SMTPConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new configuration
			config = model.SMTPConfig{
				Enabled:   req.Enabled,
				Host:      req.Host,
				Port:      req.Port,
				Username:  req.Username,
				Password:  req.Password,
				FromEmail: req.FromEmail,
				FromName:  req.FromName,
				TestEmail: req.TestEmail,
			}
			if err := s.db.Create(&config).Error; err != nil {
				return config, fmt.Errorf("Failed to create SMTP config: %w", err)
			}
		} else {
			return config, fmt.Errorf("Failed to get SMTP config: %w", err)
		}
	} else {
		// Update existing configuration
		config.Enabled = req.Enabled
		config.Host = req.Host
		config.Port = req.Port
		config.Username = req.Username
		config.FromEmail = req.FromEmail
		config.FromName = req.FromName
		config.TestEmail = req.TestEmail

		// Only update password if new password is provided
		if req.Password != "" {
			config.Password = req.Password
		}

		if err := s.db.Save(&config).Error; err != nil {
			return config, fmt.Errorf("Failed to update SMTP config: %w", err)
		}
	}

	// Return configuration without password
	config.Password = ""
	return config, nil
}

// TestSMTP sends a test email to the configured test address (or the admin's
// email as fallback) using the stored SMTP configuration.
func (s *Service) TestSMTP(adminEmail string) (string, error) {
	var config model.SMTPConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", ErrSMTPNotConfigured
		}
		return "", err
	}

	// Check if enabled
	if !config.Enabled {
		return "", ErrSMTPDisabled
	}

	// Check required fields
	if config.Host == "" || config.Port == 0 || config.Username == "" || config.Password == "" || config.FromEmail == "" {
		return "", ErrSMTPIncomplete
	}

	// Get recipient email: use configured test email first, otherwise use current admin email
	var recipientEmail string
	if config.TestEmail != "" {
		recipientEmail = config.TestEmail
	} else {
		recipientEmail = adminEmail
	}
	if recipientEmail == "" {
		return "", ErrSMTPNoRecipient
	}

	// Send test email
	subject := "SMTP Configuration Test Email"
	textBody := fmt.Sprintf(`
Dear %s,

This is a test email to verify your SMTP configuration is working correctly.

If you receive this email, it means your SMTP configuration is successful!

Configuration details:
- SMTP Server: %s:%d
- Sender: %s <%s>

Time: %s
	`, config.FromName, config.Host, config.Port, config.FromName, config.FromEmail, time.Now().Format("2006-01-02 15:04:05"))

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; background-color: #f9f9f9; }
        .success { color: #4CAF50; font-weight: bold; }
        .info { background-color: #e3f2fd; padding: 15px; border-radius: 4px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>SMTP Configuration Test</h1>
        </div>
        <div class="content">
            <p>Dear %s,</p>
            <p class="success">✓ Test email sent successfully!</p>
            <p>Your SMTP configuration is working correctly. You can now use email verification features.</p>

            <div class="info">
                <strong>Configuration details:</strong><br>
                SMTP Server: %s:%d<br>
                Sender: %s <%s>
            </div>

            <p>Time: %s</p>
        </div>
    </div>
</body>
</html>
	`, config.FromName, config.Host, config.Port, config.FromName, config.FromEmail, time.Now().Format("2006-01-02 15:04:05"))

	// Build email message
	from := fmt.Sprintf("%s <%s>", config.FromName, config.FromEmail)
	to := recipientEmail

	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "multipart/alternative; boundary=\"boundary\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n"
	message += "--boundary\r\n"
	message += "Content-Type: text/plain; charset=UTF-8\r\n\r\n"
	message += strings.TrimSpace(textBody) + "\r\n\r\n"
	message += "--boundary\r\n"
	message += "Content-Type: text/html; charset=UTF-8\r\n\r\n"
	message += strings.TrimSpace(htmlBody) + "\r\n\r\n"
	message += "--boundary--\r\n"

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)

	if err := smtp.SendMail(addr, auth, config.FromEmail, []string{to}, []byte(message)); err != nil {
		return "", fmt.Errorf("Failed to send test email: %w", err)
	}

	return recipientEmail, nil
}

// GeneralSettingsRequest carries the fields accepted when updating the
// general settings.
type GeneralSettingsRequest struct {
	CaptchaEnabled      bool
	RegistrationEnabled bool
	AllowGuestViewPosts bool
	SiteName            string
	SiteDescription     string
	SiteIcon            string
	ItemsPerPage        int
}

// GetGeneralSettings returns the stored general settings, or the defaults
// when no row exists.
func (s *Service) GetGeneralSettings() (model.GeneralSettings, error) {
	var config model.GeneralSettings
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return default configuration
			return model.GeneralSettings{
				CaptchaEnabled:      false,
				RegistrationEnabled: true,
				AllowGuestViewPosts: true,
				SiteName:            "VexGo",
				SiteDescription:     "",
				SiteIcon:            "",
				ItemsPerPage:        20,
			}, nil
		}
		return config, err
	}
	return config, nil
}

// UpdateGeneralSettings creates or updates the general settings.
func (s *Service) UpdateGeneralSettings(req GeneralSettingsRequest) (model.GeneralSettings, error) {
	var config model.GeneralSettings
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new configuration
			config = model.GeneralSettings{
				CaptchaEnabled:      req.CaptchaEnabled,
				RegistrationEnabled: req.RegistrationEnabled,
				AllowGuestViewPosts: req.AllowGuestViewPosts,
				SiteName:            req.SiteName,
				SiteDescription:     req.SiteDescription,
				SiteIcon:            req.SiteIcon,
				ItemsPerPage:        req.ItemsPerPage,
			}
			if err := s.db.Create(&config).Error; err != nil {
				return config, fmt.Errorf("Failed to create general settings: %w", err)
			}
		} else {
			return config, fmt.Errorf("Failed to get general settings: %w", err)
		}
	} else {
		// Update existing configuration
		config.CaptchaEnabled = req.CaptchaEnabled
		config.RegistrationEnabled = req.RegistrationEnabled
		config.AllowGuestViewPosts = req.AllowGuestViewPosts
		config.SiteName = req.SiteName
		config.SiteDescription = req.SiteDescription
		config.SiteIcon = req.SiteIcon
		config.ItemsPerPage = req.ItemsPerPage

		if err := s.db.Save(&config).Error; err != nil {
			return config, fmt.Errorf("Failed to update general settings: %w", err)
		}
	}

	return config, nil
}

// AIConfigRequest carries the fields accepted when updating the AI config.
type AIConfigRequest struct {
	Enabled     bool
	Provider    string
	ApiEndpoint string
	ApiKey      string // if empty, don't update API key
	ModelName   string
}

// GetAIConfig returns the stored AI configuration with the API key masked,
// or the defaults when no row exists.
func (s *Service) GetAIConfig() (model.AIConfig, error) {
	var config model.AIConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return default configuration
			return model.AIConfig{
				Enabled:     false,
				Provider:    "openai",
				ApiEndpoint: "",
				ApiKey:      "",
				ModelName:   "gpt-3.5-turbo",
			}, nil
		}
		return config, err
	}

	// Don't return API key field
	config.ApiKey = ""
	return config, nil
}

// UpdateAIConfig creates or updates the AI configuration and returns it with
// the API key masked.
func (s *Service) UpdateAIConfig(req AIConfigRequest) (model.AIConfig, error) {
	var config model.AIConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new configuration
			config = model.AIConfig{
				Enabled:     req.Enabled,
				Provider:    req.Provider,
				ApiEndpoint: req.ApiEndpoint,
				ApiKey:      req.ApiKey,
				ModelName:   req.ModelName,
			}
			if err := s.db.Create(&config).Error; err != nil {
				return config, fmt.Errorf("Failed to create AI config: %w", err)
			}
		} else {
			return config, fmt.Errorf("Failed to get AI config: %w", err)
		}
	} else {
		// Update existing configuration
		config.Enabled = req.Enabled
		config.Provider = req.Provider
		config.ApiEndpoint = req.ApiEndpoint
		config.ModelName = req.ModelName

		// Only update API key if new API key is provided
		if req.ApiKey != "" {
			config.ApiKey = req.ApiKey
		}

		if err := s.db.Save(&config).Error; err != nil {
			return config, fmt.Errorf("Failed to update AI config: %w", err)
		}
	}

	// Return configuration without API key
	config.ApiKey = ""
	return config, nil
}

// AIResult carries the outcome of an AI connection test or model listing.
type AIResult struct {
	Message  string
	Response interface{}
}

// TestAI verifies the AI configuration by calling the chat completions
// endpoint with a test prompt.
func (s *Service) TestAI() (*AIResult, error) {
	var config model.AIConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAINotConfigured
		}
		return nil, err
	}

	// Check if enabled
	if !config.Enabled {
		return nil, ErrAIDisabled
	}

	// Check required fields
	if config.ApiEndpoint == "" || config.ApiKey == "" || config.ModelName == "" {
		return nil, ErrAIIncomplete
	}

	// Build base API URL
	// Remove trailing slash to ensure consistent format
	baseURL := strings.TrimSuffix(config.ApiEndpoint, "/")

	// If user entered full chat endpoint (ending with /chat/completions), extract base part
	if strings.HasSuffix(baseURL, "/v1/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
		baseURL = strings.TrimSuffix(baseURL, "/v1")
		baseURL = baseURL + "/v1"
	} else if strings.HasSuffix(baseURL, "/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	}

	// Ensure baseURL ends with /v1
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	// Build chat completion endpoint
	chatCompletionsURL := baseURL + "/chat/completions"

	// Build model validation endpoint
	modelsURL := baseURL + "/models"

	// Get test prompt: use simple test question
	testPrompt := "Say this is a test"

	// Step 1: Verify model exists (optional but recommended)
	modelExists, modelErr := checkModelExists(modelsURL, config.ApiKey, config.ModelName)
	if modelErr != nil {
		// Model check failed, but continue testing chat completion, endpoint may not support model listing
		fmt.Printf("Model validation warning (will continue test): %v\n", modelErr)
	} else if !modelExists {
		return nil, fmt.Errorf("Model '%s' does not exist or is not available, please check model name", config.ModelName)
	}

	// Step 2: Test chat completion functionality
	requestBody := map[string]interface{}{
		"model": config.ModelName,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": testPrompt,
			},
		},
		"max_tokens":  100,
		"temperature": 0.7,
	}

	// Send HTTP request to AI API
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", chatCompletionsURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.ApiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to AI API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Failed to parse AI response: %w", err)
	}

	// Check for error field
	if errorMsg, ok := result["error"]; ok {
		return nil, fmt.Errorf("AI API error: %v", errorMsg)
	}

	// Extract AI response content
	var aiResponse string
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					aiResponse = content
				}
			}
		}
	}

	return &AIResult{
		Message:  "AI connection test successful!",
		Response: aiResponse,
	}, nil
}

// AIModels returns the list of models available from the configured AI
// endpoint.
func (s *Service) AIModels() (*AIResult, error) {
	var config model.AIConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAINotConfigured
		}
		return nil, err
	}

	// Check if enabled
	if !config.Enabled {
		return nil, ErrAIDisabled
	}

	// Check required fields
	if config.ApiEndpoint == "" || config.ApiKey == "" {
		return nil, ErrAIIncompleteModels
	}

	// Build base API URL (consistent with TestAI function)
	baseURL := strings.TrimSuffix(config.ApiEndpoint, "/")

	// If user entered full chat endpoint (ending with /chat/completions), extract base part
	if strings.HasSuffix(baseURL, "/v1/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
		baseURL = strings.TrimSuffix(baseURL, "/v1")
		baseURL = baseURL + "/v1"
	} else if strings.HasSuffix(baseURL, "/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	}

	// Ensure baseURL ends with /v1
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	// Build models list endpoint
	modelsURL := baseURL + "/models"

	// Send request to fetch models list
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.ApiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Failed to fetch models, status: %d, response: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Failed to parse models response: %w", err)
	}

	// Check for errors
	if errorMsg, ok := result["error"]; ok {
		return nil, fmt.Errorf("API error: %v", errorMsg)
	}

	// Extract models list
	var models []map[string]interface{}
	if data, ok := result["data"].([]interface{}); ok {
		for _, model := range data {
			if modelMap, ok := model.(map[string]interface{}); ok {
				modelInfo := map[string]interface{}{
					"id":       modelMap["id"],
					"object":   modelMap["object"],
					"created":  modelMap["created"],
					"owned_by": modelMap["owned_by"],
				}
				models = append(models, modelInfo)
			}
		}
	}

	return &AIResult{
		Message:  "Models fetched successfully",
		Response: models,
	}, nil
}

// checkModelExists checks if model exists
func checkModelExists(modelsURL, apiKey, modelName string) (bool, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to connect to models endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to parse models response: %v", err)
	}

	// Check models list
	if data, ok := result["data"].([]interface{}); ok {
		for _, model := range data {
			if modelMap, ok := model.(map[string]interface{}); ok {
				if id, ok := modelMap["id"].(string); ok && id == modelName {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// GetThemes returns all available themes.
func (s *Service) GetThemes() []public.ThemeInfo {
	return public.GetAvailableThemes()
}

// ThemePreview resolves the preview image path for a theme.
func (s *Service) ThemePreview(themeID string) (string, error) {
	// Check if theme exists
	if !public.ThemeExists(themeID) {
		return "", ErrThemeNotFound
	}

	// Read theme metadata
	metaPath := filepath.Join(public.DataDir, public.ThemesDir, themeID, public.ThemeMetaFile)
	content, err := os.ReadFile(metaPath)
	if err != nil {
		return "", fmt.Errorf("Failed to read theme metadata: %w", err)
	}

	var themeInfo public.ThemeInfo
	if err := json.Unmarshal(content, &themeInfo); err != nil {
		return "", fmt.Errorf("Invalid theme metadata: %w", err)
	}

	// Check if preview is specified
	if themeInfo.Preview == "" {
		return "", ErrPreviewNotSpecified
	}

	// Build preview image path
	previewPath := filepath.Join(public.DataDir, public.ThemesDir, themeID, themeInfo.Preview)

	// Check if preview image exists
	if _, err := os.Stat(previewPath); os.IsNotExist(err) {
		return "", ErrPreviewNotFound
	}

	return previewPath, nil
}

// GetThemeConfig returns the currently active theme stored in the database,
// falling back to the default theme.
func (s *Service) GetThemeConfig() (string, error) {
	var config model.ThemeConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return public.DefaultTheme, nil
		}
		return "", err
	}
	activeTheme := config.ActiveTheme
	if activeTheme == "" {
		activeTheme = public.DefaultTheme
	}
	return activeTheme, nil
}

// UpdateThemeConfig sets the globally active theme in the database.
func (s *Service) UpdateThemeConfig(activeTheme string) (string, error) {
	// Validate that the requested theme actually exists
	if !public.ThemeExists(activeTheme) {
		return "", ErrThemeNotFound
	}

	var config model.ThemeConfig
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			config = model.ThemeConfig{ActiveTheme: activeTheme}
			if err := s.db.Create(&config).Error; err != nil {
				return "", fmt.Errorf("Failed to save theme config: %w", err)
			}
		} else {
			return "", fmt.Errorf("Failed to get theme config: %w", err)
		}
	} else {
		config.ActiveTheme = activeTheme
		if err := s.db.Save(&config).Error; err != nil {
			return "", fmt.Errorf("Failed to update theme config: %w", err)
		}
	}

	return config.ActiveTheme, nil
}

// SetupThemeProvider injects a DB-backed theme provider into the public
// package. It must be called after the database is initialized.
func SetupThemeProvider(db *gorm.DB) {
	public.ActiveThemeProvider = func() string {
		var config model.ThemeConfig
		if err := db.First(&config).Error; err != nil {
			return public.DefaultTheme
		}
		if config.ActiveTheme == "" {
			return public.DefaultTheme
		}
		return config.ActiveTheme
	}
}
