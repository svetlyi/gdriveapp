package contracts

type Drive interface {
	GetFilesList(chan *File)
	DownloadFile(File)
	UploadFile(File)
	DeleteFile(File)
	DetermineChangedFiles(chan *File)
	GetStats() Stats
}

type Stats struct {
	totalSpace, freeSpace, occupiedSpace int64
}
