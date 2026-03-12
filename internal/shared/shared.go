package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func ExecutableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	return filepath.Dir(exePath)
}

func RuntimeBaseDir() string {
	exeDir := ExecutableDir()
	lower := strings.ToLower(exeDir)
	if strings.Contains(lower, "go-build") || strings.Contains(lower, "temp") {
		cwd, err := os.Getwd()
		if err == nil {
			return cwd
		}
	}
	return exeDir
}

func PrettyJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, perm); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

func ReadJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 10<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain a single JSON object")
		}
		return err
	}

	return nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

func HashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func NormalizeDeviceID(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char + ('a' - 'A'))
		}
	}

	return builder.String()
}
