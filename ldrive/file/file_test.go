package file

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsDeletedForExistingFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		"_parent1/_parent2/child1/_test_file.txt",
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	var (
		isDeleted bool
	)
	isDeleted, err = f.IsDeleted()
	if err != nil {
		t.Errorf("could not check if the file was deleted: %v", err)
	}
	if isDeleted {
		t.Error("should not be deleted. The file exists")
	}
}

func TestIsDeletedForNotExistingFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		"_parent1/_parent2/child1/wrong_file.txt",
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		true,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	isDeleted, err := f.IsDeleted()
	if err != nil {
		t.Errorf("could not check if the file was deleted: %v", err)
	}
	if !isDeleted {
		t.Error("should be deleted. The file does not exist")
	}
}

func TestGetNameForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		"_parent1/_parent2/child1/_test_file.txt",
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	name := f.GetName()
	expectedName := "_test_file.txt"
	if name != expectedName {
		t.Errorf("%s name expected. got: %s", expectedName, name)
	}
}

func TestGetNameForFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		"_parent1/_parent2/child1/",
		"",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	name := f.GetName()
	expectedName := "child1"
	if name != expectedName {
		t.Errorf("%s name expected. got: %s", expectedName, name)
	}
}

func TestIsChangedForChangedFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		"_parent1/_parent2/child1/_test_file.txt",
		"5df6fddbd34e2cb7d165b36cfb1b62f6",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	isChanged, err := f.IsChanged()
	if err != nil {
		t.Fatalf("could not determine if the file was changed: %v", err)
	}
	if !isChanged {
		t.Error("the file was changed")
	}
}

func TestIsChangedForNotChangedFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	stat, err := os.Stat(relFilePath)
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		stat.ModTime(),
		9,
		dir,
	)
	isChanged, err := f.IsChanged()
	if err != nil {
		t.Fatalf("could not determine if the file was changed: %v", err)
	}
	if isChanged {
		t.Error("the file was not changed. Download time and modified time are equal")
	}
}

func TestIsChangedForNotChangedFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/"
	stat, err := os.Stat(relFilePath)
	if err != nil {
		t.Fatal(err)
	}
	f := New(
		relFilePath,
		"",
		false,
		stat.ModTime(),
		0,
		dir,
	)
	isChanged, err := f.IsChanged()
	if err != nil {
		t.Fatalf("could not determine if the folder was changed: %v", err)
	}
	if isChanged {
		t.Error("the folder was not changed. Download time and modified time are equal")
	}
}

func TestGetRelativePathForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	if relFilePath != f.GetRelativePath() {
		t.Errorf("incorrect relative path. Expected: %s. Got: %s", relFilePath, f.GetRelativePath())
	}
}

func TestGetRelativePathForFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	if relFilePath != f.GetRelativePath() {
		t.Errorf("incorrect relative path. Expected: %s. Got: %s", relFilePath, f.GetRelativePath())
	}
}

func TestGetFullPathForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	expectedFullPath := filepath.Join(dir, relFilePath)
	if expectedFullPath != f.GetFullPath() {
		t.Errorf("incorrect full path. Expected: %s. Got: %s", expectedFullPath, f.GetFullPath())
	}
}

func TestGetFullPathForFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/"
	f := New(
		relFilePath,
		"",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	expectedFullPath := filepath.Join(dir, relFilePath)
	if expectedFullPath != f.GetFullPath() {
		t.Errorf("incorrect full path. Expected: %s. Got: %s", expectedFullPath, f.GetFullPath())
	}
}

func TestGetParentFullPathForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	expectedFullPath := filepath.Join(dir, "_parent1/_parent2/child1/")
	if expectedFullPath != f.GetParentFullPath() {
		t.Errorf("incorrect parent full path. Expected: %s. Got: %s", expectedFullPath, f.GetParentFullPath())
	}
}

func TestGetParentFullPathForFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/"
	f := New(
		relFilePath,
		"",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	expectedFullPath := filepath.Join(dir, "_parent1/_parent2/")
	if expectedFullPath != f.GetParentFullPath() {
		t.Errorf("incorrect parent full path. Expected: %s. Got: %s", expectedFullPath, f.GetParentFullPath())
	}
}

func TestGetReaderForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	expectedContent := "test file"
	reader, err := f.GetReader()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if expectedContent != string(bytes) {
		t.Errorf("incorrect content. Expected: %s. Got: %s", expectedContent, string(bytes))
	}
}

func TestGetHash(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	expectedHash := "f20d9f2072bbeb6691c0f9c5099b01f3"

	f := New(
		relFilePath,
		expectedHash,
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	hash, err := f.GetHash()
	if err != nil {
		t.Fatal(err)
	}
	if expectedHash != hash {
		t.Errorf("incorrect hash. Expected: %s. Got: %s", expectedHash, hash)
	}
}

func TestIsFolderForFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/_test_file.txt"
	f := New(
		relFilePath,
		"f20d9f2072bbeb6691c0f9c5099b01f3",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	isFolder, err := f.IsFolder()
	if err != nil {
		t.Fatal(err)
	}
	if isFolder {
		t.Errorf("%s is not a folder", relFilePath)
	}
}

func TestIsFolderForFolder(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relFilePath := "_parent1/_parent2/child1/"
	f := New(
		relFilePath,
		"",
		false,
		time.Date(2020, 9, 1, 1, 1, 1, 1, time.UTC),
		9,
		dir,
	)
	isFolder, err := f.IsFolder()
	if err != nil {
		t.Fatal(err)
	}
	if !isFolder {
		t.Errorf("%s is a folder", relFilePath)
	}
}
