package synchronization

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/logger"
	"github.com/svetlyi/gdriveapp/rdrive"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"github.com/svetlyi/gdriveapp/rdrive/db/migration"
	"google.golang.org/api/drive/v3"
	"os"
	"path/filepath"
	"testing"
)

var appName = "svetlyi_gdriveapp_test"
var testDb = filepath.Join(os.TempDir(), appName+".db")

func TestSet(t *testing.T) {
	err, db, l := setup()
	defer tearDown()

	if nil != err {
		t.Error("could not tear up", err)
	}
	fr := file.NewRepository(db, l)
	rd := rdrive.New()
	sychronizer := New(fr, l)
	var expectedToken = "test-change-token"
	if err = store.Set(NextChangeToken, expectedToken); nil != err {
		t.Error("could not set NextChangeToken", err)
	}
	actualToken, err := store.Get(NextChangeToken)
	if nil != err {
		t.Error("could not get NextChangeToken", err)
	}
	if actualToken != expectedToken {
		t.Error("actual and expected tokens aren't equal")
	}
}

func setup() (error, *sql.DB, contracts.Logger) {
	db, err := sql.Open("sqlite3", testDb)
	if nil != err {
		return errors.Wrap(err, "setup: could not open a database"), nil, nil
	}
	l, err := logger.New(appName, 10000, 10, false)
	if err != nil {
		return errors.Wrap(err, "setup: could not create a logger"), nil, nil
	}
	if err = migration.RunMigrations(db, l); err != nil {
		return errors.Wrap(err, "setup: could not migrate"), nil, nil
	}
	return nil, db, l
}

func tearDown() error {
	return os.Remove(testDb)
}

type FilesService struct{}

func (fs *FilesService) Copy(fileId string, file *drive.File) contracts.FilesCopyCall {
}
func (fs *FilesService) Create(file *drive.File) contracts.FilesCreateCall                {}
func (fs *FilesService) Delete(fileId string) contracts.FilesDeleteCall                   {}
func (fs *FilesService) Get(fileId string) contracts.FilesGetCall                         {}
func (fs *FilesService) List() contracts.FilesListCall                                    {}
func (fs *FilesService) Update(fileId string, file *drive.File) contracts.FilesUpdateCall {}
