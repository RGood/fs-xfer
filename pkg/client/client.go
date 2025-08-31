package client

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"

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

// Download downloads the remote path to the local path and returns the total size of the downloaded data.
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

// Upload uploads the file or folder at the given path and returns the remote address of the folder and its size.
func (s *StorageClient) Upload(ctx context.Context, localPath string) (string, int64, error) {
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uploadClient, err := s.c.Upload(cancelCtx)
	if err != nil {
		return "", 0, err
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
		return "", 0, streamErr
	}

	res, err := uploadClient.CloseAndRecv()
	if err != nil {
		return "", 0, fmt.Errorf("could not receive upload response: %v", err)
	}

	return res.GetId(), res.GetSize(), nil
}

type File struct {
	name string
}

func (f *File) GetName() string {
	return f.name
}

func (f *File) GetChildren() []FSEntry {
	return nil
}

func (f *File) cmp(e FSEntry) bool {
	if _, ok := e.(*Folder); ok {
		return false
	}
	return f.name < e.GetName()
}

type Folder struct {
	name     string
	children []FSEntry
}

func (f *Folder) GetName() string {
	return f.name + "/"
}

func (f *Folder) GetChildren() []FSEntry {
	return f.children
}

func (f *Folder) cmp(e FSEntry) bool {
	if _, ok := e.(*File); ok {
		return true
	}
	return f.name < e.GetName()
}

type FSEntry interface {
	GetName() string
	GetChildren() []FSEntry
	cmp(FSEntry) bool
}

// SortEntries sorts a list of entries in place
// with directories always coming before files and capitalized
// names coming before lowercase.
func SortEntries(entries []FSEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].cmp(entries[j])
	})
}

func mapEntries(manifestEntries []*filesystem.FSEntry) []FSEntry {
	var entries []FSEntry
	for _, entry := range manifestEntries {
		switch e := entry.Value.(type) {
		case *filesystem.FSEntry_File:
			entries = append(entries, &File{name: e.File.GetName()})
		case *filesystem.FSEntry_Directory:
			entries = append(entries, &Folder{name: e.Directory.GetName(), children: mapEntries(e.Directory.GetEntries())})
		}
	}
	return entries
}

func (s *StorageClient) GetManifest(ctx context.Context, remotePath string, recursive bool) ([]FSEntry, error) {
	// Implementation for retrieving the manifest
	manifest, err := s.c.GetManifest(ctx, &filesystem.ManifestRequest{Path: remotePath, Recursive: recursive})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve manifest: %v", err)
	}
	return mapEntries(manifest.GetEntries()), nil
}
