package contracts

import "time"

type File struct {
	Id             string
	PrevRemoteName string
	CurRemoteName  string
	// full path of the file
	PrevPath string
	CurPath  string
	Hash     string
	// when the last time the file was downloaded. It is actually
	// the last local modification date
	DownloadTime time.Time
	// previous modification time. After synchronization the CurRemoteModTime
	// field is populated with the actual modification time. After we see, if the fields
	// PrevRemoteModTime and CurRemoteModTime are not the same, it means
	// the remote file has been changed since the last synchronization.
	PrevRemoteModTime time.Time
	// after synchronization with the remote drive, the field is filled with
	// the last modification time on the server. Having the field, we can say if it
	// has been changed since the last synchronization.
	CurRemoteModTime time.Time
	MimeType         string
	Shared           uint8
	RootFolder       uint8
	SizeBytes        uint64
	RemovedRemotely  uint8
	// if it was placed to trash
	Trashed uint8
}

type FilesChan chan File

type FileChangeType string

const (
	FILE_DELETED     FileChangeType = "deleted"
	FILE_UPDATED     FileChangeType = "updated"
	FILE_NOT_CHANGED FileChangeType = "not_changed"
	FILE_NOT_EXIST   FileChangeType = "not_exist"
	FILE_ERROR       FileChangeType = "error"
)
