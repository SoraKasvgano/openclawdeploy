package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type localPathEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type localPathBrowserResponse struct {
	CurrentPath       string           `json:"current_path"`
	ParentPath        string           `json:"parent_path"`
	CanGoUp           bool             `json:"can_go_up"`
	SuggestedFilePath string           `json:"suggested_file_path"`
	Roots             []localPathEntry `json:"roots"`
	Entries           []localPathEntry `json:"entries"`
}

func buildLocalPathBrowserResponse(rawPath string, currentOpenClawPath string) (localPathBrowserResponse, error) {
	currentDir := resolveBrowseDirectory(rawPath, currentOpenClawPath)
	entries, err := listLocalPathEntries(currentDir)
	if err != nil {
		return localPathBrowserResponse{}, err
	}

	parentPath := filepath.Dir(currentDir)
	canGoUp := parentPath != currentDir

	return localPathBrowserResponse{
		CurrentPath:       currentDir,
		ParentPath:        parentPath,
		CanGoUp:           canGoUp,
		SuggestedFilePath: filepath.Join(currentDir, "openclaw.json"),
		Roots:             listFilesystemRoots(),
		Entries:           entries,
	}, nil
}

func resolveBrowseDirectory(rawPath string, currentOpenClawPath string) string {
	candidate := normalizeFilesystemPath(rawPath)
	if candidate == "" {
		candidate = normalizeFilesystemPath(currentOpenClawPath)
	}
	if candidate == "" {
		candidate = defaultOpenClawPath()
	}

	if info, err := os.Stat(candidate); err == nil {
		if info.IsDir() {
			return candidate
		}
		return filepath.Dir(candidate)
	}

	if strings.EqualFold(filepath.Ext(candidate), ".json") {
		candidate = filepath.Dir(candidate)
	}

	if existing := nearestExistingDirectory(candidate); existing != "" {
		return existing
	}

	if runtime.GOOS == "windows" {
		roots := listFilesystemRoots()
		if len(roots) > 0 {
			return roots[0].Path
		}
	}
	return string(filepath.Separator)
}

func nearestExistingDirectory(path string) string {
	path = normalizeFilesystemPath(path)
	if path == "" {
		return ""
	}

	for {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}

		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func listLocalPathEntries(dir string) ([]localPathEntry, error) {
	readDirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("browse %s: %w", dir, err)
	}

	directories := make([]localPathEntry, 0)
	files := make([]localPathEntry, 0)
	for _, entry := range readDirEntries {
		fullPath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			directories = append(directories, localPathEntry{
				Name:  entry.Name(),
				Path:  fullPath,
				IsDir: true,
			})
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		files = append(files, localPathEntry{
			Name:  entry.Name(),
			Path:  fullPath,
			IsDir: false,
		})
	}

	sort.Slice(directories, func(i, j int) bool {
		return strings.ToLower(directories[i].Name) < strings.ToLower(directories[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return append(directories, files...), nil
}

func listFilesystemRoots() []localPathEntry {
	if runtime.GOOS != "windows" {
		return []localPathEntry{{
			Name:  string(filepath.Separator),
			Path:  string(filepath.Separator),
			IsDir: true,
		}}
	}

	roots := make([]localPathEntry, 0)
	for drive := 'A'; drive <= 'Z'; drive++ {
		path := fmt.Sprintf("%c:\\", drive)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		roots = append(roots, localPathEntry{
			Name:  path,
			Path:  path,
			IsDir: true,
		})
	}
	return roots
}
