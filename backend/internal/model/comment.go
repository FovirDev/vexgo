package model

import "time"

// Comment model
// Supports parent comments (parentId) for nesting
// Associated with User to return author information
// Associated with Post for cascading on statistics or deletion
// GORM automatically creates foreign key

type Comment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	PostID    uint      `json:"postId"`
	Post      Post      `json:"-" gorm:"foreignKey:PostID"`
	UserID    uint      `json:"userId"`
	User      User      `json:"author" gorm:"foreignKey:UserID"`
	Content   string    `json:"content" gorm:"type:text"`
	Status    string    `json:"status" gorm:"size:20;default:'published'"` // published, pending, rejected
	ParentID  *uint     `json:"parentId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
