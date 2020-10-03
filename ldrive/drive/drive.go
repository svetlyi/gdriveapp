package drive

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	fileRepo "github.com/svetlyi/gdriveapp/ldrive/db/repository"
	lfileHash "github.com/svetlyi/gdriveapp/ldrive/file/hash"
	"io"
	"os"
	"path/filepath"
)

type Drive struct {
	drivePath string

	logger         contracts.Logger
	fileRepository fileRepo.Repository

	log contracts.Logger
}

func New(drivePath string, logger contracts.Logger, fileRepository fileRepo.Repository) *Drive {
	return &Drive{
		drivePath:      drivePath,
		logger:         logger,
		fileRepository: fileRepository,
	}
}

func (d Drive) GetFilesToCheck(ctx context.Context, files contracts.FilesInterfaceChan, errCh contracts.ErrorChan) {
	defer close(errCh)
	defer close(files)
	root, err := d.fileRepository.GetRootFolder()

	if errors.Cause(err) == sql.ErrNoRows {
		return
	} else if err != nil {
		errCh <- errors.Wrap(err, "could not get root folder")
		return
	}
	err = filepath.Walk(
		root.GetFullPath(),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.Wrapf(err, "cold not walk in path %s", path)
			}
			// we are not interested in the directory where our drive is
			if d.drivePath == path {
				return nil
			}
			d.logger.Debug("next local path", path)
			curRelativeFilePath := path[len(d.drivePath):]
			fileInDb, err := d.fileRepository.GetFileByRelPath(curRelativeFilePath)
			if errors.Cause(err) == sql.ErrNoRows {
				fileInDb, err = d.fileRepository.CreateFile(curRelativeFilePath, info.ModTime(), uint64(info.Size()))
				if err != nil {
					return errors.Wrapf(err, "cold not create file %s", path)
				}
			} else if err != nil {
				return errors.Wrapf(err, "cold not get download time for %s", path)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case files <- fileInDb:
			}

			return nil
		},
	)
	if err != nil {
		errCh <- errors.Wrap(err, "error while traversing")
	}
}

func (d *Drive) Create(ctx context.Context, file contracts.File) error {
	fileFullPath := file.GetFullPath()
	err := d.writeFile(file)
	if err != nil {
		return errors.Wrapf(err, "could not write file %s", fileFullPath)
	}
	stat, err := os.Stat(fileFullPath)
	if err != nil {
		return errors.Wrapf(err, "could not get stat info for %s", file.GetFullPath())
	}

	_, err = d.fileRepository.CreateFile(file.GetRelativePath(), stat.ModTime(), uint64(stat.Size()))
	return err
}

func (d *Drive) Update(_ context.Context, file contracts.File) error {
	fileFullPath := file.GetFullPath()
	err := d.writeFile(file)
	if err != nil {
		return errors.Wrapf(err, "could not write file %s", fileFullPath)
	}
	stat, err := os.Stat(fileFullPath)
	if err != nil {
		return errors.Wrapf(err, "could not get stat info for %s", file.GetFullPath())
	}

	return d.fileRepository.Update(file, stat.ModTime(), uint64(stat.Size()))
}

func (d *Drive) writeFile(file contracts.File) error {
	if same, err := d.isLocalTheSame(file); err != nil {
		return err
	} else if same {
		return nil
	}
	lf, err := os.OpenFile(file.GetFullPath(), os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return errors.Wrapf(err, "could not create file %s", file.GetFullPath())
	}
	defer lf.Close()

	reader, err := file.GetReader()
	if err != nil {
		return err
	}
	buf := make([]byte, 1024)
	for {
		// read a chunk
		n, err := reader.Read(buf)

		// if everything is wrong, we will try to remove the incorrectly downloaded parts of the file
		if err != nil && err != io.EOF {
			if remErr := os.Remove(file.GetFullPath()); remErr != nil {
				return errors.Wrapf(err, "could not remove failed file %s", file.GetFullPath())
			}
			return errors.Wrapf(err, "unable to read file into %s", file.GetFullPath())
		}
		if n == 0 {
			break
		}

		// write a chunk
		if _, err := lf.Write(buf[:n]); err != nil {
			if remErr := os.Remove(file.GetFullPath()); remErr != nil {
				return errors.Wrapf(remErr, "unable to remove file %s", file.GetFullPath())
			}
			return errors.Wrapf(err, "could not write a chunk into %s", file.GetFullPath())
		}
	}

	return nil
}

func (d *Drive) Delete(_ context.Context, file contracts.File) error {
	return os.Remove(file.GetFullPath())
}

func (d *Drive) RenameConflicted(_ context.Context, file contracts.File, maxCopies uint16) (string, error) {
	fileOrigName := file.GetName()
	var (
		i       uint16
		newName string
		ext     = filepath.Ext(fileOrigName)
		err     error
	)
	for i = 0; i < maxCopies; i++ {
		newName = fmt.Sprintf("%s (%d)%s", fileOrigName[:len(fileOrigName)-len(ext)], i, ext)
		newFullName := filepath.Join(filepath.Dir(file.GetFullPath()), newName)
		_, err = os.Stat(newFullName)
		if os.IsNotExist(err) {
			err = os.Rename(file.GetFullPath(), newFullName)
			if err != nil {
				return "", errors.Wrapf(err, "could not rename local file %s to %s", file.GetFullPath(), newFullName)
			}
			stat, err := os.Stat(newFullName)
			if err != nil {
				return "", errors.Wrapf(err, "could not get stats from file %s", newFullName)
			}
			return newName, d.fileRepository.Rename(file, newFullName, stat.ModTime())
		}
	}

	return "", errors.New("could not rename file. Attempts limit exceeded")
}

func (d *Drive) Rename(_ context.Context, file contracts.File, newName string) error {
	fileOrigFullPath := file.GetFullPath()
	newFullPath := filepath.Join(filepath.Dir(fileOrigFullPath), newName)
	_, err := os.Stat(newFullPath)
	if err == nil {
		return errors.Wrapf(err, "file %s already exists", newName)
	}
	err = os.Rename(fileOrigFullPath, newFullPath)
	if err != nil {
		return errors.Wrapf(err, "could not rename local file %s to %s", fileOrigFullPath, newFullPath)
	}
	stat, err := os.Stat(newFullPath)
	if err != nil {
		return errors.Wrapf(err, "could not get stats from file %s", newFullPath)
	}
	return d.fileRepository.Rename(file, newFullPath, stat.ModTime())
}

func (d *Drive) GetByFullPath(path string) (contracts.File, error) {
	return d.fileRepository.GetFileByRelPath(path[len(d.drivePath):])
}

// isLocalTheSame checks that a file with the same path, name and hash exists
// if it exists, we won't download it. We need it when for some reason the database was empty
// or the downloaded time in the database is null
func (d *Drive) isLocalTheSame(remoteFile contracts.File) (bool, error) {
	remoteFileFullPath := remoteFile.GetFullPath()
	stat, err := os.Stat(remoteFileFullPath)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, errors.Wrapf(err, "could not check if the remoteFile %s exists", remoteFileFullPath)
		}
	}

	if stat.IsDir() {
		return true, nil
	}
	fileHash, err := remoteFile.GetHash()
	if err != nil {
		return false, errors.Wrapf(err, "could not compare local remoteFile %s with remote", remoteFileFullPath)
	}

	if localHash, err := lfileHash.CalcCachedHash(remoteFileFullPath); nil != err {
		return false, err
	} else {
		//d.log.Debug(fmt.Sprintf("calculated localHash for %s: %s", remoteFileFullPath, localHash))
		return fileHash == localHash, nil
	}
}
