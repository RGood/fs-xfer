package main

import (
	"context"
	"fmt"
	"os"
	"path"
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

func upload(url string, folder string) {
	file, err := os.OpenFile(folder, os.O_RDONLY, os.ModePerm)
	if err != nil {
		panic(err)
	}

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
		if err := files.Stream("", file, fileChan); err != nil {
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

	file.Close()

	res, err := uploadClient.CloseAndRecv()
	if err != nil {
		panic(fmt.Errorf("could not receive upload response: %v", err))
	}
	fmt.Printf("Upload complete. %s uploaded to dir: %s\n", units.FormatBytesIEC(res.GetSize()), res.GetId())
}

func download(id string, folder string) {

}

func manifest(id string) {

}

func main() {
	args := os.Args

	if len(args) < 2 {
		fmt.Println("Usage: fs <command> [<args>]")
		fmt.Println("Commands:")
		fmt.Println("  upload <url> <folder>   Upload a folder to the target url")
		fmt.Println("  download <id> <folder>  Download a folder from the server")
		fmt.Println("  manifest <id>           List the manifest of a remote folder")
		return
	}

	if strings.ToLower(args[1]) == "upload" {
		if len(args) != 4 {
			fmt.Println("Usage: fs upload <url> <folder>")
			return
		}
		upload(args[2], args[3])
	}
}
