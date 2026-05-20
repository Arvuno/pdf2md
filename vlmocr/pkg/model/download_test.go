package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsModelDownloaded(t *testing.T) {
	dir := t.TempDir()
	if IsModelDownloaded(dir) {
		t.Error("expected false without config.json")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if !IsModelDownloaded(dir) {
		t.Error("expected true with config.json")
	}
}

func TestDownload_MissingRepoID(t *testing.T) {
	_, err := Download(DownloadConfig{TargetDir: t.TempDir()})
	if err == nil {
		t.Error("expected error for empty repo ID")
	}
}

func TestDownload_MissingTargetDir(t *testing.T) {
	_, err := Download(DownloadConfig{RepoID: "test-org/test-model"})
	if err == nil {
		t.Error("expected error for empty target dir")
	}
}

func TestDownload_AlreadyPrepared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := Download(DownloadConfig{
		RepoID:    "test-org/test-model",
		TargetDir: dir,
	})
	if err != nil {
		t.Fatalf("expected no error for already-prepared model, got: %v", err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs failed: %v", err)
	}
	if got != absDir {
		t.Errorf("expected %s, got %s", absDir, got)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "nested", "dst.txt")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("expected copied content, got %q", string(got))
	}
}
