package embed

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	modelURL   = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"
	vocabURL   = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"
	ortVersion = "1.22.0"
)

// NewEmbedder creates an embedder based on the provider name.
// provider: "openai", "local"/"onnx", or "" (returns nil).
func NewEmbedder(provider, model string) (Embedder, error) {
	switch provider {
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
		}
		return NewOpenAIEmbedder(apiKey, model)

	case "local", "onnx":
		if !ModelExists() {
			return nil, fmt.Errorf("ONNX model not found; run 'agent-memory embeddings enable --local' to download it")
		}
		return NewOnnxEmbedder(DefaultModelPath(), DefaultVocabPath())

	case "":
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", provider)
	}
}

// EnsureModel downloads the ONNX model, vocab, and runtime library if not already present.
func EnsureModel() (string, string, error) {
	modelPath := DefaultModelPath()
	vocabPath := DefaultVocabPath()

	// Ensure ONNX Runtime library
	if findOnnxLib() == "" {
		fmt.Println("ONNX Runtime not found on system. Downloading...")
		if err := ensureOnnxRuntime(); err != nil {
			return "", "", fmt.Errorf("install ONNX Runtime: %w", err)
		}
	}

	if ModelExists() {
		return modelPath, vocabPath, nil
	}

	dir := ModelDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", fmt.Errorf("create model dir: %w", err)
	}

	fmt.Println("Downloading ONNX model (all-MiniLM-L6-v2)...")

	if err := downloadFile(modelURL, modelPath, "model.onnx (~87MB)"); err != nil {
		os.Remove(modelPath)
		return "", "", fmt.Errorf("download model: %w", err)
	}

	if err := downloadFile(vocabURL, vocabPath, "vocab.txt"); err != nil {
		os.Remove(vocabPath)
		return "", "", fmt.Errorf("download vocab: %w", err)
	}

	fmt.Printf("Model saved to: %s\n", dir)
	return modelPath, vocabPath, nil
}

// ortLibDir returns ~/.agent-memory/lib/ where we store the downloaded ORT library.
func ortLibDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-memory", "lib")
}

// ortReleaseURL returns the download URL for ONNX Runtime for the current platform.
func ortReleaseURL() (string, string, error) {
	base := "https://github.com/microsoft/onnxruntime/releases/download/v" + ortVersion + "/"

	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "amd64" {
			name := "onnxruntime-linux-x64-" + ortVersion + ".tgz"
			return base + name, name, nil
		}
		return "", "", fmt.Errorf("unsupported Linux arch: %s", runtime.GOARCH)

	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			name := "onnxruntime-osx-arm64-" + ortVersion + ".tgz"
			return base + name, name, nil
		case "amd64":
			name := "onnxruntime-osx-x86_64-" + ortVersion + ".tgz"
			return base + name, name, nil
		default:
			return "", "", fmt.Errorf("unsupported macOS arch: %s", runtime.GOARCH)
		}

	case "windows":
		if runtime.GOARCH == "amd64" {
			name := "onnxruntime-win-x64-" + ortVersion + ".zip"
			return base + name, name, nil
		}
		return "", "", fmt.Errorf("unsupported Windows arch: %s", runtime.GOARCH)

	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ortLibName returns the expected library filename for the current platform.
func ortLibName() string {
	switch runtime.GOOS {
	case "linux":
		return "libonnxruntime.so"
	case "darwin":
		return "libonnxruntime.dylib"
	case "windows":
		return "onnxruntime.dll"
	default:
		return "libonnxruntime.so"
	}
}

// ensureOnnxRuntime downloads and extracts the ONNX Runtime shared library.
func ensureOnnxRuntime() error {
	libDir := ortLibDir()
	libPath := filepath.Join(libDir, ortLibName())

	// Already downloaded?
	if _, err := os.Stat(libPath); err == nil {
		return nil
	}

	url, archiveName, err := ortReleaseURL()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(libDir, 0700); err != nil {
		return fmt.Errorf("create lib dir: %w", err)
	}

	// Download archive to temp
	archivePath := filepath.Join(libDir, archiveName)
	if err := downloadFile(url, archivePath, fmt.Sprintf("ONNX Runtime %s", ortVersion)); err != nil {
		return err
	}
	defer os.Remove(archivePath)

	// Extract the shared library
	if strings.HasSuffix(archiveName, ".tgz") {
		if err := extractTgzLib(archivePath, libDir); err != nil {
			return fmt.Errorf("extract tgz: %w", err)
		}
	} else if strings.HasSuffix(archiveName, ".zip") {
		if err := extractZipLib(archivePath, libDir); err != nil {
			return fmt.Errorf("extract zip: %w", err)
		}
	}

	// Verify
	if _, err := os.Stat(libPath); err != nil {
		return fmt.Errorf("library not found after extraction: %s", libPath)
	}

	fmt.Printf("ONNX Runtime installed to: %s\n", libDir)
	return nil
}

// extractTgzLib extracts libonnxruntime.* from a .tgz archive into destDir.
func extractTgzLib(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	libPrefix := "libonnxruntime."

	// First pass: extract real files, collect symlinks
	type symlink struct {
		name, target string
	}
	var symlinks []symlink

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		base := filepath.Base(hdr.Name)
		if !strings.HasPrefix(base, libPrefix) {
			continue
		}

		dest := filepath.Join(destDir, base)

		switch hdr.Typeflag {
		case tar.TypeSymlink:
			symlinks = append(symlinks, symlink{name: dest, target: filepath.Base(hdr.Linkname)})
		case tar.TypeReg:
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}

	// Create symlinks after all files are extracted
	for _, sl := range symlinks {
		os.Remove(sl.name) // remove any empty file from previous attempt
		if err := os.Symlink(sl.target, sl.name); err != nil {
			// If symlinks fail (e.g. Windows), copy the target file instead
			target := filepath.Join(destDir, sl.target)
			if copyErr := copyFile(target, sl.name); copyErr != nil {
				return fmt.Errorf("create symlink %s -> %s: %w", sl.name, sl.target, err)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// extractZipLib extracts onnxruntime.dll from a .zip archive into destDir.
func extractZipLib(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base != "onnxruntime.dll" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		dest := filepath.Join(destDir, base)
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
		break
	}
	return nil
}

func downloadFile(url, dest, label string) error {
	fmt.Printf("  Downloading %s...\n", label)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}

	if written == 0 {
		os.Remove(tmp)
		return fmt.Errorf("downloaded 0 bytes")
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}

	fmt.Printf("  %s: %d bytes\n", filepath.Base(dest), written)
	return nil
}
