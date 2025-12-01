package writer

import (
	"os"
	"path"
	"strings"
)

type FileWriter struct {
}

func NewFileWriter() *FileWriter {
	return &FileWriter{}
}

func (f *FileWriter) Init(folder string) {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		os.Mkdir(folder, 0755)
	}
}

func (f *FileWriter) Write(folder string, fileName string, data string) error {
	f.Init(folder)
	filePath := path.Join(folder, fileName)

	return os.WriteFile(filePath, []byte(data), 0644)
}

func (f *FileWriter) Remove(folder string, fileName string) error {
	f.Init(folder)
	filePath := path.Join(folder, fileName)
	return os.Remove(filePath)
}

func (f *FileWriter) IsJSON(fileName string) bool {
	return strings.HasSuffix(fileName, ".json")
}
