package file

import (
	"github.com/svetlyi/gdriveapp/contracts"
	"path/filepath"
)

func GetCurFullPath(file contracts.File) string {
	return filepath.Join("/media/photon/371F40450619A640/", file.CurPath)
}

func GetPrevFullPath(file contracts.File) string {
	return filepath.Join("/media/photon/371F40450619A640/", file.PrevPath)
}
