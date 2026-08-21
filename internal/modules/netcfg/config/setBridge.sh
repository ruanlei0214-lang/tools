#!/bin/sh

f=/opt/runtime/ip
fmask=/opt/runtime/mask
fgw=/opt/runtime/gateway
DEF_IP=192.168.1.136
DEF_MASK=255.255.255.0

LAN3_IP=10.113.0.136
LAN4_IP=10.113.1.136
LAN34_MASK=255.255.255.0

# 并发保护：mkdir 原子锁；持锁进程异常死亡（kill -9）时清残留
lockd=/tmp/setBridge.lock.d
while ! mkdir "$lockd" 2>/dev/null; do
    pid=$(cat "$lockd/pid" 2>/dev/null)
    if [ -n "$pid" ] && [ ! -d "/proc/$pid" ]; then
        rm -rf "$lockd"
        continue
    fi
    echo "setBridge 已在运行，退出"
    exit 0
done
echo $$ > "$lockd/pid"
trap 'rm -rf "$lockd"' EXIT

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

chk() {
    # 拒绝前导零（如 001），防止 i2n 算术按八进制解析崩溃
    echo "$1" | grep -Eq '^((0|[1-9][0-9]{0,2})\.){3}(0|[1-9][0-9]{0,2})$' || return 1
    # 拒绝全 0 / 全 1
    [ "$1" = "0.0.0.0" ] && return 1
    [ "$1" = "255.255.255.255" ] && return 1
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    for n in "$1" "$2" "$3" "$4"; do
        [ "$n" -le 255 ] 2>/dev/null || return 1
    done
}

# 连续 1 后接连续 0（0.0.0.0 已被 chk 拒绝）
chk_mask() {
    chk "$1" || return 1
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
    if printf '%s' "$IP" > "$f.tmp" && mv "$f.tmp" "$f"; then
        log "【日志】已回写默认 IP 到 $f"
    else
        log "【异常】回写默认 IP 到 $f 失败"
    fi
}
# 掩码非法只在本次启动用默认值，不回写文件
chk_mask "$MASK" || { log "【异常】掩码非法，用默认 $DEF_MASK"; MASK=$DEF_MASK; }

# 禁止三层转发，隔离各网段（网桥为二层转发，不依赖此项）
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

# 兼容老版本：清掉可能残留在 lan1 上的默认路由（没有则立即退出循环，无副作用）
while ip route del default dev lan1 2>/dev/null; do :; done

gw_log="无默认网关"
gw_ok=0
if chk "$GW" && [ "$GW" != "$IP" ]; then
    i2n "$IP"; ip_n=$N
    i2n "$MASK"; mask_n=$N
    i2n "$GW"; gw_n=$N
    net_n=$(( ip_n & mask_n ))
    bcast_n=$(( (net_n | ~mask_n) & 0xFFFFFFFF ))
    # 同网段，且不是网络地址/广播地址
    [ $(( gw_n & mask_n )) -eq "$net_n" ] && [ "$gw_n" -ne "$net_n" ] && [ "$gw_n" -ne "$bcast_n" ] && gw_ok=1
fi
if [ "$gw_ok" -eq 1 ]; then
    if ip route replace default via "$GW" dev br0; then
        gw_log="GW:$GW"
    else
        log "【异常】默认网关配置失败 GW:$GW"
        ip route del default dev br0 2>/dev/null
    fi
else
    log "【异常】网关[${GW:-空}]不可用，不配默认路由"
    ip route del default dev br0 2>/dev/null
fi
log "【日志】br0 配置完成 IP:$IP MASK:$MASK $gw_log (绑定 lan1)"

exit 0
