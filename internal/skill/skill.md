# dida 使用指南

面向大模型与命令行的滴答清单客户端. 数据通道是官方 MCP 端点 `https://mcp.dida365.com`, 认证只需要一个 `dp_` 开头的 API 口令.

## 输出契约

stdout 永远是同一个形状的 JSON 信封, 成功与失败只差 `ok` 字段:

```json
{
  "ok": true,
  "command": "task create",
  "data": { "id2etag": { "6ab51c89e4b06220cd6f0525": "..." } },
  "meta": { "count": 1, "read_only": false, "duration_ms": 412 }
}
```

```json
{
  "ok": false,
  "command": "task delete",
  "error": {
    "type": "confirmation_required",
    "message": "dida task delete 属于破坏性操作, 需要显式确认",
    "hint": "确认目标无误后加上 --yes 重新执行, 例如: --yes"
  },
  "meta": { "read_only": false, "duration_ms": 3 }
}
```

日志全部走 stderr, stdout 不会被日志污染, 可以放心用管道接 `jq`.

## 退出码

| 退出码 | error.type | 含义 |
| --- | --- | --- |
| 0 | 无 | 成功 |
| 1 | `general` | 未归类的错误 |
| 2 | `usage` | 参数用法错误 |
| 3 | `auth` | 口令缺失或无效 |
| 4 | `not_found` | 资源不存在 |
| 5 | `upstream` | 官方接口返回错误 |
| 6 | `confirmation_required` | 破坏性操作缺少 `--yes` |
| 7 | `read_only` 或 `non_destructive` | 被本地闸门拦下 |

判定成功看退出码或 `ok`, 不要只看 HTTP 是否返回.

## 两道本地闸门

| 配置项 | 作用 | 拦截范围 |
| --- | --- | --- |
| `read_only` | 禁止一切对账户的写入 | 30 个写工具 |
| `non_destructive` | 禁止删除类操作, 允许创建与更新 | 5 个破坏性工具 |

三个来源任一为真即生效, 且只收紧不放松: 配置文件 `read_only = true`, 环境变量 `DIDA_READ_ONLY=1`, 命令行 `--read-only`. 闸门一旦被配置打开, 命令行无法关闭, 只能改配置文件.

`error.hint` 会写明闸门来自哪个来源以及需要改哪个文件.

## 立即上手

```shell
printf 'dp_你的口令' | dida auth login
dida doctor
dida view today --compact
```

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

## 参数模式

每个命令都有两种传参方式, 共用同一套内部映射.

具名参数模式用于日常调用:

```shell
dida task create --title 买牛奶 --due tomorrow --priority 3
```

完整参数模式用于参数很多或结构嵌套的场景, `--body-json` 接受 JSON 文本, `-` 表示从 stdin 读, `@文件` 表示从文件读:

```shell
dida task create --body-json '{"task":{"title":"买牛奶","projectId":"inbox"}}'
dida task batch-add --body-json @tasks.json
```

具名参数模式的时间字段会自动归一化成服务端要求的格式, 完整参数模式不做任何加工, 传什么发什么.

## 日期写法

写操作的时间字段支持四种写法, 统一按配置时区解释后转成服务端线格式:

| 写法 | 含义 |
| --- | --- |
| `2026-09-25` | 当天零点 |
| `2026-09-25 09:00` | 指定时刻 |
| `2026-09-25T09:00:00+08:00` | 带时区的标准写法 |
| `today` / `tomorrow` / `+3d` / `-1w` / `+2h` | 相对写法 |

相对写法后面可以接具体时刻, 例如 `"tomorrow 09:00"`.

`dida view` 系列的相对时间由官方服务端解析, 客户端只负责透传, 语义与账号时区一致.

## 典型工作流

看一眼今天该做什么:

```shell
dida view today --compact
```

只看到期未完成的, 按截止时间排序:

```shell
dida view overdue --compact
```

创建任务并确认请求体, 再真正执行:

```shell
dida task create --title 交报告 --due tomorrow --project inbox --dry-run
dida task create --title 交报告 --due tomorrow --project inbox
```

批量创建:

```shell
dida task batch-add --tasks-json '[{"title":"a","projectId":"inbox"},{"title":"b","projectId":"inbox"}]' --dry-run
```

找到任务后完成它:

```shell
dida task search --query 交报告 --compact
dida task complete 6ab51c89e4b06220cd6f0525
```

写入往返验证时记得清理:

```shell
dida task create --title agent-smoke-test --project inbox
dida task search --query agent-smoke-test --compact
dida task delete 6ab51c89e4b06220cd6f0525 --yes
```

## 参数自省

不要凭记忆猜参数名. 两条命令可以查到权威信息:

```shell
dida schema show task create
dida tool show create_task
```

`dida schema show` 给出的是本 CLI 的对外参数名, `dida tool show` 给出的是官方工具的原始参数名. 通过 `dida tool call` 调用时必须用官方参数名.

## 已知边界

- 官方偶尔会把业务失败包成 `{"error": "..."}` 放进成功响应里, 本 CLI 会把它提升为退出码 5 的 `upstream` 错误, 不需要再自己检查 `data.error`. 如果确实需要用原始返回判断, 用 `dida tool call` 读到的仍是同一份结果.
- `task complete` / `task delete` 这类命令官方要求同时给出 `project_id` 与 `task_id`. 只给任务 id 时, 本 CLI 会自动反查一次所属清单.
- 未指定清单的新任务落在收集箱 `inbox`.
- `view overdue` 官方没有对应工具, 是本地按截止时间筛选出来的, 因此结果准确性取决于服务端返回的未完成任务是否完整.
- `focus list` 的时间范围不得超过一个月, `focus create` 的结束时间不能晚于当前时刻, `task complete-project` 单次最多 20 个任务, 这些都是官方限制.
- 官方通道没有删除习惯的工具, 因此 `habit create` 不可撤销, 只能去网页端处理.
- `tag update` 不能改标签名, 改名请用 `tag rename`.
- `comment add` 的正文在官方契约里叫 `title`, 本 CLI 对外统一叫 `--text`.
- 搜索是最终一致而非实时: 刚创建的任务可能短时间内搜不到, 用 `dida task get <task-id>` 按 id 读取不受影响.

## 未覆盖的能力

官方 MCP 通道没有附件, 回收站, 统计报表, 模板, 共享协作这些能力, 它们在网页私有接口上. 需要这些能力时本 CLI 无法替代.
