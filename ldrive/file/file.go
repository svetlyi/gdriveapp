package file

import (
	"crypto/md5"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type File struct {
	relPath string
	// prevModTime shows when the last time the file was downloaded. It is actually
	// the last local modification date
	prevModTime time.Time
	sizeBytes   uint64
	prevHash    string
	removed     bool

	drivePath string
	// hashCache stores the cached hash of the file
	hashCache string
}

func New(
	relPath string,
	prevHash string,
	removed bool,
	prevModTime time.Time,
	sizeBytes uint64,
	drivePath string,
) *File {
	return &File{
		relPath:     relPath,
		prevModTime: prevModTime,
		sizeBytes:   sizeBytes,
		prevHash:    prevHash,
		removed:     removed,
		drivePath:   drivePath,
	}
}

func (f *File) IsDeleted() (bool, error) {
	if _, err := os.Stat(f.GetFullPath()); os.IsNotExist(err) {
		return !f.prevModTime.IsZero(), nil
	} else if err != nil {
		return false, errors.Wrapf(err, "could not check if the file %s is deleted", f.GetFullPath())
	}
	return false, nil
}

func (f *File) IsChanged() (bool, error) {
	stats, err := os.Stat(f.GetFullPath())
	if os.IsNotExist(err) {
		return true, nil
	} else if err != nil {
		return false, errors.Wrapf(err, "could not check if the file %s is updated", f.GetFullPath())
	}

	if f.prevModTime.IsZero() { // it has never been downloaded, but it exists
		return true, nil
	}

	isFolder, err := f.IsFolder()
	if err != nil {
		return false, errors.Wrapf(err, "could not check if %s is a folder", f.GetFullPath())
	}
	return !f.prevModTime.Equal(stats.ModTime()) ||
			(!isFolder && int64(f.sizeBytes) != stats.Size()),
		nil
}

func (f *File) GetName() string {
	relativePath := f.GetRelativePath()
	// remove all the trailing separators in case it was a folder
	relativePath = strings.TrimRight(relativePath, string(os.PathSeparator))
	lastSeparator := strings.LastIndex(relativePath, string(os.PathSeparator))
	return relativePath[lastSeparator+1:]
}

func (f *File) GetRelativePath() string {
	return f.relPath
}

func (f *File) GetFullPath() string {
	return filepath.Join(f.drivePath, f.GetRelativePath())
}

func (f *File) GetReader() (io.Reader, error) {
	return os.Open(f.GetFullPath())
}

func (f *File) GetHash() (string, error) {
	if f.hashCache != "" {
		return f.hashCache, nil
	}

	file, err := os.Open(f.GetFullPath())

	if err != nil {
		return "", errors.Wrapf(err, "could not calculate hash for file %s", f.GetFullPath())
	}
	defer file.Close()
	if stat, err := file.Stat(); nil == err {
		if stat.IsDir() {
			return "", errors.Wrapf(err, "file %s is a directory", f.GetFullPath())
		}
	}

	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", errors.Wrapf(err, "could not get the file's %s hash", f.GetFullPath())
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GetParentFullPath returns the full path of the files' parent
func (f *File) GetParentFullPath() string {
	fullPath := f.GetFullPath()
	return filepath.Clean(fullPath[:len(fullPath)-len(f.GetName())])
}

func (f *File) IsFolder() (bool, error) {
	if stat, err := os.Stat(f.GetFullPath()); err != nil {
		return false, err
	} else {
		return stat.IsDir(), nil
	}
}
