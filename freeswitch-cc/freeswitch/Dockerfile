# FreeSWITCH 1.11.1 升级镜像（Debian 13 + SignalWire 官方 apt 源）
# 与现有 Dockerfile（safarov 1.10.12，基础镜像已停更）并行，独立构建验证，
# 验证通过后切换生产 compose 引用本镜像。
#
# 构建（PAT 经 BuildKit secret 注入，不落镜像层）：
#   docker build --secret id=fs_apt_auth,src=/etc/go-admin-kit/fs-apt-auth.conf \
#     -t go-freeswitch-cc-freeswitch-111 -f Dockerfile.fs111 .
# secret 文件内容为一行 apt auth.conf：machine freeswitch.signalwire.com login signalwire password pat_xxx
FROM debian:13-slim

# 阻止 apt postinst 自动启动服务（容器内无 systemd——freeswitch 包 postinst
# 会启动 FS 前台进程卡死 dpkg 收尾，08-10 构建实证）
RUN echo '#!/bin/sh\nexit 101' > /usr/sbin/policy-rc.d && chmod +x /usr/sbin/policy-rc.d

# ---- 1. apt 源（SignalWire 官方，PAT 认证）----
RUN --mount=type=secret,id=fs_apt_auth \
    apt-get update \
    && apt-get install -y --no-install-recommends gnupg2 wget ca-certificates lsb-release \
    && cp /run/secrets/fs_apt_auth /etc/apt/auth.conf \
    && chmod 600 /etc/apt/auth.conf \
    && wget -q --http-user=signalwire --http-password="$(sed -n 's/.*password //p' /etc/apt/auth.conf)" \
         -O /usr/share/keyrings/signalwire-freeswitch-repo.gpg \
         https://freeswitch.signalwire.com/repo/deb/debian-release/signalwire-freeswitch-repo.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/signalwire-freeswitch-repo.gpg] https://freeswitch.signalwire.com/repo/deb/debian-release/ trixie main" \
         > /etc/apt/sources.list.d/freeswitch.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends freeswitch-meta-all \
    && rm -f /etc/apt/auth.conf \
    && rm -rf /var/lib/apt/lists/*

# ---- 2. 本项目配置覆盖（Debian 包配置在 /etc/freeswitch，直接 COPY）----
# Debian 包预装 vanilla 示例分机（1004-1019 等 20 个，无 dial-string），
# COPY 只添加不删——先清空再 COPY 本项目目录（1.11 适配点 3）
RUN rm -f /etc/freeswitch/directory/default/*.xml \
    && rm -f /etc/freeswitch/sip_profiles/internal.xml /etc/freeswitch/sip_profiles/external.xml
COPY conf/autoload_configs/ /etc/freeswitch/autoload_configs/
COPY conf/dialplan/ /etc/freeswitch/dialplan/
COPY conf/sip_profiles/ /etc/freeswitch/sip_profiles/
COPY conf/directory/ /etc/freeswitch/directory/
COPY prompts/ /etc/freeswitch/prompts/

# ---- 3. 模块清单：只启用本项目需要的（1.11 移除的边缘模块不在清单内）----
# 启用：callcenter（ACD 队列）/blacklist/json_cdr（话单 HTTP 投递）/shout（mp3 排队音乐）/
# xml_curl（动态目录）/cdr_csv（话单磁盘兜底）/voicemail/cidlookup（来显归属）/
# directory（411 按名拨号）/flite（英文 TTS 兜底）
# 不启用：verto/signalwire（本项目不用，且 verto 有 CVE-2026-49848 历史）
RUN sed -i \
    -e 's|<!-- *<load module="mod_callcenter"/> *-->|<load module="mod_callcenter"/>|' \
    -e 's|<!-- *<load module="mod_blacklist"/> *-->|<load module="mod_blacklist"/>|' \
    -e 's|<!-- *<load module="mod_json_cdr"/> *-->|<load module="mod_json_cdr"/>|' \
    -e 's|<!-- *<load module="mod_cdr_csv"/> *-->|<load module="mod_cdr_csv"/>|' \
    -e 's|<!-- *<load module="mod_shout"/> *-->|<load module="mod_shout"/>|' \
    -e 's|<!-- *<load module="mod_xml_curl"/> *-->|<load module="mod_xml_curl"/>|' \
    -e 's|<!-- *<load module="mod_voicemail"/> *-->|<load module="mod_voicemail"/>|' \
    -e 's|<!-- *<load module="mod_cidlookup"/> *-->|<load module="mod_cidlookup"/>|' \
    -e 's|<!-- *<load module="mod_directory"/> *-->|<load module="mod_directory"/>|' \
    -e 's|<!-- *<load module="mod_flite"/> *-->|<load module="mod_flite"/>|' \
    /etc/freeswitch/autoload_configs/modules.conf.xml \
    && for m in mod_callcenter mod_blacklist mod_json_cdr mod_shout mod_xml_curl \
                mod_cdr_csv mod_voicemail mod_cidlookup mod_db mod_hash \
                mod_directory mod_flite; do \
         grep -q "<load module=\"$m\"/>" /etc/freeswitch/autoload_configs/modules.conf.xml \
           || sed -i "s|</modules>|  <load module=\"$m\"/>\n  </modules>|" /etc/freeswitch/autoload_configs/modules.conf.xml; \
       done \
    && grep -q '<load module="mod_verto"/>' /etc/freeswitch/autoload_configs/modules.conf.xml \
       && sed -i '/<load module="mod_verto"\/>/d' /etc/freeswitch/autoload_configs/modules.conf.xml || true

# ---- 4. 音库：英文 + 中文（语音信箱/IVR 提示音，同现有逻辑）----
RUN wget -q https://files.freeswitch.org/releases/sounds/freeswitch-sounds-en-us-callie-8000-1.0.52.tar.gz -O /s.tar.gz \
    && tar xzf /s.tar.gz -C /usr/share/freeswitch/sounds/ \
    && rm /s.tar.gz \
    && wget -q https://files.freeswitch.org/releases/sounds/freeswitch-sounds-zh-cn-sinmei-16000-1.0.51.tar.gz -O /z.tar.gz \
    && tar xzf /z.tar.gz -C /usr/share/freeswitch/sounds/ \
    && rm /z.tar.gz

# ---- 5. 运行：前台启动（容器内无 systemd）----
# 录音目录
RUN mkdir -p /var/lib/freeswitch/recordings /var/lib/freeswitch/early-media /var/lib/freeswitch/json_cdr_err /var/lib/freeswitch/cdr-csv \
    && chown -R freeswitch:freeswitch /var/lib/freeswitch 2>/dev/null || true

EXPOSE 5060/tcp 5060/udp 5061/tcp 5066/tcp 7443/tcp 8021/tcp 16384-16484/udp

# -nf：不 fork 后台（容器内前台运行，否则容器立即退出——1.11 适配点 1）
CMD ["freeswitch", "-nonat", "-nf", "-nc"]
