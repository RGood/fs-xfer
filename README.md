# fs-xfer

Lightweight Golang gRPC service for transferring files between remote systems.

Supports arbitrary file size by transferring in chunks.

## Usage

### Help

`fs`

### Upload

`fs <address> upload <local_path>`

### Download

Download folder or file from remote server to local filesystem.

`fs <address> download <remote_path>:<local_path>`

### List

`fs <address> ls|manifest [-r] <remote_path>`

## Development

### Prerequisites

1. Running docker engine and have docker-compose installed
2. Have protoc & protoc-gen-go installed

### Run local server

`make run`

### Build & Install Client

`make client`

### Regenerate protos

`make protos`