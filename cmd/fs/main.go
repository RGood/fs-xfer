package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RGood/fs-xfer/pkg/client"
	"github.com/RGood/fs-xfer/pkg/units"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

func prettyPrintManifest(entries []client.FSEntry, depth int) {
	if entries == nil {
		return
	}

	client.SortEntries(entries)
	for _, entry := range entries {
		fmt.Printf("%s%s\n", strings.Repeat("  ", depth), entry.GetName())
		prettyPrintManifest(entry.GetChildren(), depth+1)
	}
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

	if len(args) < 3 {
		printHelp()
		return
	}

	url := args[1]
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		panic(err)
	}

	c := client.NewStorageClient(conn)

	if strings.ToLower(args[2]) == "upload" {
		if len(args) != 4 {
			fmt.Println("Usage: fs <url> upload <folder>")
			return
		}
		remoteAddr, size, err := c.Upload(context.Background(), resolveHomeDir(args[3]))
		if err != nil {
			panic(err)
		}
		fmt.Printf("Uploaded %s bytes to %s\n", units.FormatBytesIEC(size), remoteAddr)
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

		manifest, err := c.GetManifest(context.Background(), manifestArgs.Arg(0), *recursive)
		if err != nil {
			panic(err)
		}

		prettyPrintManifest(manifest, 0)
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
		size, err := c.Download(context.Background(), parts[0], resolvedFolder)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Downloaded %s bytes to %s\n", units.FormatBytesIEC(size), resolvedFolder)
	} else {
		println("Unknown command:", args[2])
		printHelp()
		return
	}
}
