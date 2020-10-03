package drive

import (
	"context"
	"database/sql"
	"github.com/golang/mock/gomock"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	db2 "github.com/svetlyi/gdriveapp/ldrive/db"
	fileRepo "github.com/svetlyi/gdriveapp/ldrive/db/repository"
	"github.com/svetlyi/gdriveapp/logger"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	testDb  = filepath.Join(os.TempDir(), appName+".db")
	appName = "svetlyi_gdriveapp_test"
)

// TestGetFilesToCheckWithoutRootFolder checks that without root folder
// there are no files
func TestGetFilesToCheckWithoutRootFolder(t *testing.T) {
	err, db, l := setup()
	if err != nil {
		t.Fatal(err)
	}
	defer tearDown(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	d := New(
		dir,
		l,
		fileRepo.New(db, l, dir),
	)
	var (
		files   = make(contracts.FilesInterfaceChan)
		errChan = make(contracts.ErrorChan)
	)
	go d.GetFilesToCheck(context.Background(), files, errChan)

	for {
		select {
		case err, isOpen := <-errChan:
			if !isOpen {
				return
			}
			if err != nil {
				t.Error(err)
			}
		case _, isOpen := <-files:
			if !isOpen {
				return
			}
			t.Error("there must not be any files")
			return
		}
	}
}

// TestGetFilesToCheckWithRootFolder checks files within a given root folder
func TestGetFilesToCheckWithRootFolder(t *testing.T) {
	db, dir, d := setupWithRoot(t)
	defer tearDown(t)

	var (
		files   = make(contracts.FilesInterfaceChan)
		errChan = make(contracts.ErrorChan)
	)
	go d.GetFilesToCheck(context.Background(), files, errChan)

	expectedFiles := []string{
		filepath.Join(dir, "_someroot"),
		filepath.Join(dir, "_someroot", "_parent2"),
		filepath.Join(dir, "_someroot", "_parent2", "child1"),
		filepath.Join(dir, "_someroot", "_parent2", "child1", "_test_file.txt"),
	}

	expectedFilesCounter := 0
forloop:
	for {
		select {
		case err, isOpen := <-errChan:
			if !isOpen {
				return
			}
			if err != nil {
				t.Error(err)
			}
		case f, isOpen := <-files:
			if !isOpen {
				break forloop
			}
			if expectedFiles[expectedFilesCounter] != f.GetFullPath() {
				t.Errorf("expected: %s; got: %s", expectedFiles[expectedFilesCounter], f.GetFullPath())
			}
			expectedFilesCounter++
		}
	}
	if expectedFilesCounter != 4 {
		t.Error("wrong amount of files")
	}
	// now we need to check the amount of files in the database
	rows, err := (*db).Query(`
		SELECT 
	    relative_path, prev_hash, prev_mod_time, prev_size, removed
		FROM ldrive_files
		ORDER BY length(relative_path)
	    `,
	)
	if err != nil {
		t.Fatal(err)
	}
	type FileInDb struct {
		relPath     string
		prevHash    string
		prevModTime string
		prevSize    int64
		removed     uint8
	}
	fileInDb := FileInDb{}
	expectedFilesData := []FileInDb{
		{
			relPath:  "_someroot",
			prevHash: "",
			removed:  0,
		},
		{
			relPath:  filepath.Join("_someroot", "_parent2"),
			prevHash: "",
			removed:  0,
		},
		{
			relPath:  filepath.Join("_someroot", "_parent2", "child1"),
			prevHash: "",
			removed:  0,
		},
		{
			relPath:  filepath.Join("_someroot", "_parent2", "child1", "_test_file.txt"),
			prevHash: "f20d9f2072bbeb6691c0f9c5099b01f3",
			removed:  0,
		},
	}
	expectedFilesCounter = 0
	for rows.Next() {
		err := rows.Scan(
			&fileInDb.relPath,
			&fileInDb.prevHash,
			&fileInDb.prevModTime,
			&fileInDb.prevSize,
			&fileInDb.removed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if fileInDb.relPath != expectedFilesData[expectedFilesCounter].relPath {
			t.Errorf("expected: %s; got: %s", expectedFilesData[expectedFilesCounter].relPath, fileInDb.relPath)
		}
		if fileInDb.prevHash != expectedFilesData[expectedFilesCounter].prevHash {
			t.Errorf(
				"expected: %s; got: %s for %s",
				expectedFilesData[expectedFilesCounter].prevHash,
				fileInDb.prevHash,
				fileInDb.relPath,
			)
		}
		fileStat, err := os.Stat(filepath.Join(dir, fileInDb.relPath))
		if err != nil {
			t.Fatal(err)
		}
		if fileInDb.prevModTime != fileStat.ModTime().Format(time.RFC3339Nano) {
			t.Errorf(
				"expected: %s; got: %s for %s",
				fileStat.ModTime().Format(time.RFC3339Nano),
				fileInDb.prevModTime,
				fileInDb.relPath,
			)
		}
		if fileInDb.prevSize != fileStat.Size() {
			t.Errorf(
				"expected: %d; got: %d for %s",
				fileStat.Size(),
				fileInDb.prevSize,
				fileInDb.relPath,
			)
		}
		if fileInDb.removed != expectedFilesData[expectedFilesCounter].removed {
			t.Errorf(
				"expected: %d; got: %d for %s",
				expectedFilesData[expectedFilesCounter].removed,
				fileInDb.removed,
				fileInDb.relPath,
			)
		}
		expectedFilesCounter++
	}
}

func TestCreate(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	newRelPath := filepath.Join("_someroot", "_parent2", "somenewfile.txt")
	mockedFile.EXPECT().GetRelativePath().Return(newRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, newRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	newFileExpectedContent := "new file test"
	mockedFile.EXPECT().GetReader().Return(strings.NewReader(newFileExpectedContent), nil).AnyTimes()
	mockedFile.EXPECT().GetName().Return("somenewfile.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()
	defer func(t *testing.T) {
		if err := os.Remove(newFileFullPath); err != nil {
			t.Fatal(err)
		}
	}(t)
	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	newFileContent, err := ioutil.ReadFile(newFileFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if newFileExpectedContent != string(newFileContent) {
		t.Errorf("expected content: %s; got: %s", newFileExpectedContent, string(newFileContent))
	}
}

func TestUpdate(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	newRelPath := filepath.Join("_someroot", "_parent2", "somenewfile.txt")
	mockedFile.EXPECT().GetRelativePath().Return(newRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, newRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	mockedFile.EXPECT().GetReader().Return(strings.NewReader("new file test"), nil).Times(1)
	mockedFile.EXPECT().GetName().Return("somenewfile.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()
	defer func(t *testing.T) {
		if err := os.Remove(newFileFullPath); err != nil {
			t.Fatal(err)
		}
	}(t)
	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	updatedContent := "someupdatedcontent"
	mockedFile.EXPECT().GetReader().Return(strings.NewReader(updatedContent), nil).Times(1)
	err = d.Update(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	newFileContent, err := ioutil.ReadFile(newFileFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if updatedContent != string(newFileContent) {
		t.Errorf("expected content: %s; got: %s", updatedContent, string(newFileContent))
	}
}

func TestDelete(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	newRelPath := filepath.Join("_someroot", "_parent2", "somenewfile-to-delete.txt")
	mockedFile.EXPECT().GetRelativePath().Return(newRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, newRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	mockedFile.EXPECT().GetReader().Return(strings.NewReader("new file test"), nil).AnyTimes()
	mockedFile.EXPECT().GetName().Return("somenewfile-to-delete.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()

	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	err = d.Delete(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(newFileFullPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("could not delete file %s: %v", newFileFullPath, err)
	}
}

func TestRenameConflicted(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	newRelPath := filepath.Join("_someroot", "_parent2", "somenewfile-to-rename.txt")
	mockedFile.EXPECT().GetRelativePath().Return(newRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, newRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	mockedFile.EXPECT().GetReader().Return(strings.NewReader("new file test"), nil).AnyTimes()
	mockedFile.EXPECT().GetName().Return("somenewfile-to-rename.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()

	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	expectedFileName := "somenewfile-to-rename (0).txt"
	renamedFileExpectedRelPath := filepath.Join("_someroot", "_parent2", expectedFileName)
	renamedFileExpectedFullPath := filepath.Join(dir, renamedFileExpectedRelPath)
	defer func(t *testing.T, expectedPath string) {
		if err := os.Remove(expectedPath); err != nil {
			t.Fatal(err)
		}
	}(t, renamedFileExpectedFullPath)
	newName, err := d.RenameConflicted(context.Background(), mockedFile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if newName != expectedFileName {
		t.Errorf("expected: %s; got: %s", renamedFileExpectedRelPath, newName)
	}
	if _, err = os.Stat(renamedFileExpectedFullPath); err != nil {
		t.Errorf("could not rename file %s: %v", renamedFileExpectedFullPath, err)
	}
}

func TestRename(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	newRelPath := filepath.Join("_someroot", "_parent2", "somenewfile-to-rename.txt")
	mockedFile.EXPECT().GetRelativePath().Return(newRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, newRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	mockedFile.EXPECT().GetReader().Return(strings.NewReader("new file test"), nil).AnyTimes()
	mockedFile.EXPECT().GetName().Return("somenewfile-to-rename.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()

	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	expectedFileName := "somenewfile-renamed-to-new-name.txt"
	renamedFileExpectedRelPath := filepath.Join("_someroot", "_parent2", expectedFileName)
	renamedFileExpectedFullPath := filepath.Join(dir, renamedFileExpectedRelPath)
	defer func(t *testing.T, expectedPath string) {
		if err := os.Remove(expectedPath); err != nil {
			t.Fatal(err)
		}
	}(t, renamedFileExpectedFullPath)
	err = d.Rename(context.Background(), mockedFile, expectedFileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(renamedFileExpectedFullPath); err != nil {
		t.Errorf("could not rename file %s: %v", renamedFileExpectedFullPath, err)
	}
	renamedFile, err := d.GetByFullPath(renamedFileExpectedFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if renamedFile.GetFullPath() != renamedFileExpectedFullPath {
		t.Errorf(
			"expected renamed file full path: %s; got: %s",
			renamedFileExpectedFullPath,
			renamedFile.GetFullPath(),
		)
	}
}

func TestGetByFullPath(t *testing.T) {
	_, dir, d := setupWithRoot(t)
	defer tearDown(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockedFile := contracts.NewMockFile(mockCtrl)
	mockedFile.EXPECT().GetHash().Return("9719bb5dddc9e917899380858e3b9102", nil).AnyTimes()
	expectedRelPath := filepath.Join("_someroot", "_parent2", "somenewfile-to-find.txt")
	mockedFile.EXPECT().GetRelativePath().Return(expectedRelPath).AnyTimes()
	newFileFullPath := filepath.Join(dir, expectedRelPath)
	mockedFile.EXPECT().GetFullPath().Return(newFileFullPath).AnyTimes()
	mockedFile.EXPECT().GetReader().Return(strings.NewReader("new file test"), nil).AnyTimes()
	mockedFile.EXPECT().GetName().Return("somenewfile-to-find.txt").AnyTimes()
	mockedFile.EXPECT().GetParentFullPath().Return(filepath.Join(dir, "_someroot", "_parent2")).AnyTimes()

	err := d.Create(context.Background(), mockedFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func(t *testing.T, filePath string) {
		if err := os.Remove(filePath); err != nil {
			t.Fatal(err)
		}
	}(t, newFileFullPath)
	foundFile, err := d.GetByFullPath(newFileFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if foundFile.GetRelativePath() != expectedRelPath {
		t.Errorf("expected: %s; got: %s", expectedRelPath, foundFile.GetRelativePath())
	}
}

func setup() (error, *sql.DB, contracts.Logger) {
	db, err := sql.Open("sqlite3", testDb)
	if err != nil {
		return errors.Wrap(err, "setup: could not open a database"), nil, nil
	}
	l, err := logger.New(appName, 10000, 10, false)
	if err != nil {
		return errors.Wrap(err, "setup: could not create a logger"), nil, nil
	}
	if err = db2.Migrate(db, l); err != nil {
		return errors.Wrap(err, "setup: could not migrate"), nil, nil
	}
	return nil, db, l
}

func setupWithRoot(t *testing.T) (*sql.DB, string, *Drive) {
	err, db, l := setup()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	repo := fileRepo.New(db, l, dir)
	d := New(
		dir,
		l,
		repo,
	)
	stat, err := os.Stat(filepath.Join(dir, "_someroot"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateFile(
		"_someroot",
		stat.ModTime(),
		uint64(stat.Size()),
	)
	if err != nil {
		t.Fatal(err)
	}
	return db, dir, d
}

func tearDown(t *testing.T) {
	err := os.Remove(testDb)
	if err != nil {
		t.Fatal(err)
	}
}
