package config

import (
	"io/fs"
	"os"
	"path/filepath"
)

type Loader struct {
	WorkDir string
}

func NewLoader(workDir string) *Loader {
	return &Loader{WorkDir: workDir}
}

func (l *Loader) Files() []string {
	info, err := os.Stat(l.WorkDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var files []string
	_ = filepath.WalkDir(l.WorkDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		extension := filepath.Ext(entry.Name())
		if extension == ".yaml" || extension == ".yml" {
			files = append(files, path)
		}

		return nil
	})

	return files
}
