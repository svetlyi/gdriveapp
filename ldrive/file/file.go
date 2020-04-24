package file

import (
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/contracts"
	"path/filepath"
)

func GetCurFullPath(file contracts.File) string {
	return filepath.Join(config.GetCfg().DrivePath, file.CurPath)
}

func GetPrevFullPath(file contracts.File) string {
	return filepath.Join(config.GetCfg().DrivePath, file.PrevPath)
}
