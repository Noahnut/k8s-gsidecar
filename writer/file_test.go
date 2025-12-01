package writer

import (
	"os"
	"testing"
)

func TestFileWriter_NestedDirectory(t *testing.T) {
	testFolder := "test-nested/subfolder/deep"
	defer os.RemoveAll("test-nested")

	fw := NewFileWriter()
	err := fw.Write(testFolder, "test.txt", "content")
	if err != nil {
		t.Fatalf("Failed to write to nested directory: %v", err)
	}

	if _, err := os.Stat(testFolder + "/test.txt"); os.IsNotExist(err) {
		t.Errorf("File was not created")
	}
}
