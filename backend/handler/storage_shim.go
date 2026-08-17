package handler

import (
	"vexgo/backend/internal/upload"
)

// DataDir is a transition-era global for local media storage paths, still used
// by user_management.go. It is removed once that domain migrates.
var DataDir string

// fileRemover is a temporary bridge that gives the legacy handler package
// access to the injected file storage (used by UpdateProfile's avatar
// deletion). It is set by main.go and disappears together with the handler
// package once the auth domain migrates to internal/auth.
var fileRemover upload.Storage

// SetFileRemover wires the storage instance into the legacy handler package.
func SetFileRemover(s upload.Storage) {
	fileRemover = s
}

// deleteImageFile deletes a stored file by its public URL through the injected
// storage. Temporary shim for UpdateProfile; post domain uses post.FileRemover.
func deleteImageFile(url string) error {
	return fileRemover.Delete(url)
}
