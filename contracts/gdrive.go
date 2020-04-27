package contracts

import (
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"io"
	"net/http"
)

type FilesService interface {
	Copy(fileId string, file *drive.File) FilesCopyCall
	Create(file *drive.File) FilesCreateCall
	Delete(fileId string) FilesDeleteCall
	Get(fileId string) FilesGetCall
	List() FilesListCall
	Update(fileId string, file *drive.File) *drive.FilesUpdateCall
}

type FilesCreateCall interface {
	Fields(s ...googleapi.Field) FilesCreateCall
	Media(r io.Reader, options ...googleapi.MediaOption) FilesCreateCall
	Do(opts ...googleapi.CallOption) (*drive.File, error)
}

type FilesCopyCall interface {
	Fields(s ...googleapi.Field) FilesCopyCall
	Do(opts ...googleapi.CallOption) (*drive.File, error)
}

type FilesDeleteCall interface {
	Fields(s ...googleapi.Field) *FilesDeleteCall
	Do(opts ...googleapi.CallOption) error
}

type FilesGetCall interface {
	Fields(s ...googleapi.Field) FilesCopyCall
	Download(opts ...googleapi.CallOption) (*http.Response, error)
	Do(opts ...googleapi.CallOption) (*drive.File, error)
}

type FilesListCall interface {
	PageSize(pageSize int64) FilesListCall
	PageToken(pageToken string) FilesListCall
	Fields(s ...googleapi.Field) FilesListCall
	Do(opts ...googleapi.CallOption) (*drive.FileList, error)
}

type FilesUpdateCall interface {
	AddParents(addParents string) FilesUpdateCall
	RemoveParents(removeParents string) FilesUpdateCall
	Media(r io.Reader, options ...googleapi.MediaOption) FilesUpdateCall
	Fields(s ...googleapi.Field) FilesUpdateCall
	Do(opts ...googleapi.CallOption) (*File, error)
}

type ChangesService interface {
	GetStartPageToken() *drive.ChangesGetStartPageTokenCall
	List(pageToken string) *drive.ChangesListCall
}
