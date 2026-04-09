#!/bin/bash

echo "=== Pan Player CMD 构建和测试脚本 ==="

# 1. 检查 Go 环境
echo "1. 检查 Go 环境..."
go version

# 2. 下载依赖
echo -e "\n2. 下载依赖..."
go mod download

# 3. 编译项目
echo -e "\n3. 编译项目..."
go build -o pan-player ./cmd/pan-player

if [ $? -eq 0 ]; then
    echo "✓ 编译成功！"
else
    echo "✗ 编译失败"
    exit 1
fi

# 4. 检查可执行文件
echo -e "\n4. 检查可执行文件..."
ls -lh pan-player

# 5. 提示运行
echo -e "\n5. 运行程序..."
echo "执行: ./pan-player"
echo ""
echo "✓ API 凭证已内置在程序中，直接运行即可"
echo "首次运行时使用手机百度网盘 APP 扫描二维码登录"
