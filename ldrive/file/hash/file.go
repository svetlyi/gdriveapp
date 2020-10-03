package file

import (
	"crypto/md5"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os"
)

var filePathHashMap map[string]string

// CalcCachedHash calculates hash and stores it (caches). The result for the second
// and further calls will be returned from cache
func CalcCachedHash(fileFullPath string) (string, error) {
	hash, exists := filePathHashMap[fileFullPath]
	if exists {
		return hash, nil
	}

	f, err := os.Open(fileFullPath)

	if err != nil {
		return "", errors.Wrapf(err, "could not open file %s to calculate hash", fileFullPath)
	}
	defer f.Close()
	if stat, err := f.Stat(); nil == err {
		if stat.IsDir() {
			return "", errors.Wrapf(err, "file %s is a directory", fileFullPath)
		}
	}

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrapf(err, "could not get the file's %s hash", fileFullPath)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
