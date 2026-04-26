# Installation

## Docker (Recommended)

```bash
docker run -d \
  -p 64580:80 \
  -v caslink-config:/config \
  -v caslink-data:/data \
  casapps/caslink:latest
```

## Binary

Download the latest release for your platform:

```bash
# Linux
wget https://github.com/casapps/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64
./caslink-linux-amd64
```

## Building from Source

Requires Docker (no Go installation needed):

```bash
git clone https://github.com/casapps/caslink
cd caslink
make build
./binaries/caslink
```

## First Run

On first run, you'll be redirected to `/setup` to create the primary admin account.
