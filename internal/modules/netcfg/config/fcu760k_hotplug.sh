#!/bin/sh
# USB WiFi 热插拔后恢复 AP
# 由 udev 在 wlan0 add/remove 时触发
# 恢复直接复用 /opt/setWifi.sh，与开机自启路径一致（wlan0 桥接进 br0）

LOCK=/var/run/fcu760k_hotplug.lock
IFINDEX_FILE=/var/run/fcu760k_wlan.ifindex
LOG=/tmp/fcu760k_hotplug.log

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >> "$LOG"
}

# udev 环境下异步执行，避免阻塞事件队列
# 整个恢复链钉在 CPU1：避开 CPU0（IRQ 密集）和 CPU2/3（webarm/CoVision）
if [ "$1" != "run" ]; then
    action="$1"
    [ -z "$action" ] && action="add"
    taskset -c 1 /opt/shell/fcu760k_hotplug.sh run "$action" &
    exit 0
fi

action="$2"
[ -z "$action" ] && action="add"

# wlan0 拔出：立刻杀掉旧 hostapd，避免插回后误判“已在运行”
# 不能杀 udhcpd：桥接模式下它服务 br0 有线网段，拔 WiFi 不应中断 LAN 的 DHCP
if [ "$action" = "remove" ]; then
    log "wlan0 remove, stop AP"
    killall hostapd wpa_supplicant udhcpc 2>/dev/null
    rm -f "$IFINDEX_FILE"
    exit 0
fi

# 串行化互斥：上一个恢复还在跑时等它结束（最多60s），而不是丢弃事件
# 快速连插拔时若直接跳过，上一次恢复可能已被 remove 打断，AP 就再也起不来
i=0
while [ $i -lt 60 ]; do
    if [ -f "$LOCK" ]; then
        oldpid=$(cat "$LOCK" 2>/dev/null)
        if [ -n "$oldpid" ] && kill -0 "$oldpid" 2>/dev/null; then
            i=$((i + 1))
            sleep 1
            continue
        fi
    fi
    break
done
if [ $i -ge 60 ]; then
    log "wait lock timeout, skip"
    exit 0
fi
echo $$ > "$LOCK"
trap 'rm -f "$LOCK"' EXIT

log "wlan hotplug add, recover AP"

# 等接口稳定
sleep 2

wait_wlan() {
    i=0
    while [ $i -lt 30 ]; do
        if [ -d /sys/class/net/wlan0 ]; then
            return 0
        fi
        i=$((i + 1))
        sleep 1
    done
    return 1
}

get_ifindex() {
    cat /sys/class/net/wlan0/ifindex 2>/dev/null
}

if ! wait_wlan; then
    log "wlan0 not present, try init"
    /opt/shell/fcu760k_init.sh >> "$LOG" 2>&1
    wait_wlan || {
        log "init failed, wlan0 still missing"
        exit 1
    }
fi

cur_ifindex=$(get_ifindex)
old_ifindex=$(cat "$IFINDEX_FILE" 2>/dev/null)

# 仅当 ifindex 未变且 hostapd 仍在时跳过（同一块网卡的重复 udev 事件）
# 热插拔后 wlan0 名字不变，但 ifindex 会变；旧 hostapd 已失效，必须重建
if [ -n "$cur_ifindex" ] && [ -n "$old_ifindex" ] && [ "$cur_ifindex" = "$old_ifindex" ] \
    && pidof hostapd >/dev/null 2>&1; then
    log "same ifindex=$cur_ifindex and hostapd alive, skip"
    exit 0
fi

# 开机阶段 autorun 的 setWifi.sh 正在启动 AP，交给它完成，避免并发重建
if ps 2>/dev/null | grep -v grep | grep -q "setWifi.sh"; then
    log "setWifi.sh running (boot path), skip"
    exit 0
fi

# 开机路径已完成（hostapd 在跑且 wlan0 已入 br0），补记 ifindex 后跳过
if pidof hostapd >/dev/null 2>&1 && brctl show br0 2>/dev/null | grep -q wlan0; then
    [ -n "$cur_ifindex" ] && echo "$cur_ifindex" > "$IFINDEX_FILE"
    log "AP already up on br0 ifindex=$cur_ifindex, skip"
    exit 0
fi

log "restart AP (old_ifindex=$old_ifindex new_ifindex=$cur_ifindex)"
killall hostapd wpa_supplicant udhcpc 2>/dev/null
sleep 1

# 复用开机 WiFi 配置脚本：读 wifiAp（SSID/密码/信道/频段）、起 AP、
# wlan0 加入 br0、br0 上起 DHCP。setWifi.sh 内以相对路径读 wifiAp，必须先 cd /opt
if [ ! -f /opt/setWifi.sh ]; then
    log "error: /opt/setWifi.sh not found"
    exit 1
fi
cd /opt
./setWifi.sh >> "$LOG" 2>&1
wifi_ret=$?

if [ $wifi_ret -eq 0 ] && pidof hostapd >/dev/null 2>&1; then
    cur_ifindex=$(get_ifindex)
    [ -n "$cur_ifindex" ] && echo "$cur_ifindex" > "$IFINDEX_FILE"
    log "hotplug recover done ifindex=$cur_ifindex"
else
    rm -f "$IFINDEX_FILE"
    log "hotplug recover failed ret=$wifi_ret"
    exit 1
fi

exit 0
