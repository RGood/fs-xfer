package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/RGood/fs-xfer/pkg/files"
	filesystem "github.com/RGood/fs-xfer/pkg/generated/filesystem"
	"github.com/RGood/fs-xfer/pkg/units"
	"github.com/google/uuid"
)

type StorageService struct {
	filesystem.UnimplementedStorageServiceServer
	root string
}

// NewLocalStorageService creates a new instance of StorageService with the given root directory.
func NewLocalStorageService(root string) *StorageService {
	return &StorageService{
		root: filepath.Clean(root),
	}
}

func (s *StorageService) Upload(stream filesystem.StorageService_UploadServer) (err error) {
	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(stream.Context(), 60*time.Second)
	defer cancel()

	totalSize := 0

	// Cleanup + Logging
	defer func() {
		if err != nil {
			println("Upload failed. Deleting directory:", path.Join(s.root, id), err.Error())
			os.RemoveAll(path.Join(s.root, id))
		} else {
			fmt.Printf("Upload complete. %s bytes stored in: %s\n", units.FormatBytesIEC(int64(totalSize)), path.Join(s.root, id))

		}
	}()

	curFileName := ""
	var curFile *os.File

	for file, err := stream.Recv(); err != io.EOF; file, err = stream.Recv() {
		if err != nil {
			return err
		} else if ctx.Err() != nil {
			return ctx.Err()
		}

		cleanedPath := filepath.Clean(file.Path)
		fullFileName := path.Join(s.root, id, cleanedPath, file.Name)

		if curFileName != fullFileName {
			// Close current file if it exists
			curFile.Close()

			// Ensure parent dir exists
			if err := os.MkdirAll(path.Dir(fullFileName), os.ModeDir); err != nil {
				return err
			}

			// Open the file for writing
			f, err := os.OpenFile(fullFileName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}

			// Update the current file + filename
			curFileName = fullFileName
			curFile = f
		}

		// Write chunk to current file
		b, err := curFile.Write(file.Data)
		if err != nil {
			return err
		}

		totalSize += b
	}

	curFile.Close()

	return stream.SendAndClose(&filesystem.UploadFilesystemResponse{Id: id, Size: int64(totalSize)})
}

func (s *StorageService) Download(req *filesystem.DownloadRequest, stream filesystem.StorageService_DownloadServer) error {
	basePath, err := s.getBasePath(req.GetPath())
	if err != nil {
		return err
	}

	fileStream := make(chan *files.FileProgress)

	go func() {
		files.Stream(basePath, fileStream)
		close(fileStream)
	}()

	for progress := range fileStream {
		progress.File.Path = path.Clean(strings.TrimPrefix(progress.File.Path, basePath))
		if err := stream.Send(progress.File); err != nil {
			return fmt.Errorf("error sending file chunk: %v", err)
		}
	}

	return nil
}

func (s *StorageService) pathIsValid(path string) bool {
	res := strings.HasPrefix(path, s.root)
	return res
}

func (s *StorageService) getBasePath(targetPath string) (string, error) {
	inputPath := path.Clean(targetPath)

	if strings.HasPrefix(inputPath, "..") || inputPath == "." || inputPath == "/" {
		return "", fmt.Errorf("invalid path: %s", targetPath)
	}

	basePath := path.Clean(path.Join(s.root, inputPath))
	if !s.pathIsValid(basePath) {
		// Prevent path traversal attacks
		return "", fmt.Errorf("invalid base path: %s", targetPath)
	}

	return basePath, nil
}

func (s *StorageService) populateManifest(filePath string, dirFile *os.File, recursive bool) ([]*filesystem.FSEntry, error) {
	entries := []*filesystem.FSEntry{}

	files, err := dirFile.Readdir(0)
	if err != nil {
		return nil, err
	}

	for _, fileInfo := range files {
		entry := &filesystem.FSEntry{}
		if fileInfo.IsDir() {
			entries := []*filesystem.FSEntry{}
			if recursive {
				file, err := os.OpenFile(filepath.Join(filePath, fileInfo.Name()), os.O_RDONLY, os.ModePerm)
				if err != nil {
					return nil, err
				}
				defer file.Close()

				entries, err = s.populateManifest(filepath.Join(filePath, fileInfo.Name()), file, true)
				if err != nil {
					return nil, err
				}
			}
			entry.Value = &filesystem.FSEntry_Directory{
				Directory: &filesystem.Directory{
					Name:    fileInfo.Name(),
					Entries: entries,
				},
			}
		} else {
			entry.Value = &filesystem.FSEntry_File{
				File: &filesystem.FileInfo{
					Name: fileInfo.Name(),
				},
			}
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (s *StorageService) GetManifest(ctx context.Context, req *filesystem.ManifestRequest) (*filesystem.ManifestResponse, error) {
	basePath, err := s.getBasePath(req.GetPath())
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(basePath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries, err := s.populateManifest(basePath, f, req.GetRecursive())
	if err != nil {
		return nil, err
	}

	return &filesystem.ManifestResponse{Entries: entries}, nil
}
