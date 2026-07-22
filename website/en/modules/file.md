# File Service

Unified upload/download with three storage backends selected by `UPLOAD_STORAGE_TYPE`:

| Value | Backend |
|------|------|
| `local` | Local disk (default) |
| `minio` | Self-hosted MinIO |
| `s3` | **Any S3-compatible cloud**: AWS S3, Aliyun OSS, Tencent COS, Qiniu… |

S3-compatible example:

```bash
UPLOAD_STORAGE_TYPE=s3
UPLOAD_S3_ENDPOINT=https://oss-cn-hangzhou.aliyuncs.com
UPLOAD_S3_BUCKET=your-bucket
UPLOAD_S3_REGION=cn-hangzhou
UPLOAD_S3_ACCESS_KEY=...
UPLOAD_S3_SECRET_KEY=...
UPLOAD_S3_BUCKET_LOOKUP=dns   # dns for Aliyun/Tencent endpoints; path for MinIO/IP; default auto
UPLOAD_PUBLIC_BASE_URL=https://cdn.example.com
```

`BUCKET_LOOKUP` covers the one real behavioural difference among S3-compatible providers — virtual-host vs path-style bucket addressing. Size/type allow-lists, image constraints with thumbnails, and object-key sanitisation are built in.
