package client

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/RGood/fs-xfer/pkg/files"
	"github.com/RGood/fs-xfer/pkg/generated/filesystem"
	"google.golang.org/grpc"
)

type StorageClient struct {
	c filesystem.StorageServiceClient
}

func NewStorageClient(conn *grpc.ClientConn) *StorageClient {
	return &StorageClient{
		c: filesystem.NewStorageServiceClient(conn),
	}
}

func (s *StorageClient) Download(ctx context.Context, remotePath string, localPath string) (int64, error) {
	downloadClient, err := s.c.Download(ctx, &filesystem.DownloadRequest{Path: remotePath})
	if err != nil {
		return 0, err
	}

	var totalSize int64

	downloadClient.CloseSend()

	curFilename := ""
	var curFile *os.File

	for {
		file, err := downloadClient.Recv()
		if err != nil {
			break
		}

		fullFileName := path.Join(localPath, file.GetPath(), file.GetName())

		if fullFileName != curFilename {
			curFile.Close()

			dir := path.Dir(fullFileName)
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				return 0, err
			}

			f, err := os.OpenFile(fullFileName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return 0, err
			}

			curFile = f
			curFilename = fullFileName
		}

		b, err := curFile.Write(file.GetData())
		if err != nil {
			panic(err)
		}
		totalSize += int64(b)
	}

	return totalSize, nil
}

func (s *StorageClient) Upload(ctx context.Context, localPath string) (*filesystem.UploadFilesystemResponse, error) {
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uploadClient, err := s.c.Upload(cancelCtx)
	if err != nil {
		return nil, err
	}

	fileChan := make(chan *files.FileProgress)
	var streamErr error

	go func() {
		streamErr = files.Stream(localPath, fileChan)
		close(fileChan)
	}()

	for p := range fileChan {
		uploadClient.Send(p.File)
	}

	if streamErr != nil {
		return nil, streamErr
	}

	res, err := uploadClient.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("could not receive upload response: %v", err)
	}

	return res, nil
}

func (s *StorageClient) GetManifest(ctx context.Context, remotePath string, recursive bool) (*filesystem.ManifestResponse, error) {
	// Implementation for retrieving the manifest
	manifest, err := s.c.GetManifest(ctx, &filesystem.ManifestRequest{Path: remotePath, Recursive: recursive})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve manifest: %v", err)
	}
	return manifest, nil
}
