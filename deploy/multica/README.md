# Multica 84 部署配置

## 目录结构

```
/opt/multica/
├── docker-compose.selfhost.yml  # 由 GitHub Actions 同步
├── .env                         # 环境变量（首次部署需要手动创建）
└── data/
    ├── postgres/                 # PostgreSQL 数据卷
    └── uploads/                  # 文件上传目录
```

## 首次部署

GitHub Actions 会自动：
1. 构建并推送 backend + web 镜像到阿里云镜像仓库
2. 通过 SSH 创建 `/opt/multica/` 目录
3. 同步 `docker-compose.selfhost.yml` 到服务器
4. 拉取新镜像并启动容器

首次部署前需确保服务器 `/opt/multica/.env` 已存在（否则部署会报错）。

## 服务端口

| 服务 | 内部端口 | 外部映射 |
|------|---------|---------|
| PostgreSQL | 5432 | 仅容器内 |
| Backend | 8080 | 127.0.0.1:8080 |
| Frontend | 3000 | 127.0.0.1:3033 |

## Nginx 反向代理参考

```nginx
server {
    listen 443 ssl;
    server_name multica.quizck.cn;

    location / {
        proxy_pass http://127.0.0.1:3033;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /ws/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```
