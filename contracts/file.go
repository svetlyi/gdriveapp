package contracts

import (
	"errors"
	"io"
	"os"
)

var NotExist = errors.New("does not exist")

type ExtendedFileInfo struct {
	FileInfo os.FileInfo
	FullPath string
}

// File is a common interface for a file for some drive
type File interface {
	IsDeleted() (bool, error)
	IsChanged() (bool, error)

	GetName() string

	GetRelativePath() string
	GetFullPath() string

	// GetParentFullPath returns the full path of the files' parent
	GetParentFullPath() string

	GetReader() (io.Reader, error)

	GetHash() (string, error)

	IsFolder() (bool, error)
}

type FilesInterfaceChan chan File
