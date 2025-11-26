package writer

import (
	"os"
	"path"
	"strings"
)

type FileWriter struct {
	folder string
}

func NewFileWriter(folder string) *FileWriter {

	return &FileWriter{
		folder: folder,
	}
}

func (f *FileWriter) Write(fileName string, data string) error {
	filePath := path.Join(f.folder, fileName)
	return os.WriteFile(filePath, []byte(data), 0644)
}

func (f *FileWriter) Remove(fileName string) error {
	filePath := path.Join(f.folder, fileName)
	return os.Remove(filePath)
}

func (f *FileWriter) IsJSON(fileName string) bool {
	return strings.HasSuffix(fileName, ".json")
}
