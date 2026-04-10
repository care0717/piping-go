# piping-go

Transfer data between devices via HTTP. Zero dependencies, single binary.

A Go reimplementation of [piping-server](https://github.com/nwtgck/piping-server) and [piping-ui-web](https://github.com/nwtgck/piping-ui-web) as a single self-contained binary. The original projects by [@nwtgck](https://github.com/nwtgck) pioneered the idea of transferring data between devices using pure HTTP.

A sender and receiver connect to the same path, and data streams through in real time. Works with `curl`, browsers, or any HTTP client.

## Install

Download a binary from [Releases](https://github.com/care0717/piping-go/releases) or build from source:

```bash
go build -o piping-go .
```

## Usage

Start the server:

```bash
./piping-go
```

Default port is `8888`. Override with the `PORT` environment variable:

```bash
PORT=3000 ./piping-go
```

### curl

Send a file:

```bash
curl -T myfile.txt http://localhost:8888/mysecret
```

Receive:

```bash
curl http://localhost:8888/mysecret > myfile.txt
```

Pipe text:

```bash
echo "hello" | curl -T - http://localhost:8888/mysecret
```

Send to multiple receivers:

```bash
# sender
curl -T myfile.txt "http://localhost:8888/mysecret?n=3"

# 3 receivers
curl http://localhost:8888/mysecret?n=3 > copy1.txt
curl http://localhost:8888/mysecret?n=3 > copy2.txt
curl http://localhost:8888/mysecret?n=3 > copy3.txt
```

### Web UI

Open http://localhost:8888/ in your browser.

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | Web UI |
| `GET /version` | Version string |
| `GET /help` | Usage help |
| `POST,PUT /<path>` | Send data |
| `GET /<path>` | Receive data |

## Acknowledgments

This project is inspired by and based on the work of [@nwtgck](https://github.com/nwtgck):

- [piping-server](https://github.com/nwtgck/piping-server) - The original Piping Server (Node.js)
- [piping-ui-web](https://github.com/nwtgck/piping-ui-web) - The original Web UI (Vue.js)

## License

MIT
