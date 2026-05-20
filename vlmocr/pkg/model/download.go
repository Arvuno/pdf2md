// Package model handles downloading and managing VLM model weights from HuggingFace.
// It uses go-huggingface to fetch files, then materializes a plain model directory
// that can be mounted directly into vLLM.
package model

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gomlx/go-huggingface/hub"
)

// DownloadConfig holds configuration for model download.
type DownloadConfig struct {
	RepoID    string // HuggingFace repo ID (e.g. "rednote-hilab/dots.mocr")
	TargetDir string // Plain local model directory (e.g. "weights/dots-ocr")
	Token     string // HuggingFace API token (optional, reads HF_TOKEN env if empty)
}

// Download downloads a model from HuggingFace and prepares it in TargetDir.
// If TargetDir already contains config.json, the download is skipped.
func Download(cfg DownloadConfig) (string, error) {
	if cfg.RepoID == "" {
		return "", fmt.Errorf("repo ID is required")
	}
	if cfg.TargetDir == "" {
		return "", fmt.Errorf("target dir is required")
	}

	targetDir, err := filepath.Abs(cfg.TargetDir)
	if err != nil {
		return "", fmt.Errorf("resolving target dir: %w", err)
	}

	if IsModelDownloaded(targetDir) {
		fmt.Printf("Model already prepared: %s\n", targetDir)
		return targetDir, nil
	}

	token := cfg.Token
	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}

	repo := hub.New(cfg.RepoID).WithAuth(token)
	fmt.Printf("Downloading model %s to %s (this may take a while)...\n", cfg.RepoID, targetDir)

	var fileNames []string
	for fileName, err := range repo.IterFileNames() {
		if err != nil {
			return "", fmt.Errorf("listing files: %w", err)
		}
		fileNames = append(fileNames, fileName)
	}
	if len(fileNames) == 0 {
		return "", fmt.Errorf("no files found in repo %s", cfg.RepoID)
	}

	downloadedPaths, err := repo.DownloadFiles(fileNames...)
	if err != nil {
		return "", fmt.Errorf("downloading files: %w", err)
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return "", fmt.Errorf("cleaning target dir: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("creating target dir: %w", err)
	}

	for i, src := range downloadedPaths {
		dst := filepath.Join(targetDir, filepath.FromSlash(fileNames[i]))
		if err := copyFile(src, dst); err != nil {
			return "", fmt.Errorf("copying %s: %w", fileNames[i], err)
		}
	}

	if !IsModelDownloaded(targetDir) {
		return "", fmt.Errorf("downloaded model is missing config.json in %s", targetDir)
	}

	fmt.Printf("Prepared %d files in %s\n", len(downloadedPaths), targetDir)
	return targetDir, nil
}

// IsModelDownloaded checks if a plain local model directory is ready for vLLM.
func IsModelDownloaded(modelDir string) bool {
	if modelDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(modelDir, "config.json"))
	return err == nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src) // follows HF cache symlinks
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0644
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
