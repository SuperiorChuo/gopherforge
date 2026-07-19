# 微服务前端：React + Ant Design（Vite 构建）
FROM node:24-alpine AS builder

WORKDIR /app

COPY package*.json ./

# 内网服务器构建用国内 npm 镜像
RUN npm config set registry https://registry.npmmirror.com \
    && if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY . .

RUN npm run build

FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
