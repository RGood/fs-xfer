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

func NewLocalStorageService(root string) *StorageService {
	return &StorageService{
		root: filepath.Clean(root),
	}
}

func (s *StorageService) Upload(stream filesystem.StorageService_UploadServer) error {
	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(stream.Context(), 60*time.Second)
	defer cancel()

	totalSize := 0

	curFileName := ""
	var curFile *os.File

	for file, err := stream.Recv(); err != io.EOF; file, err = stream.Recv() {
		if err != nil {
			println("Upload exited unsuccessfully. Deleting directory:", path.Join(s.root, id), err.Error())
			os.RemoveAll(path.Join(s.root, id))
			return err
		} else if ctx.Err() != nil {
			println("Upload exited due to context error. Deleting directory:", path.Join(s.root, id), ctx.Err().Error())
			os.RemoveAll(path.Join(s.root, id))
			return ctx.Err()
		}

		cleanedPath := filepath.Clean(file.Path)
		fullFileName := path.Join(s.root, id, cleanedPath, file.Name)

		if curFileName != fullFileName {
			curFile.Close()
			curFileName = fullFileName

			if err := os.MkdirAll(path.Dir(fullFileName), os.ModeDir); err != nil {
				return err
			}

			f, err := os.OpenFile(fullFileName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				println("Error creating file:", fullFileName)
				return err
			}

			curFile = f
		}

		b, err := curFile.Write(file.Data)
		if err != nil {
			return err
		}
		//fmt.Printf("Wrote %d bytes to `%s`\n", b, fullFileName)
		totalSize += b
	}

	curFile.Close()

	fmt.Printf("Upload complete. %s bytes stored in: %s\n", units.FormatBytesIEC(int64(totalSize)), path.Join(s.root, id))

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

type entryWithPath struct {
	os.FileInfo
	path string
}

func mapToEntryWithPath(entries []os.FileInfo, root string) []entryWithPath {
	manifestEntries := make([]entryWithPath, len(entries))
	for i, entry := range entries {
		manifestEntries[i] = entryWithPath{
			FileInfo: entry,
			path:     root,
		}
	}
	return manifestEntries
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

	files := map[string]struct{}{}

	entries, err := f.Readdir(0)
	if err != nil {
		return nil, err
	}

	manifestEntries := mapToEntryWithPath(entries, ".")

	for len(manifestEntries) > 0 {
		entry := manifestEntries[0]
		manifestEntries = manifestEntries[1:]

		if !entry.IsDir() {
			files[path.Join(entry.path, entry.Name())] = struct{}{}
		} else {
			f, err := os.OpenFile(path.Join(basePath, entry.path, entry.Name()), os.O_RDONLY, os.ModePerm)
			if err != nil {
				return nil, err
			}

			newEntries, err := f.Readdir(0)
			if err != nil {
				return nil, err
			}

			files[path.Join(entry.path, entry.Name())+"/"] = struct{}{}

			if req.Recursive {
				manifestEntries = append(manifestEntries, mapToEntryWithPath(newEntries, path.Join(entry.path, entry.Name()))...)
			}
		}
	}

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}

	return &filesystem.ManifestResponse{Files: keys}, nil
}
