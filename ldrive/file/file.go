package file

import "github.com/svetlyi/gdriveapp/contracts"

func IsLocalVersionOlder() {

}

func GetFullPath(file contracts.File) string {
	return "/media/photon/371F40450619A640/" + file.Path
}
