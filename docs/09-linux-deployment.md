# 旧版 Nginx + systemd 文档已停用

Linux 的标准部署方式已统一为 root + PostgreSQL + Caddy + PM2。

请只使用：

[`docs/10-linux-caddy-pm2-deployment.md`](10-linux-caddy-pm2-deployment.md)

不要再按旧版 Nginx + systemd 文档安装，也不要将两套部署命令混用。旧流程中的系统
升级、重启、PostgreSQL 手工监听配置和服务器端测试步骤不再属于标准安装流程。
