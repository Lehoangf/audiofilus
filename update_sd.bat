@echo off
chcp 65001 >nul
echo ======================================================================
echo    CẬP NHẬT BRICK PRO FN CONFIG VÀ FN-DAEMON VÀO THẺ NHỚ KNULLI
echo ======================================================================

set TARGET_DRIVE=
for %%d in (D E F G H I J K L M N) do (
    if exist "%%d:\system\fn_switch" (
        set TARGET_DRIVE=%%d:
        goto found
    )
    if exist "%%d:\roms\ports" (
        set TARGET_DRIVE=%%d:
        goto found
    )
)

echo [!] Chưa tìm thấy phân vùng thẻ nhớ Knulli (/userdata).
echo     Vui lòng cắm đầu đọc thẻ nhớ vào máy tính và chạy lại script này!
pause
exit /b 1

:found
echo [+] Đã tìm thấy thẻ nhớ tại ổ đĩa: %TARGET_DRIVE%

echo [*] Đang sao chép fn-daemon...
mkdir "%TARGET_DRIVE%\system\fn_switch" 2>nul
copy /y "fn-switch\fn-daemon" "%TARGET_DRIVE%\system\fn_switch\fn-daemon"

echo [*] Đang sao chép brick-config...
copy /y "dist\brick-config" "%TARGET_DRIVE%\system\fn_switch\brick-config"

echo [*] Đang sao chép config.json...
copy /y "fn-switch\config.json" "%TARGET_DRIVE%\system\fn_switch\config.json"

echo [*] Đang sao chép launcher script vào Ports...
mkdir "%TARGET_DRIVE%\roms\ports" 2>nul
copy /y "fn-switch\BrickProConfig.sh" "%TARGET_DRIVE%\roms\ports\Brick Pro FN Config.sh"

echo ======================================================================
echo [OK] ĐÃ CẬP NHẬT THÀNH CÔNG VÀO THẺ NHỚ (%TARGET_DRIVE%)!
echo      Bây giờ bạn có thể rút thẻ nhớ ra và gắn lại vào máy TrimUI Brick Pro.
echo ======================================================================
pause
