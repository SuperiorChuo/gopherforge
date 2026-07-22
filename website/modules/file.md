# 文件服务

file 服务提供统一的上传下载与存储抽象，三种后端按环境变量切换。

## 存储后端

| `UPLOAD_STORAGE_TYPE` | 说明 |
|------|------|
| `local` | 本地磁盘（默认，零依赖） |
| `minio` | 自建 MinIO（compose 已内置可选容器） |
| `s3` | **任意 S3 兼容云**：AWS S3、阿里云 OSS、腾讯云 COS、七牛等 |

S3 兼容云配置要点（`.env`）：

```bash
UPLOAD_STORAGE_TYPE=s3
UPLOAD_S3_ENDPOINT=https://oss-cn-hangzhou.aliyuncs.com   # 云商 S3 兼容端点
UPLOAD_S3_BUCKET=your-bucket
UPLOAD_S3_REGION=cn-hangzhou
UPLOAD_S3_ACCESS_KEY=...
UPLOAD_S3_SECRET_KEY=...
UPLOAD_S3_BUCKET_LOOKUP=dns    # 阿里/腾讯官方端点用 dns；MinIO/IP 端点用 path；缺省 auto
UPLOAD_PUBLIC_BASE_URL=https://cdn.example.com   # 公网访问域（CDN/自定义域）
```

`BUCKET_LOOKUP` 处理的是 S3 兼容云之间唯一的真实差异——bucket 寻址风格（virtual-host vs path-style）。

## 上传能力

大小与类型白名单限制、图片尺寸约束与缩略图、对象 key 规范化防目录穿越；上传记录入库可查询管理。
