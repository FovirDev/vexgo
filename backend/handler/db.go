package handler

import (
	"gorm.io/gorm"
)

// db is the package-global database instance used by the legacy handler
// package. It is a transition artifact: domain packages receive the
// *gorm.DB via constructor injection, and this global is removed once the
// migration to internal/ is complete.
var db *gorm.DB

// DB returns the database instance
func DB() *gorm.DB {
	return db
}

// SetDB sets the database instance
func SetDB(database *gorm.DB) {
	db = database
}
