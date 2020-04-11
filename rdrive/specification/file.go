// Package specification describes rules and specifications
// of the remote files, such as if it can be downloaded, if it is a folder and so on.
package specification

import (
	"github.com/svetlyi/gdriveapp/contracts"
	"strings"
)

func GetFolderMime() string {
	return "application/vnd.google-apps.folder"
}

func IsFolder(file contracts.File) bool {
	return file.MimeType == GetFolderMime()
}

func CanDownloadFile(file contracts.File) bool {
	return !strings.Contains(file.MimeType, "application/vnd.google-apps")
}
