package sftp

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipToDirWritesFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	writer, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(writer)
	file, err := zipWriter.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "out")
	count, err := ExtractZipArchive(archive, destination)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatalf("extracted = %d", count)
	}
	data, err := os.ReadFile(filepath.Join(destination, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hi" {
		t.Fatalf("content = %q", data)
	}
}
