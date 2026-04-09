#!/bin/bash
# 自动下载 ffmpeg 静态二进制文件
# 支持多平台：macOS, Linux, Windows

set -e

FFMPEG_DIR="third_party/ffmpeg"
mkdir -p "$FFMPEG_DIR"

OS=$(uname -s)
ARCH=$(uname -m)

echo "检测到系统: $OS ($ARCH)"

if [ "$OS" = "Darwin" ]; then
    # macOS
    if [ "$ARCH" = "arm64" ]; then
        URL="https://evermeet.cx/ffmpeg/getrelease/ffmpeg/zip"
        FILENAME="ffmpeg-macos-arm64.zip"
    else
        URL="https://evermeet.cx/ffmpeg/getrelease/ffmpeg/zip"
        FILENAME="ffmpeg-macos-amd64.zip"
    fi

    echo "下载 ffmpeg for macOS..."
    curl -L "$URL" -o "$FFMPEG_DIR/$FILENAME"
    cd "$FFMPEG_DIR"
    unzip -o "$FILENAME"
    chmod +x ffmpeg
    cd -

elif [ "$OS" = "Linux" ]; then
    # Linux
    URL="https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"
    FILENAME="ffmpeg-linux-amd64.tar.xz"

    echo "下载 ffmpeg for Linux..."
    curl -L "$URL" -o "$FFMPEG_DIR/$FILENAME"
    cd "$FFMPEG_DIR"
    tar -xf "$FILENAME" --strip-components=1
    chmod +x ffmpeg
    cd -

else
    echo "不支持的操作系统: $OS"
    echo "请手动安装 ffmpeg: https://ffmpeg.org/download.html"
    exit 1
fi

echo "✓ ffmpeg 已下载到 $FFMPEG_DIR/ffmpeg"
