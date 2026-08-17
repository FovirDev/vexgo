package handler

import (
	"net/http"

	"vexgo/backend/model"

	"github.com/gin-gonic/gin"
)

// GetStats returns aggregate site statistics.
func GetStats(c *gin.Context) {
	// Get current user role
	var userRole string
	if userContext, exists := c.Get("user"); exists {
		if userMap, ok := userContext.(map[string]interface{}); ok {
			if role, ok := userMap["role"].(string); ok {
				userRole = role
			}
		}
	}

	// Check if guest viewing is allowed
	var allowGuestView bool
	var config model.GeneralSettings
	if err := db.First(&config).Error; err != nil {
		// Default to true if config not found
		allowGuestView = true
	} else {
		allowGuestView = config.AllowGuestViewPosts
	}

	// If not logged in and guest viewing is not allowed, return empty result
	if userRole == "" && !allowGuestView {
		c.JSON(http.StatusOK, gin.H{
			"stats": gin.H{
				"posts":      0,
				"users":      0,
				"comments":   0,
				"categories": 0,
				"tags":       0,
			},
		})
		return
	}

	var postsCount, usersCount, categoriesCount, tagsCount, commentsCount int64

	db.Model(&model.Post{}).Count(&postsCount)
	db.Model(&model.User{}).Count(&usersCount)
	db.Model(&model.Category{}).Count(&categoriesCount)
	db.Model(&model.Tag{}).Count(&tagsCount)
	db.Model(&model.Comment{}).Count(&commentsCount)

	c.JSON(http.StatusOK, gin.H{
		"stats": gin.H{
			"posts":      postsCount,
			"users":      usersCount,
			"comments":   commentsCount,
			"categories": categoriesCount,
			"tags":       tagsCount,
		},
	})
}
