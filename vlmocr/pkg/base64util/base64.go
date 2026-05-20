// Package base64util provides image-to-base64 data URI encoding
// compatible with the dots.mocr OpenAI API format.
package base64util

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImageToBase64DataURI reads an image file and returns a base64 data URI
// suitable for OpenAI chat completion API image_url fields.
func ImageToBase64DataURI(imagePath string) (string, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("reading image file %s: %w", imagePath, err)
	}

	mediaType := detectMediaType(imagePath)
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mediaType, encoded), nil
}

// detectMediaType returns the MIME type based on file extension.
func detectMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
}
