package util

import (
	"os"
	"path/filepath"
)

func RemoveDirectory(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return err
	}
	for _, file := range files {
		err = os.RemoveAll(file)
		if err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}
