# 微服务前端：React + Ant Design（Vite 构建）
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时直接取本机 npm run build 的产物
# .bin/html/（rsync 排除了 dist 这个名字，故产物同步用 .bin/html），跳过容器内 npm 构建。
ARG MAIN_SRC=builder
FROM node:24-alpine AS builder

ARG VITE_CC_SIP_WS_URL
ENV VITE_CC_SIP_WS_URL=${VITE_CC_SIP_WS_URL}

WORKDIR /app

COPY package*.json ./

# 内网服务器构建用国内 npm 镜像；npm 缓存挂 BuildKit cache mount，lockfile 变更时不必全量重下
RUN --mount=type=cache,target=/root/.npm \
    npm config set registry https://registry.npmmirror.com \
    && if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY . .

RUN npm run build

FROM alpine:3.24 AS prebuilt
COPY .bin/html /app/dist

FROM ${MAIN_SRC} AS assets

FROM nginx:alpine

COPY --from=assets /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY docker-entrypoint.d/40-runtime-config.sh /docker-entrypoint.d/40-runtime-config.sh
RUN chmod 755 /docker-entrypoint.d/40-runtime-config.sh

# nginx:alpine 默认 root；改为非 root（swarm 安全加固）。
# nginx 需要写 /tmp（pid 文件 + client_body_temp），/var/cache/nginx 已属 nginx。
# /var/run 是 tmpfs 不能在 build 时 chown，改 pid 路径到 /tmp/nginx.pid。
RUN sed -i 's|pid\s*/run/nginx.pid;|pid /tmp/nginx.pid;|' /etc/nginx/nginx.conf \
    && chown -R nginx:nginx /usr/share/nginx/html /var/cache/nginx /tmp /docker-entrypoint.d \
    && chown nginx:nginx /etc/nginx/conf.d/default.conf
USER nginx

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
