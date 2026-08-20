#!/bin/sh

fileName="wifiAp"
SNPath="/opt/runtime/control/product_config.yaml"
hostap_conf=/tmp/hostapd_fcu760k.conf
wifiName="760K"
password="codroid123"
CHAN="${FCU760K_5G_CHANNEL:-149}"
BAND="5G"

if [ -f $fileName ]; then
    wifiName=$(sed -n '1p' $fileName | tr -d '\r' | tr -d ' ')
    password=$(sed -n '2p' $fileName | tr -d '\r' | tr -d ' ')
    ch=$(sed -n '3p' $fileName | tr -d '\r' | tr -d ' ')
    case $ch in ''|*[!0-9]*) ;; *) CHAN=$ch ;; esac
    band=$(sed -n '4p' $fileName | tr -d '\r' | tr -d ' ')
    [ "$band" = "2.4G" ] && BAND="2.4G"
fi

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

/opt/shell/fcu760k_init.sh
i=0
while ! ifconfig -a 2>/dev/null | grep -q wlan0 && [ $i -lt 12 ]; do i=$((i + 1)); sleep 1; done
/opt/shell/fcu760k_ap.sh "$wifiName" "$password" "$BAND"

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

i=0
while ! ip link show br0 >/dev/null 2>&1 && [ $i -lt 10 ]; do i=$((i + 1)); sleep 1; done
ip link set br0 up 2>/dev/null
brctl addif br0 wlan0 2>/dev/null || true

br0IP=$(ip addr show br0 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -n 1)
[ -n "$br0IP" ] || br0IP=$(sed -n '1p' /opt/runtime/ip 2>/dev/null | tr -d '\r' | tr -d ' ')
[ -n "$br0IP" ] || exit 0
seg=$(echo "$br0IP" | cut -d. -f1-3)
cat > /tmp/udhcp_br0.conf << EOF
start           $seg.200
end             $seg.220
interface       br0
opt     router  $br0IP
option  subnet  255.255.255.0
option  lease   600
EOF
taskset -c 0-1 udhcpd /tmp/udhcp_br0.conf
