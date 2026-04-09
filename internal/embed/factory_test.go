package embed

import (
	"runtime"
	"testing"
)

func TestNewEmbedderEmpty(t *testing.T) {
	e, err := NewEmbedder("", "")
	if err != nil {
		t.Fatalf("empty provider should return nil, nil; got err: %v", err)
	}
	if e != nil {
		t.Error("empty provider should return nil embedder")
	}
}

func TestNewEmbedderUnknown(t *testing.T) {
	_, err := NewEmbedder("imaginary", "")
	if err == nil {
		t.Error("unknown provider should return error")
	}
}

func TestNewEmbedderLocalNoModel(t *testing.T) {
	// With a non-existent model path, should error
	_, err := NewEmbedder("local", "")
	// Either "model not found" or init error -- both are acceptable
	if err == nil && !ModelExists() {
		t.Error("local provider without model should return error")
	}
}

func TestOrtLibName(t *testing.T) {
	name := ortLibName()
	switch runtime.GOOS {
	case "linux":
		if name != "libonnxruntime.so" {
			t.Errorf("expected libonnxruntime.so on linux, got %s", name)
		}
	case "darwin":
		if name != "libonnxruntime.dylib" {
			t.Errorf("expected libonnxruntime.dylib on darwin, got %s", name)
		}
	case "windows":
		if name != "onnxruntime.dll" {
			t.Errorf("expected onnxruntime.dll on windows, got %s", name)
		}
	}
}

func TestOrtReleaseURL(t *testing.T) {
	url, name, err := ortReleaseURL()
	if err != nil {
		t.Fatalf("ortReleaseURL: %v", err)
	}
	if url == "" || name == "" {
		t.Error("expected non-empty URL and name")
	}
	if len(url) < 20 {
		t.Errorf("URL too short: %s", url)
	}
}
