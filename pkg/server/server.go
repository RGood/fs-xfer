package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

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
		root: root,
	}
}

func (s *StorageService) Upload(stream filesystem.StorageService_UploadServer) error {
	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(stream.Context(), 60*time.Second)
	defer cancel()

	totalSize := 0

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
		if err := os.MkdirAll(path.Dir(fullFileName), os.ModeDir); err != nil {
			return err
		}

		f, err := os.OpenFile(fullFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			println("Error creating file:", fullFileName)
			return err
		}
		b, err := f.Write(file.Data)
		if err != nil {
			return err
		}
		//fmt.Printf("Wrote %d bytes to `%s`\n", b, fullFileName)
		totalSize += b
		f.Close()
	}

	fmt.Printf("Upload complete. %s bytes stored in: %s\n", units.FormatBytesIEC(int64(totalSize)), path.Join(s.root, id))

	return stream.SendAndClose(&filesystem.UploadFilesystemResponse{Id: id, Size: int64(totalSize)})
}

func (s *StorageService) Download(req *filesystem.DownloadRequest, stream filesystem.StorageService_DownloadServer) error {
	return nil
}

func (s *StorageService) GetManifest(ctx context.Context, req *filesystem.ManifestRequest) (*filesystem.ManifestResponse, error) {
	return nil, nil
}
