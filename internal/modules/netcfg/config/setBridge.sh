#!/bin/sh

f=/opt/runtime/ip
fmask=/opt/runtime/mask
fgw=/opt/runtime/gateway
LOG=/opt/setBridge.log
LOG_MAX=200
DEF_IP=192.168.1.136
DEF_MASK=255.255.255.0

LAN3_IP=10.113.0.136
LAN4_IP=10.113.1.136
LAN34_MASK=255.255.255.0

log() {
    msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo "$msg"
    echo "$msg" >> "$LOG"
    if [ "$(wc -l < "$LOG" 2>/dev/null || echo 0)" -gt "$LOG_MAX" ]; then
        tail -n "$LOG_MAX" "$LOG" > "$LOG.tmp" 2>/dev/null && mv "$LOG.tmp" "$LOG"
    fi
}

chk() {
    echo "$1" | grep -Eq '^([0-9]{1,3}\.){3}[0-9]{1,3}$' || return 1
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    for n in "$1" "$2" "$3" "$4"; do
        [ "$n" -le 255 ] 2>/dev/null || return 1
    done
}

# 连续 1 后接连续 0，禁止 0.0.0.0
chk_mask() {
    chk "$1" || return 1
    [ "$1" = "0.0.0.0" ] && return 1
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    gap=0
    for n in "$1" "$2" "$3" "$4"; do
        if [ "$gap" -eq 1 ]; then
            [ "$n" -eq 0 ] || return 1
            continue
        fi
        case "$n" in
            255) ;;
            254|252|248|240|224|192|128|0) gap=1 ;;
            *) return 1 ;;
        esac
    done
}

# 结果放 N 返回，调用方不用 $() 起子 shell
i2n() {
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    N=$(( ($1 << 24) + ($2 << 16) + ($3 << 8) + $4 ))
}

# 不进桥；失败只记日志，不中断
set_fixed() {
    ifc=$1
    addr=$2
    brctl delif br0 "$ifc" 2>/dev/null
    ip addr flush dev "$ifc" 2>/dev/null
    if ifconfig "$ifc" "$addr" netmask "$LAN34_MASK" up; then
        log "【日志】$ifc 配置完成 IP:$addr MASK:$LAN34_MASK"
    else
        log "【异常】$ifc IP配置失败"
    fi
}

# 与 br0 无关，必须在任何 exit 之前做完
set_fixed lan3 "$LAN3_IP"
set_fixed lan4 "$LAN4_IP"

# 掩码、网关各有自己的文件，和 ip 同目录；文件不存在时创建——
# 掩码按默认值，网关默认为空（无默认路由）
[ -f "$fmask" ] || { printf '%s' "$DEF_MASK" > "$fmask"; log "【日志】$fmask 不存在，已按默认 $DEF_MASK 创建"; }
[ -f "$fgw" ]   || { : > "$fgw";                           log "【日志】$fgw 不存在，已创建空文件"; }

IP=$(sed -n '1p' "$f" 2>/dev/null | tr -d '\r' | xargs)
MASK=$(sed -n '1p' "$fmask" 2>/dev/null | tr -d '\r' | xargs)
GW=$(sed -n '1p' "$fgw" 2>/dev/null | tr -d '\r' | xargs)

chk "$IP" || {
    log "【异常】IP非法，用默认 $DEF_IP"
    IP=$DEF_IP
    # ip 文件只保留一行 IP，多余行随回写一起清掉
    printf '%s' "$IP" > "$f.tmp" && mv "$f.tmp" "$f"
    log "【日志】已回写默认 IP 到 $f"
}
# 掩码非法只在本次启动用默认值，不回写文件
chk_mask "$MASK" || { log "【异常】掩码非法，用默认 $DEF_MASK"; MASK=$DEF_MASK; }

echo 0 > /proc/sys/net/ipv4/ip_forward 2>/dev/null

[ -d /sys/class/net/lan1 ] || { log "【异常】网口 lan1 不存在"; exit 1; }
[ -d /sys/class/net/br0 ] || brctl addbr br0 || { log "【异常】创建 br0 失败"; exit 1; }
ip addr flush dev lan1 2>/dev/null
# 重复执行时 lan1 已在 br0 中，addif 会报错；以成员身份为准
brctl addif br0 lan1 2>/dev/null
[ -d /sys/class/net/br0/brif/lan1 ] || { log "【异常】lan1 加入 br0 失败"; exit 1; }
ip link set lan1 up; ip link set br0 up
ip addr flush dev br0 2>/dev/null
ifconfig br0 "$IP" netmask "$MASK" up || { log "【异常】br0 IP配置失败"; exit 1; }

gw_log="无默认网关"
gw_ok=0
if chk "$GW" && [ "$GW" != "$IP" ]; then
    i2n "$IP"; ip_n=$N
    i2n "$MASK"; mask_n=$N
    i2n "$GW"; gw_n=$N
    [ $(( ip_n & mask_n )) -eq $(( gw_n & mask_n )) ] && gw_ok=1
fi
if [ "$gw_ok" -eq 1 ]; then
    if ip route replace default via "$GW" dev br0; then
        gw_log="GW:$GW"
    else
        log "【异常】默认网关配置失败 GW:$GW"
        ip route del default 2>/dev/null
    fi
else
    log "【异常】网关[${GW:-空}]不可用，不配默认路由"
    ip route del default 2>/dev/null
fi
log "【日志】br0 配置完成 IP:$IP MASK:$MASK $gw_log (绑定 lan1)"

exit 0
