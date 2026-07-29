# 微服务前端：React + Ant Design（Vite 构建）
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时直接取本机 npm run build 的产物
# .bin/html/（rsync 排除了 dist 这个名字，故产物同步用 .bin/html），跳过容器内 npm 构建。
ARG MAIN_SRC=builder
FROM node:24-alpine AS builder

WORKDIR /app

COPY package*.json ./

# 内网服务器构建用国内 npm 镜像；npm 缓存挂 BuildKit cache mount，lockfile 变更时不必全量重下
RUN --mount=type=cache,target=/root/.npm \
    npm config set registry https://registry.npmmirror.com \
    && if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY . .

RUN npm run build

FROM alpine:3.20 AS prebuilt
COPY .bin/html /app/dist

FROM ${MAIN_SRC} AS assets

FROM nginx:alpine

COPY --from=assets /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
