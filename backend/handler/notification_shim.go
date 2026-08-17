package handler

import (
	"vexgo/backend/internal/message"
)

// CreateNotification is a temporary shim that forwards to the message domain.
// It exists so legacy handler files keep compiling during the incremental
// migration; each domain switches to message.NewService(...).CreateNotification
// as it migrates, and this shim disappears together with the handler package.
func CreateNotification(userID uint, notificationType string, title string, content string, relatedID string, relatedType string) error {
	return message.NewService(message.Deps{DB: db}).CreateNotification(userID, notificationType, title, content, relatedID, relatedType)
}
