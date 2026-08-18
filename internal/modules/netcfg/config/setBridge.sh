#!/bin/sh

f=/opt/runtime/ip
LOG=/opt/setBridge.log
LOG_MAX=200
DEF_IP=192.168.1.136
DEF_MASK=255.255.255.0
DEF_GW=192.168.1.1

LAN3_IP=10.113.0.136
LAN4_IP=10.113.1.136
LAN34_MASK=255.255.255.0

log() {
    msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo "$msg"
    echo "$msg" >> "$LOG"
    # 超出上限则丢弃最旧的记录
    if [ "$(wc -l < "$LOG" 2>/dev/null || echo 0)" -gt "$LOG_MAX" ]; then
        tail -n "$LOG_MAX" "$LOG" > "$LOG.tmp" 2>/dev/null && mv "$LOG.tmp" "$LOG"
    fi
}

# IPv4校验
chk() {
    echo "$1" | grep -Eq '^([0-9]{1,3}\.){3}[0-9]{1,3}$' || return 1
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    for n in "$1" "$2" "$3" "$4"; do
        [ "$n" -le 255 ] 2>/dev/null || return 1
    done
}

# 子网掩码校验：连续1后接连续0；禁止0.0.0.0
# 合法段：255/254/252/248/240/224/192/128/0；出现非255后后续必须为0
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

# IP→整数
i2n() {
    oIFS=$IFS; IFS=.; set -- $1; IFS=$oIFS
    echo $(( ($1 << 24) + ($2 << 16) + ($3 << 8) + $4 ))
}

IP=$(sed -n '1p' "$f" 2>/dev/null | tr -d '\r' | xargs)
MASK=$(sed -n '2p' "$f" 2>/dev/null | tr -d '\r' | xargs)
GW=$(sed -n '3p' "$f" 2>/dev/null | tr -d '\r' | xargs)

fix=0
chk "$IP"        || { log "【异常】IP非法，用默认 $DEF_IP"; IP=$DEF_IP; fix=1; }
chk_mask "$MASK" || { log "【异常】掩码非法，用默认 $DEF_MASK"; MASK=$DEF_MASK; fix=1; }

# 网关必须合法、同网段且不等于本机，否则不配默认路由
gw_ok=0
if chk "$GW" && [ "$GW" != "$IP" ] \
    && [ $(( $(i2n "$IP") & $(i2n "$MASK") )) -eq $(( $(i2n "$GW") & $(i2n "$MASK") )) ]; then
    gw_ok=1
else
    log "【异常】网关[${GW:-空}]不可用，不配默认路由"
fi

# 有回退默认值时，把生效配置回写配置文件
[ "$fix" -eq 1 ] && { printf '%s\n%s\n%s\n' "$IP" "$MASK" "$GW" > "$f"; log "【日志】已回写默认配置到 $f"; }

echo 0 > /proc/sys/net/ipv4/ip_forward 2>/dev/null

ip link show lan1 >/dev/null 2>&1 || { log "【异常】网口 lan1 不存在"; exit 1; }

# br0 只绑定 lan1；lan3/lan4 不进桥
brctl show 2>/dev/null | grep -q '^br0' || brctl addbr br0 || { log "【异常】创建 br0 失败"; exit 1; }
brctl delif br0 lan3 2>/dev/null; brctl delif br0 lan4 2>/dev/null
ip addr flush dev lan1 2>/dev/null
brctl addif br0 lan1 2>/dev/null || { log "【异常】lan1 加入 br0 失败"; exit 1; }
ip link set lan1 up; ip link set br0 up
ip addr flush dev br0 2>/dev/null
ifconfig br0 "$IP" netmask "$MASK" up || { log "【异常】br0 IP配置失败"; exit 1; }
if [ "$gw_ok" -eq 1 ]; then
    ip route replace default via "$GW" dev br0 || { log "【异常】默认网关配置失败 GW:$GW"; exit 1; }
    log "【日志】br0 配置完成 IP:$IP MASK:$MASK GW:$GW (绑定 lan1)"
else
    ip route del default 2>/dev/null
    log "【日志】br0 配置完成 IP:$IP MASK:$MASK (绑定 lan1，无默认网关)"
fi

ip addr flush dev lan3 2>/dev/null
ifconfig lan3 "$LAN3_IP" netmask "$LAN34_MASK" up || { log "【异常】lan3 IP配置失败"; exit 1; }
log "【日志】lan3 配置完成 IP:$LAN3_IP MASK:$LAN34_MASK"
ip addr flush dev lan4 2>/dev/null
ifconfig lan4 "$LAN4_IP" netmask "$LAN34_MASK" up || { log "【异常】lan4 IP配置失败"; exit 1; }
log "【日志】lan4 配置完成 IP:$LAN4_IP MASK:$LAN34_MASK"

exit 0
