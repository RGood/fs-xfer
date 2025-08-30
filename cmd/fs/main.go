package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RGood/fs-xfer/pkg/files"
	"github.com/RGood/fs-xfer/pkg/generated/filesystem"
	"github.com/RGood/fs-xfer/pkg/units"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func progressBar(cur, max int64) string {
	progress := 100 * cur / max
	bar := fmt.Sprintf("[%3d%%]", progress)
	return bar
}

func resolveHomeDir(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		return strings.Replace(path, "~", homeDir, 1)
	}
	return path
}

func upload(url string, folder string) {
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := filesystem.NewStorageServiceClient(conn)

	uploadClient, err := client.Upload(context.Background())
	if err != nil {
		panic(err)
	}

	fileChan := make(chan *files.FileProgress)

	go func() {
		if err := files.Stream(".", fileChan); err != nil {
			panic(err)
		}
		close(fileChan)
	}()

	for p := range fileChan {
		output := fmt.Sprintf("%s %s", progressBar(p.Chunk, p.TotalChunks), path.Join(p.File.Path, p.File.Name))
		outputLen := len(output)
		print(output)
		uploadClient.Send(p.File)
		//time.Sleep(10 * time.Millisecond)
		os.Stdout.Sync()
		print("\r" + strings.Repeat(" ", outputLen))
		print("\r")
		os.Stdout.Sync()
	}

	res, err := uploadClient.CloseAndRecv()
	if err != nil {
		panic(fmt.Errorf("could not receive upload response: %v", err))
	}
	fmt.Printf("Upload complete. %s uploaded to dir: %s\n", units.FormatBytesIEC(res.GetSize()), res.GetId())
}

func download(url string, remoteFolder, localFolder string) {
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := filesystem.NewStorageServiceClient(conn)

	downloadClient, err := client.Download(context.Background(), &filesystem.DownloadRequest{Path: remoteFolder})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	totalSize := 0

	downloadClient.CloseSend()

	curFilename := ""
	var curFile *os.File

	for {
		file, err := downloadClient.Recv()
		if err != nil {
			break
		}

		fullFileName := path.Join(localFolder, file.GetPath(), file.GetName())

		if fullFileName != curFilename {
			curFile.Close()

			dir := path.Dir(fullFileName)
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				panic(err)
			}

			f, err := os.OpenFile(fullFileName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				panic(err)
			}

			curFile = f
			curFilename = fullFileName
		}

		b, err := curFile.Write(file.GetData())
		if err != nil {
			panic(err)
		}
		totalSize += b
	}

	fmt.Printf("Downloaded %s bytes to %s\n", units.FormatBytesIEC(int64(totalSize)), localFolder)
}

type folder struct {
	name    string
	folders map[string]*folder
	files   map[string]struct{}
}

func newFolder(name string) *folder {
	return &folder{
		name:    name,
		folders: make(map[string]*folder),
		files:   make(map[string]struct{}),
	}
}

func (f *folder) addFile(name string) {
	f.files[name] = struct{}{}
}

func (f *folder) getFolder(name string) *folder {
	if folder, ok := f.folders[name]; ok {
		return folder
	} else {
		f.folders[name] = newFolder(name)
		return f.folders[name]
	}
}

func structureManifest(files []string) *folder {
	manifest := newFolder(".")

	for _, fullPath := range files {
		parts := strings.Split(fullPath, "/")
		current := manifest

		for i, part := range parts {
			if len(parts)-1 == i {
				if part == "" {
					continue
				}
				current.addFile(part)
			} else {
				current = current.getFolder(part)
			}
		}
	}

	return manifest
}

func prettyPrintManifest(manifest *folder, indent int) {
	folderKeys := make([]string, 0, len(manifest.folders))
	for k := range manifest.folders {
		folderKeys = append(folderKeys, k)
	}
	sort.Strings(folderKeys)

	for _, name := range folderKeys {
		fmt.Printf("%s%s/\n", strings.Repeat("  ", indent), name)
		prettyPrintManifest(manifest.folders[name], indent+1)
	}

	fileNames := make([]string, 0, len(manifest.files))
	for k := range manifest.files {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		fmt.Printf("%s%s\n", strings.Repeat("  ", indent), name)
	}
}

func manifest(url, path string, recursive bool) {
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := filesystem.NewStorageServiceClient(conn)
	res, err := client.GetManifest(
		context.Background(),
		&filesystem.ManifestRequest{
			Path:      path,
			Recursive: recursive,
		})
	if err != nil {
		panic(err)
	}

	manifest := structureManifest(res.GetFiles())
	prettyPrintManifest(manifest, 0)
}

func printHelp() {
	fmt.Println("Usage: fs <remote_host> <command> [flags] <args>")
	fmt.Println("Commands:")
	fmt.Println("  upload <local_folder>              Upload a folder to the target url")
	fmt.Println("  cp <folder>:<local_folder>         Download a folder from the target url")
	fmt.Println("  ls [-r] <folder>                   List the manifest of a remote folder")
	fmt.Println("  help                               Show this help message")
}

func main() {
	args := os.Args

	if len(args) < 2 {
		printHelp()
		return
	}

	if strings.ToLower(args[2]) == "upload" {
		if len(args) != 4 {
			fmt.Println("Usage: fs <url> upload <folder>")
			return
		}
		upload(args[1], args[3])

	} else if strings.ToLower(args[2]) == "help" {
		printHelp()
	} else if strings.ToLower(args[2]) == "manifest" || strings.ToLower(args[2]) == "ls" {
		manifestArgs := flag.NewFlagSet("manifest", flag.ExitOnError)
		recursive := manifestArgs.Bool("r", false, "List files recursively")
		manifestArgs.Parse(args[3:])

		if len(args) < 4 {
			fmt.Println("Usage: fs <url> ls <folder>")
			return
		}

		manifest(args[1], manifestArgs.Arg(0), *recursive)
	} else if strings.ToLower(args[2]) == "cp" || strings.ToLower(args[2]) == "download" {
		if len(args) != 4 {
			fmt.Println("Usage: fs <url> cp <folder>:<local_folder>")
			return
		}

		parts := strings.SplitN(args[3], ":", 2)
		if len(parts) != 2 {
			fmt.Println("Usage: fs <url> cp <folder>:<local_folder>")
			return
		}

		resolvedFolder, err := filepath.Abs(resolveHomeDir(parts[1]))
		if err != nil {
			panic(err)
		}

		println("Downloading", parts[0], "to", resolvedFolder)

		download(args[1], parts[0], resolvedFolder)
	} else {
		println("Unknown command:", args[2])
		printHelp()
		return
	}
}
