package fileutils

import (
	"os"
	"path/filepath"
	"strings"
)

func ChangeDirectory(directory string) error {
	if directory != "" && directory != "." {
		if err := os.Chdir(directory); err != nil {
			return err
		}
	}
	return nil
}

var acceptedExtensions = map[string]struct{}{
	".cia":  {},
	".tik":  {},
	".cetk": {},
	".3dsx": {},
}

func HasAcceptedExtension(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	_, ok := acceptedExtensions[ext]
	return ok
}

func GetSupportedExtensions() []string {
	var supportedExtensions []string
	for ext := range acceptedExtensions {
		supportedExtensions = append(supportedExtensions, ext)
	}
	return supportedExtensions
}
