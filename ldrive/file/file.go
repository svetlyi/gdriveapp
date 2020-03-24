package file

import "github.com/svetlyi/gdriveapp/contracts"

func GetCurFullPath(file contracts.File) string {
	return "/media/photon/371F40450619A640/" + file.CurPath
}
