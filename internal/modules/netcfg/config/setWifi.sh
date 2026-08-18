#!/bin/sh

fileName="wifiAp"
SNPath="/opt/runtime/control/product_config.yaml"
hostap_conf=/tmp/hostapd_fcu760k.conf
wifiName="760K"
password="codroid123"
CHAN="${FCU760K_5G_CHANNEL:-149}"
BAND="5G"

br0IP=$(ip addr show br0 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -n 1)

if [ -f $fileName ]; then
    wifiName=$(sed -n '1p' $fileName | tr -d '\r' | tr -d ' ')
    password=$(sed -n '2p' $fileName | tr -d '\r' | tr -d ' ')
    ch=$(sed -n '3p' $fileName | tr -d '\r' | tr -d ' ')
    echo "$ch" | grep -Eq '^[0-9]+$' && CHAN=$ch
    band=$(sed -n '4p' $fileName | tr -d '\r' | tr -d ' ')
    [ "$band" = "2.4G" ] && BAND="2.4G"
fi

# 信道和频段对不上时 hostapd 起不来，按频段兜底
if [ "$BAND" = "2.4G" ]; then
    [ "$CHAN" -ge 1 ] 2>/dev/null && [ "$CHAN" -le 13 ] 2>/dev/null || CHAN=6
else
    BAND="5G"
    [ "$CHAN" -ge 36 ] 2>/dev/null || CHAN=149
fi

if [ -f $SNPath ]; then
    sn=$(grep controllerSerialNumber $SNPath | awk '{print $2}' | tr -d '"')
    [ -n "$sn" ] && wifiName=${wifiName}-${sn#*-}
fi

echo "$wifiName"
echo "$password"
echo "$BAND channel=$CHAN"

/opt/shell/fcu760k_init.sh
# 驱动重载后 wlan0 要好几秒才出现，轮询等它，而不是睡死 10 秒
i=0
while ! ifconfig -a 2>/dev/null | grep -q wlan0 && [ $i -lt 12 ]; do i=$((i + 1)); sleep 1; done
/opt/shell/fcu760k_ap.sh "$wifiName" "$password" "$BAND"

# 固定信道后重启 hostapd，再把 wlan0 挂进 br0
[ -f "$hostap_conf" ] && sed -i -e '/^bridge=/d' -e '/^acs_/d' \
    -e "s/^channel=.*/channel=$CHAN/" -e "s/^chanlist=.*/chanlist=$CHAN/" "$hostap_conf"
killall hostapd udhcpd 2>/dev/null
brctl delif br0 wlan0 2>/dev/null
ip addr flush dev wlan0 2>/dev/null
ip link set wlan0 down; sleep 1; ip link set wlan0 up
iw reg set CN 2>/dev/null
hostapd -B "$hostap_conf"

i=0
while [ $i -lt 15 ] && ! hostapd_cli -p /var/run/hostapd status 2>/dev/null | grep -q 'state=ENABLED'; do
    i=$((i + 1)); sleep 1
done
pidof hostapd >/dev/null || { echo "error: hostapd not running"; exit 1; }

if [ -n "$br0IP" ]; then
    # 进 br0，地址交给现场 DHCP
    brctl addif br0 wlan0 2>/dev/null || true

    # 拔线兜底：lan1 没载波时现场 DHCP 够不着，WiFi 直连的电脑拿不到地址。
    # 这时在 br0 上临时起 udhcpd 分一小段地址；插回网线立即停掉，不和现场 DHCP 冲突。
    if ! kill -0 "$(cat /var/run/lan-dhcp-watchdog.pid 2>/dev/null)" 2>/dev/null; then
        (
            echo $$ > /var/run/lan-dhcp-watchdog.pid
            seg=$(echo "$br0IP" | cut -d. -f1-3)
            cat > /tmp/udhcp_br0.conf << EOF
start           $seg.200
end             $seg.220
interface       br0
opt     router  $br0IP
option  subnet  255.255.255.0
option  lease   600
EOF
            while true; do
                if [ "$(cat /sys/class/net/lan1/carrier 2>/dev/null)" = "0" ]; then
                    pidof udhcpd >/dev/null || udhcpd /tmp/udhcp_br0.conf
                else
                    pidof udhcpd >/dev/null && killall udhcpd
                fi
                sleep 3
            done
        ) &
    fi
else
    # 无 br0：回退独立网段，热点仍可连
    ip addr flush dev wlan0 2>/dev/null
    ifconfig wlan0 192.168.6.1 netmask 255.255.255.0 up
    cat > /tmp/udhcp_fcu760k.conf << EOF
start           192.168.6.10
end             192.168.6.254
interface       wlan0
opt        dns        114.114.114.114
option  subnet  255.255.255.0
opt     router  192.168.6.1
option  domain  local
option  lease   864000
EOF
    udhcpd /tmp/udhcp_fcu760k.conf
fi

exit 0
