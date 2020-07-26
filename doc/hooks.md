# Hooks

gh-ost支持  _hooks_ ：hooks可以让 gh-ost在特定兴趣点执行的外部进程。


用例如下：

- 当数据迁移失败时，你期望收到邮件。
- 当ghost推迟cut-over时，你希望能接到通知（从而准备好你去手动完成这个cut-over）
- RDS用户希望在使用 `--test-on-replica` 时不让 `gh-ost`发出 `STOP SLAVE`, RDS可以使用hooks命令RDS停止复制。
- 每小时发送状态信息到您到目录。
- Perform cleanup on the _ghost_ table (drop/rename/nibble) once migration completes
- 在迁移完成之后，执行drop，rename，nibble ghost表
- etc.

ghost 定义了一些它感兴趣的事件类型，并且会在指定的点去执行这些钩子。

注意：

返回错误码的hook将在ghost中传递错误，因此你可以强制ghost遇到你指定的condition时 迁移失败。
请确保在你真的希望其他的迁移失败时再返回错误码
也许你有不止一个到hook，ghost会依次、同步的执行你的钩子函数。 因此你通常希望钩子可以尽可能快的执行，或者在后台发出任务。

### Creating hooks

所有的hooks都希望居住在单独驻留在一个单独的目录下，这个目录由--hooks-path指定。如果不提供，那不会执行任务钩子。

`gh-ost` will dynamically search for hooks in said directory. You may add and remove hooks to/from this directory as `gh-ost` makes progress (though likely you don't want to). Hook files are expected to be executable processes.

In an effort to simplify code and to standardize usage, `gh-ost` expects hooks in explicit naming conventions. As an example, the `onStartup` hook expects processes named `gh-ost-on-startup*`. It will match and accept files named:

- `gh-ost-on-startup`
- `gh-ost-on-startup--send-notification-mail`
- `gh-ost-on-startup12345`
- etc.

The full list of supported hooks is best found in code: [hooks.go](https://github.com/github/gh-ost/blob/master/go/logic/hooks.go). Documentation will always be a bit behind. At this time, though, the following are recognized:

- `gh-ost-on-startup`
- `gh-ost-on-validated`
- `gh-ost-on-rowcount-complete`
- `gh-ost-on-before-row-copy`
- `gh-ost-on-status`
- `gh-ost-on-interactive-command`
- `gh-ost-on-row-copy-complete`
- `gh-ost-on-stop-replication`
- `gh-ost-on-start-replication`
- `gh-ost-on-begin-postponed`
- `gh-ost-on-before-cut-over`
- `gh-ost-on-success`
- `gh-ost-on-failure`

### Context

`gh-ost` will set environment variables per hook invocation. Hooks are then able to read those variables, indicating schema name, table name, `alter` statement, migrated host name etc. Some variables are available on all hooks, and some are available on relevant hooks.

The following variables are available on all hooks:

- `GH_OST_DATABASE_NAME`
- `GH_OST_TABLE_NAME`
- `GH_OST_GHOST_TABLE_NAME`
- `GH_OST_OLD_TABLE_NAME` - the name the original table will be renamed to at the end of operation
- `GH_OST_DDL`
- `GH_OST_ELAPSED_SECONDS` - total runtime
- `GH_OST_ELAPSED_COPY_SECONDS` - row-copy time (excluding startup, row-count and postpone time)
- `GH_OST_ESTIMATED_ROWS` - estimated total rows in table
- `GH_OST_COPIED_ROWS` - number of rows copied by `gh-ost`
- `GH_OST_INSPECTED_LAG` - lag in seconds (floating point) of inspected server
- `GH_OST_PROGRESS` - progress pct ([0..100], floating point) of migration
- `GH_OST_MIGRATED_HOST`
- `GH_OST_INSPECTED_HOST`
- `GH_OST_EXECUTING_HOST`
- `GH_OST_HOOKS_HINT` - copy of `--hooks-hint` value
- `GH_OST_HOOKS_HINT_OWNER` - copy of `--hooks-hint-owner` value
- `GH_OST_HOOKS_HINT_TOKEN` - copy of `--hooks-hint-token` value
- `GH_OST_DRY_RUN` - whether or not the `gh-ost` run is a dry run

The following variable are available on particular hooks:

- `GH_OST_COMMAND` is only available in `gh-ost-on-interactive-command`
- `GH_OST_STATUS` is only available in `gh-ost-on-status`

### Examples

See [sample hooks](https://github.com/github/gh-ost/tree/master/resources/hooks-sample), as `bash` implementation samples.
