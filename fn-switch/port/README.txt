BRICK PRO FN CONFIG - goi port cho Knulli (TrimUI Brick Pro)
===========================================================

CACH CAI (khong can SSH):
-------------------------
1. Copy toan bo noi dung trong thu muc nay vao:
     /userdata/roms/ports/
   (ket qua se co: BrickProFNConfig.sh, gameinfo.xml va thu muc
    brickprofnconfig/ nam truc tiep trong roms/ports/)

2. Khoi dong lai EmulationStation (hoac khoi dong lai may), vao menu
   Ports -> chon "Brick Pro FN Config".

Lan dau chay,_port se tu dong:
  - cai fn-daemon, brick-config va trimui-volume-attenuate vao
    /userdata/system/fn_switch/
  - tao config.json mac dinh (NEU da co thi GIU NGUYEN cau hinh cua ban)
  - cai service /userdata/system/services/fn_switch
  - them "fn_switch" vao system.services de tu chay luc boot
  - khoi dong daemon ngay (khong can reboot)
  - mo giao dien cau hinh

Cac lan sau chi mo giao dien cau hinh; daemon van tu chay luc boot.

NOTE: neu /userdata/roms/ports/ da co san gameinfo.xml, dung noi dung cua
file nay them vao file do (them the <game> ...) thay vi ghi de, de khong
mat thong tin cac port khac.

FILE LOG:
  /userdata/system/logs/fn-service.log
  /userdata/system/logs/fn-daemon.log

GO DIEN:
  - Tab FN: gat can FN (ON/OFF) -> snapshot / khoi phuc CPU, sang, am luong, LED.
  - Tab NUT PHU: gan F1 / F2 / Home thanh mute, screenshot, LED, RetroArch...
  - Tab AUDIO: chinh DAC volume / headphone gain, co nut test tieng (tone).
