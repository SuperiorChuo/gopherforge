# FreeSWITCH 运行镜像 + 本项目拨号/分机覆盖配置
# 基础镜像可按需替换为组织私有构建
FROM safarov/freeswitch:latest

# 覆盖/追加本项目配置（与镜像内 /etc/freeswitch 合并目录）
COPY conf/directory/default/ /etc/freeswitch/directory/default/
COPY conf/dialplan/default/ /etc/freeswitch/dialplan/default/
COPY conf/autoload_configs/event_socket.conf.xml /etc/freeswitch/autoload_configs/event_socket.conf.xml

# 录音目录
RUN mkdir -p /var/lib/freeswitch/recordings \
    && chown -R freeswitch:freeswitch /var/lib/freeswitch/recordings || true

EXPOSE 5060/tcp 5060/udp 8021/tcp 16384-16484/udp
