# bookindex
Generate the index for my personal library

## Build

Build the Linux amd64 binary for the VPS:

```sh
make build
```

Build for the current machine:

```sh
make build-local
```

Build for an ARM64 VPS:

```sh
make build-linux-arm64
```

The compiled binary is written to `dist/`.

## Deploy

Build and copy the Linux amd64 binary to the VPS:

```sh
make deploy VPS=my-ssh-host
```

To copy an already-built binary:

```sh
make push VPS=my-ssh-host
```

`VPS` can be any host alias from your SSH config. The deploy target copies the binary to `$(VPS):/usr/local/bin/bookindex`.
