![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/atony2099/filesrv?display_name=tag&sort=semver) ![Go Build & Test](https://github.com/atony2099/filesrv/workflows/Go%20Build%20&%20Test/badge.svg) [![Maintainability](https://api.codeclimate.com/v1/badges/6c94935600430be08a5a/maintainability)](https://codeclimate.com/github/atony2099/filesrv/maintainability) [![Test Coverage](https://api.codeclimate.com/v1/badges/6c94935600430be08a5a/test_coverage)](https://codeclimate.com/github/atony2099/filesrv/test_coverage)

## filesrv

filesrv is a simple tool to serve a directory over HTTP and automatically refresh web pages in the browser when files change. It can be useful for web development or any situation where you want to quickly share a static website with others.

## Installation

You can install filesrv using go get:

```console
go install github.com/atony2099/filesrv@latest
```

Alternatively, you can download a pre-built binary from the releases page and add it to your PATH.

## Usage

To start the server, run:

```console
filesrv -port [PORT] -dir [DIRECTORY]
```

where [PORT] is the port number to use (default is 8080) and [DIRECTORY] is the directory to serve (default is the current directory).

Once the server is running, open your web browser and navigate to <http://localhost:[PORT>], where [PORT] is the port number you specified. You should see a file browser for the directory you specified.

Whenever a file in the directory is modified, the browser will automatically refresh to show the updated content.
