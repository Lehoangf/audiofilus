#!/bin/bash
# ==============================================================================
# Script cài đặt tự động FN-Switch Daemon cho TrimUI Brick Pro trên Knulli OS
# ==============================================================================

INSTALL_DIR="/userdata/system/fn_switch"
CUSTOM_SH="/userdata/system/custom.sh"

echo ">>> Bắt đầu cài đặt FN-Switch Daemon vào Knulli..."

# 1. Tạo thư mục cài đặt
mkdir -p "$INSTALL_DIR"

# 2. Copy binary và file cấu hình
cp -f fn-daemon "$INSTALL_DIR/fn-daemon"
chmod +x "$INSTALL_DIR/fn-daemon"

if [ ! -f "$INSTALL_DIR/config.json" ]; then
    cp -f config.json "$INSTALL_DIR/config.json"
    echo ">>> Đã tạo file cấu hình mặc định tại $INSTALL_DIR/config.json"
else
    echo ">>> File cấu hình đã tồn tại tại $INSTALL_DIR/config.json (giữ nguyên cấu hình cũ)"
fi

# 3. Đăng ký tự động chạy lúc boot vào /userdata/system/custom.sh
touch "$CUSTOM_SH"
chmod +x "$CUSTOM_SH"

if ! grep -q "fn-daemon" "$CUSTOM_SH"; then
    echo "" >> "$CUSTOM_SH"
    echo "# Tự động khởi động FN-Switch Daemon" >> "$CUSTOM_SH"
    echo "if [ -x \"$INSTALL_DIR/fn-daemon\" ]; then" >> "$CUSTOM_SH"
    echo "    $INSTALL_DIR/fn-daemon > /tmp/fn-daemon.log 2>&1 &" >> "$CUSTOM_SH"
    echo "fi" >> "$CUSTOM_SH"
    echo ">>> Đã thêm lệnh tự khởi động vào $CUSTOM_SH"
else
    echo ">>> Đã có cấu hình trong $CUSTOM_SH, bỏ qua bước thêm mới."
fi

echo "=============================================================================="
echo "✅ CÀI ĐẶT THÀNH CÔNG!"
echo "=============================================================================="
echo "Cách sử dụng:"
echo "1. Dò tìm mã phím FN switch lần đầu tiên:"
echo "   $INSTALL_DIR/fn-daemon -probe"
echo "   (Sau đó gạt cần FN qua lại, chương trình sẽ tự lưu vào config.json)"
echo ""
echo "2. Chạy thử daemon:"
echo "   $INSTALL_DIR/fn-daemon"
echo ""
echo "3. Kiểm tra thông số máy:"
echo "   $INSTALL_DIR/fn-daemon -status"
echo "=============================================================================="
