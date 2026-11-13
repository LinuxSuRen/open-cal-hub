---
name: Bug 报告
about: 报告可复现的问题
title: 'Bug: '
labels: [bug]
---
body:
  - type: markdown
    attributes:
      value: |
        感谢反馈！请先检索 [Issues](https://github.com/linuxsuren/open-cal-hub/issues) 确认没有重复。
        与 UI 相关的问题请附截图；错误信息请粘贴**关键文字**（截图不能替代文字）。
  - type: textarea
    id: description
    attributes:
      label: 问题描述
      description: 一句话说明哪里不符合预期。
    validations:
      required: true
  - type: textarea
    id: steps
    attributes:
      label: 复现步骤
      description: |
        例如：
        1. 打开 /app/calendar
        2. 创建每周重复事件
        3. 删除其中一周的实例
      placeholder: |
        1. …
        2. …
    validations:
      required: true
  - type: textarea
    id: expected
    attributes:
      label: 预期行为
    validations:
      required: true
  - type: textarea
    id: actual
    attributes:
      label: 实际行为
      description: 请附错误码 / 响应体 / 日志关键行（注意隐去敏感信息）。
    validations:
      required: true
  - type: input
    id: version
    attributes:
      label: 服务版本
      description: 页面页脚、/api/v1/meta 或二进制启动日志中的版本号。
      placeholder: 1.0.0
    validations:
      required: true
  - type: dropdown
    id: deploy
    attributes:
      label: 部署方式
      options:
        - 二进制（Releases 下载）
        - Docker / docker compose
        - systemd
        - 源码构建
    validations:
      required: true
  - type: input
    id: client
    attributes:
      label: 客户端
      description: 浏览器及版本 / CalDAV 客户端（如 macOS 日历、Thunderbird）及版本。
      placeholder: Chrome 141 / Thunderbird 128
  - type: textarea
    id: context
    attributes:
      label: 其他上下文
      description: 配置摘要（隐去密钥）、反代拓扑、出现频率等。
