package contracts

import "context"

// Drive is an interface for any kind of drive the application might want to synchronize with.
// If we have some cloud and we want to synchronize, let's say, local drive with some remote drive,
// we would have to just implement the interface and pass the implementation to the synchronizer.
type Drive interface {
	// GetFilesToCheck sends all the changed or just files, that need to be checked, to `files` channel
	GetFilesToCheck(ctx context.Context, files FilesInterfaceChan, err ErrorChan)
	Create(ctx context.Context, file File) error
	Update(ctx context.Context, file File) error
	Delete(ctx context.Context, file File) error
	// RenameConflicted renames the conflicted file. If there is a change in multiple drives of the same file,
	// one of the conflicted files will be renamed, using the function
	RenameConflicted(ctx context.Context, file File, maxCopies uint16) (newName string, err error)
	Rename(ctx context.Context, file File, newName string) error
	GetByFullPath(path string) (File, error)
}
