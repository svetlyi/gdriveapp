package file

import (
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/contracts"
	"path/filepath"
)

func GetCurFullPath(cfg config.Cfg, file contracts.File) string {
	return filepath.Join(cfg.DrivePath, file.CurPath)
}

func GetPrevFullPath(cfg config.Cfg, file contracts.File) string {
	return filepath.Join(cfg.DrivePath, file.PrevPath)
}
