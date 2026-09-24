# dida-cli

面向 agent 的滴答清单命令行工具. 数据通道是滴答官方 MCP 端点, 认证只需要一个 `dp_` 开头的 API 口令.

单 Go 二进制, 零运行时依赖, stdout 恒为稳定的 JSON 信封.

## 为什么是 CLI 而不是 MCP

MCP 的调用权掌握在宿主手里, 而命令行可以把能力直接交给 agent 与脚本. 这个项目在官方 MCP 之上补的是 agent 真正需要的那几件事:

- **输出契约稳定**: 成功与失败同一形状, 错误带分类与可直接照做的 `hint`, 退出码细分到 7 种.
- **可自省**: `dida schema` 给出 72 条命令契约, `dida tool` 直通官方 55 个原生工具, agent 不必凭记忆猜参数.
- **可预览**: 所有写命令都支持 `--dry-run`, 先看清将要发出的请求体再决定是否执行.
- **可约束**: `read_only` 与 `non_destructive` 两道闸门写在配置里, 命令行无法覆盖, 让 agent 在需要时被真正绑住手.

## 安装

```shell
go install github.com/azazo1/dida-cli/cmd/dida@latest
```

从源码构建:

```shell
just build
```

## 快速开始

在滴答清单网页版里进入头像 > 设置 > 账户与安全, 拿到 API 口令, 然后:

```shell
printf 'dp_你的口令' | dida auth login
dida doctor
dida view today --compact
```

口令落在本地配置目录并收紧到 `0600`, 全流程不经过命令行参数, 不会留在 shell 历史里.

## 命令树

```
auth      login | logout | status
config    path | list | get | set
doctor
skill     [--compact]
schema    list | show <command>
project   list | get | data | members | create | update
task      list | get | create | update | complete | delete | move | search
          checklist-done | assign | unassign | batch-add | batch-update | complete-project
habit     list | sections | get | create | update | checkin | checkins
focus     list | get | create | update | delete
tag       list | create | update | rename | delete
group     list | create | update | delete
column    list | create | update
comment   list | add | delete
countdown list
view      today | tomorrow | next | upcoming | last7day | overdue | inbox | completed | query
tool      list | show <tool-name> | call <tool-name>
version
```

给 agent 的完整用法说明用 `dida skill` 获取, 它随二进制一起分发, 相当于这个工具的 man.

## 退出码

| 退出码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 未归类的错误 |
| 2 | 参数用法错误 |
| 3 | 口令缺失或无效 |
| 4 | 资源不存在 |
| 5 | 官方接口返回错误 |
| 6 | 破坏性操作缺少 `--yes` |
| 7 | 被本地闸门拦下 |

## 两道闸门

| 配置项 | 作用 | 拦截范围 |
| --- | --- | --- |
| `read_only` | 禁止一切对账户的写入 | 30 个写工具 |
| `non_destructive` | 禁止删除类操作, 允许创建与更新 | 5 个破坏性工具 |

三个来源任一为真即生效, 且只收紧不放松:

```shell
dida config set read_only true      # 长期约束, 命令行无法关闭
DIDA_READ_ONLY=1 dida task list     # 单次环境约束
dida --read-only task list          # 单次参数约束
```

判定依据是官方工具的 `readOnlyHint` 与 `destructiveHint` 注解, 不是本地维护的静态名单. `dida tool list` 可以看到每个工具的属性.

## 参数与时间

每个命令有具名参数与完整参数两种传参模式, `--dry-run` 下都能看到最终请求体:

```shell
dida task create --title 买牛奶 --due tomorrow --priority 3
dida task create --body-json '{"task":{"title":"买牛奶","projectId":"inbox"}}'
```

时间字段接受 `2026-09-25`, `2026-09-25 09:00`, `2026-09-25T09:00:00+08:00` 以及 `today` / `tomorrow` / `+3d` / `-1w` 这类相对写法, 统一按配置时区归一化成服务端要求的线格式.

## 目录结构

```
cmd/dida            程序入口
internal/apperr     错误分类与退出码
internal/cli        命令实现与契约驱动的构建器
internal/config     配置目录, 配置迁移, 口令存储
internal/guard      只读与非破坏性两道闸门
internal/logging    只写 stderr 的结构化日志
internal/official   官方 MCP 客户端
internal/output     JSON 信封与表格渲染
internal/schema     命令契约表
internal/skill      内嵌的 agent 使用指南
internal/timeparse  时间解析与线格式归一化
```

## 已知边界

官方 MCP 通道没有附件, 回收站, 统计报表, 模板与共享协作这些能力, 它们在网页私有接口上, 本项目无法替代.

另外几条实测得到的限制: 官方通道没有删除习惯的工具, 因此 `habit create` 不可撤销; `focus list` 的时间范围不得超过一个月; `task complete-project` 单次最多 20 个任务; `tag update` 不能改名, 改名要用 `tag rename`.

## 开发

常用动作都收在 justfile 里:

```shell
just build        # 构建到 bin/dida, 版本号显示 dev-build
just test         # 运行测试
just vet          # go vet
just fmt-check    # 检查格式, 只报告不修改
just run view today --compact
just guide        # 输出 agent 使用指南
just dist         # 生成当前平台的发布产物
just version      # 当前构建应当显示的版本号
```

版本号规则: 恰好停在版本 tag 上时显示该 tag, 否则在最近一个 tag 后追加 7 位短 hash, 工作区有未提交改动时改用 `^` 分隔, 日常开发构建显示 `dev-build`.

## 许可证

[MIT](LICENSE)
