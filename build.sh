#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

install_go() {
	local arch os go_url go_file

	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	arch="$(uname -m)"
	case "$arch" in
		x86_64)  arch=amd64 ;;
		aarch64|arm64) arch=arm64 ;;
		*) echo "Unsupported arch: $arch"; exit 1 ;;
	esac

	echo "Go not found. Downloading and installing Go for ${os}/${arch}..."

	go_url="https://go.dev/dl/$(curl -sL 'https://go.dev/VERSION?m=text').${os}-${arch}.tar.gz"
	go_file="/tmp/$(basename "$go_url")"

	curl -#Lo "$go_file" "$go_url"
	sudo rm -rf /usr/local/go
	sudo tar -C /usr/local -xzf "$go_file"
	rm -f "$go_file"

	export PATH="/usr/local/go/bin:$PATH"
	echo "Go installed: $(go version)"
}

if ! command -v go &>/dev/null; then
	if [ -x /usr/local/go/bin/go ]; then
		export PATH="/usr/local/go/bin:$PATH"
	else
		install_go
	fi
fi

mkdir -p bin

echo "Building server..."
go build -o bin/server ./server/main.go

echo "Building implant..."
go build -o bin/implant ./implant/main.go

echo "Building build-implant..."
go build -o bin/build-implant ./cmd/build-implant/main.go

install_upx() {
	local os arch upx_url upx_file

	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	arch="$(uname -m)"
	case "$arch" in
		x86_64)  arch=amd64 ;;
		aarch64|arm64) arch=arm64 ;;
	esac

	echo "UPX not found. Downloading UPX for ${os}/${arch}..."

	upx_url="https://github.com/upx/upx/releases/download/v5.0.0/upx-5.0.0-${arch}_${os}.tar.xz"
	upx_file="/tmp/$(basename "$upx_url")"

	curl -#Lo "$upx_file" "$upx_url"
	sudo tar -C /usr/local -xJf "$upx_file" --strip-components=1 upx-5.0.0-${arch}_${os}/upx
	rm -f "$upx_file"

	export PATH="/usr/local/bin:$PATH"
	echo "UPX installed: $(upx --version | head -1)"
}

if ! command -v upx &>/dev/null; then
	if [ -x /usr/local/bin/upx ]; then
		export PATH="/usr/local/bin:$PATH"
	else
		install_upx
	fi
fi

echo "Compressing server..."
upx --best --lzma bin/server 2>/dev/null || true

echo "Compressing implant..."
upx --best --lzma bin/implant 2>/dev/null || true

echo "Compressing build-implant..."
upx --best --lzma bin/build-implant 2>/dev/null || true

echo "Done. Binaries in ./bin/"
