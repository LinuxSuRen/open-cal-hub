---
name: 功能建议
about: 提出新功能或改进建议
title: 'Proposal: '
labels: [enhancement]
---
body:
  - type: markdown
    attributes:
      value: |
        请先检索 [Issues](https://github.com/linuxsuren/open-cal-hub/issues) 与 [Discussions](https://github.com/linuxsuren/open-cal-hub/discussions)。
        变更涉及 API 的建议请同时说明与 [API 契约](https://github.com/linuxsuren/open-cal-hub/blob/main/docs/api-contract.md)的关系。
  - type: textarea
    id: problem
    attributes:
      label: 要解决的问题
      description: 描述遇到的场景与痛点（而不是解决方案）。
    validations:
      required: true
  - type: textarea
    id: solution
    attributes:
      label: 期望的方案
      description: 你希望产品如何表现？涉及界面的建议附示意或截图。
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: 已考虑过的替代方案
      description: 现有功能或其他产品的做法，以及为何不满足。
  - type: textarea
    id: context
    attributes:
      label: 其他上下文
