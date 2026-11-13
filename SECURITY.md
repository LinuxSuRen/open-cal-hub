# 安全策略

## 报告漏洞

**请不要通过公开 issue、讨论区或 PR 报告安全漏洞。**

请使用 GitHub 的私有漏洞报告通道：

1. 打开 [github.com/linuxsuren/open-cal-hub/security/advisories](https://github.com/linuxsuren/open-cal-hub/security/advisories)；
2. 点击 **Report a vulnerability**，按表单描述问题。

也可以在报告中附上复现步骤、影响范围与修复建议。收到报告后我们会：

- 24 小时内确认收到；
- 评估影响并按严重程度排期修复（严重问题优先）；
- 修复发布后在 GitHub Security Advisories 公开致谢（除非你希望匿名）。

在修复版本发布前，请勿公开披露漏洞细节。

## 支持版本

| 版本 | 状态 | 支持策略 |
|---|---|---|
| 1.0.x | ✅ 支持 | 安全修复与关键缺陷修复 |

我们只为最新次版本线提供安全修复，请保持使用最新发布版本。

## 报告范围

以下情况请先排查再报告（通常不属于产品漏洞）：

- 部署配置不当：如生产环境未设置 `auth.jwt_secret`、`server.base_url` 指向错误域名、数据目录权限过宽；
- 反向代理或 TLS 配置问题；
- 暴露在公网却未启用 HTTPS 导致的凭据嗅探；
- 已经停用版本的问题；
- DoS 压测类问题（请附可复现的PoC并先与我们沟通）。

## 部署加固建议

- 生产环境通过 HTTPS（反向代理 TLS）暴露服务；
- 显式配置 `auth.jwt_secret`（或妥善备份自动生成的 `data/secret.key`）；
- 容器以非 root 用户运行（官方镜像已内置）；
- 定期升级到最新发布版本；
- 邮件凭据使用专用账号，最小化 SMTP 授权范围。
