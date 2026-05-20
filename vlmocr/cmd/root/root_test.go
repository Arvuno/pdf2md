package root

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	if cmd.Use != "vlmocr [flags] <input.pdf>" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	flags := cmd.Flags()
	flagNames := []string{
		"model", "model-dir", "output", "dpi", "port",
		"concurrency", "vllm-image", "gpu",
		"no-headers", "timeout",
	}

	for _, name := range flagNames {
		if flags.Lookup(name) == nil {
			t.Errorf("expected flag %q to be defined", name)
		}
	}

	// Verify skip-download is NOT a flag
	if flags.Lookup("skip-download") != nil {
		t.Error("skip-download flag should not exist")
	}
}

func TestCommand_Help(t *testing.T) {
	cmd := NewCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	_ = cmd.Execute()

	output := buf.String()
	if !strings.Contains(output, "vlmocr") {
		t.Error("help should contain 'vlmocr'")
	}
	if !strings.Contains(output, "dots-ocr") {
		t.Error("help should mention dots-ocr model")
	}
	if !strings.Contains(output, "logics-parsing-v2") {
		t.Error("help should mention logics-parsing-v2 model")
	}
	if !strings.Contains(output, "model-dir") {
		t.Error("help should mention model-dir flag")
	}
}

func TestCommand_MissingArgs(t *testing.T) {
	cmd := NewCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestCommand_InvalidFile(t *testing.T) {
	cmd := NewCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"/nonexistent/file.pdf"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestCommand_InvalidModel(t *testing.T) {
	cmd := NewCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--model", "nonexistent", "test.pdf"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid model")
	}
	if !strings.Contains(err.Error(), "unknown model") {
		t.Errorf("expected 'unknown model' in error, got: %v", err)
	}
}

func TestCommand_ModelFlagDefault(t *testing.T) {
	cmd := NewCommand()
	f := cmd.Flags().Lookup("model")
	if f == nil {
		t.Fatal("expected 'model' flag")
	}
	if f.DefValue != "dots-ocr" {
		t.Errorf("expected default model 'dots-ocr', got %q", f.DefValue)
	}
}
