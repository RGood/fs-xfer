package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/RGood/fs-xfer/pkg/generated/filesystem"
)

const maxChunkSize = 1024 * 256 // 256 KiB

type FileProgress struct {
	TotalChunks int64
	Chunk       int64
	File        *filesystem.File
}

func Stream(parentDir string, file *os.File, fileChan chan<- *FileProgress) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}

	fileName := info.Name()
	filePath := path.Join(parentDir, fileName)

	if info.IsDir() {
		entries, err := file.ReadDir(0)
		if err != nil {
			return err
		}

		errs := make([]error, len(entries))

		for i, entry := range entries {
			entryPath := path.Join(filePath, entry.Name())
			f, err := os.OpenFile(entryPath, os.O_RDONLY, os.ModePerm)
			if err != nil {
				return err
			}

			errs[i] = Stream(filePath, f, fileChan)
		}

		return errors.Join(errs...)
	} else {
		chunks := info.Size() / maxChunkSize
		if info.Size()%maxChunkSize != 0 {
			chunks++
		}

		if chunks == 0 {
			chunks = 1
		}

		for i := int64(0); i < chunks; i++ {
			data := make([]byte, maxChunkSize)
			n, err := file.ReadAt(data, int64(i*maxChunkSize))
			if err != nil && err != io.EOF {
				return fmt.Errorf("error reading file `%s`: %v", filePath, err)
			}

			fileChan <- &FileProgress{
				TotalChunks: chunks,
				Chunk:       i,
				File: &filesystem.File{
					Name: info.Name(),
					Path: parentDir,
					Data: data[:n],
				},
			}
		}
	}

	return nil
}
