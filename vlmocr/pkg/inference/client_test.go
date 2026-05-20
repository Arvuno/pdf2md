package inference

import (
	"context"
	"testing"
	"time"

	"github.com/ninehills/pdf2md/vlmocr/pkg/models"
)

func TestNewClient(t *testing.T) {
	cfg := models.MustGet("dots-ocr")
	client := NewClient("", cfg, 0)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.model.Name != "dots-ocr" {
		t.Errorf("expected model name 'dots-ocr', got %q", client.model.Name)
	}
	if client.timeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %v", client.timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	cfg := models.MustGet("logics-parsing-v2")
	client := NewClient("http://localhost:9000/v1", cfg, 10*time.Second)
	if client.timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", client.timeout)
	}
}

func TestParseImages_EmptySlice(t *testing.T) {
	cfg := models.MustGet("dots-ocr")
	client := NewClient("http://localhost:19999/v1", cfg, 100*time.Millisecond)

	_, err := client.ParseImages(context.Background(), []string{}, 0)
	if err != nil {
		t.Logf("ParseImages with empty slice: %v", err)
	}
}
