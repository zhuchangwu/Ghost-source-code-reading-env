
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gh-ost/go/base"
	"gh-ost/go/logic"
	_ "github.com/go-sql-driver/mysql"
	"github.com/outbrain/golib/log"

	"golang.org/x/crypto/ssh/terminal"
)

//应用版本
var AppVersion string

// acceptSignals registers for OS signals
func acceptSignals(migrationContext *base.MigrationContext) {
	c := make(chan os.Signal, 1)

	signal.Notify(c, syscall.SIGHUP)
	go func() {
		for sig := range c {
			switch sig {
			case syscall.SIGHUP:
				log.Infof("Received SIGHUP. Reloading configuration")
				if err := migrationContext.ReadConfigFile(); err != nil {
					log.Errore(err)
				} else {
					migrationContext.MarkPointOfInterest()
				}
			}
		}
	}()
}

// main is the application's entry point. It will either spawn a CLI or HTTP interfaces.
func main() {
	//创建一个数据迁移上下文
	migrationContext := base.NewMigrationContext()
	// todo
	//参数host 主机ip
	flag.StringVar(&migrationContext.InspectorConnectionConfig.Key.Hostname, "host", "127.0.0.1", "MySQL hostname (preferably a replica, not the master)")

	//todo
	//参数assume-master-host  为gh-ost指定一个主库，格式为”ip:port”或者”hostname:port”。这在主主架构里比较有用，或则在gh-ost发现不到主的时候有用。
	flag.StringVar(&migrationContext.AssumeMasterHostname, "assume-master-host", "127.0.0.1:3307", "(optional) explicitly tell gh-ost the identity of the master. Format: some.host.com[:port] This is useful in master-master setups where you wish to pick an explicit master, or in a tungsten-replicator where gh-ost is unable to determine the master")
	// todo
	//参数port指定端口
	flag.IntVar(&migrationContext.InspectorConnectionConfig.Key.Port, "port", 3307, "MySQL port (preferably a replica, not the master)")
	//user指定mysql 用户
	flag.StringVar(&migrationContext.CliUser, "user", "root", "MySQL user")
	//MySQL登录密码 password
	flag.StringVar(&migrationContext.CliPassword, "password", "root", "MySQL password")
	//主库上的用户 当其与从库上的不一样时需要配合--assume-master-host这个参数
	flag.StringVar(&migrationContext.CliMasterUser, "master-user", "", "MySQL user on master, if different from that on replica. Requires --assume-master-host")
	//主库上的密码
	flag.StringVar(&migrationContext.CliMasterPassword, "master-password", "", "MySQL password on master, if different from that on replica. Requires --assume-master-host")
	//配置文件
	flag.StringVar(&migrationContext.ConfigFile, "conf", "", "Config file")
	//提示输入mysql密码
	askPass := flag.Bool("ask-pass", false, "prompt for MySQL password")
	//启用到MySQL主机的SSL加密连接
	flag.BoolVar(&migrationContext.UseTLS, "ssl", false, "Enable SSL encrypted connections to MySQL hosts")
	//用于TLS连接到MySQL主机的PEM格式的CA证书。需要--ssl
	flag.StringVar(&migrationContext.TLSCACertificate, "ssl-ca", "", "CA certificate in PEM format for TLS connections to MySQL hosts. Requires --ssl")
	//用于TLS连接到MySQL主机的PEM格式证书。需要--ssl
	flag.StringVar(&migrationContext.TLSCertificate, "ssl-cert", "", "Certificate in PEM format for TLS connections to MySQL hosts. Requires --ssl")
	//用于TLS连接到MySQL主机的PEM格式KEY。需要--ssl
	flag.StringVar(&migrationContext.TLSKey, "ssl-key", "", "Key in PEM format for TLS connections to MySQL hosts. Requires --ssl")
	//跳过MySQL主机证书链和主机名的验证。需要--ssl
	flag.BoolVar(&migrationContext.TLSAllowInsecure, "ssl-allow-insecure", false, "Skips verification of MySQL hosts' certificate chain and host name. Requires --ssl")
	// todo
	//数据库名称(必填项)
	flag.StringVar(&migrationContext.DatabaseName, "database", "lossless_ddl_test", "database name (mandatory)")
	// todo
	//表名(必填项)
	flag.StringVar(&migrationContext.OriginalTableName, "table", "user", "table name (mandatory)")
	//变更sql语句(必填项)
	// todo sql的不用显示的添加的 alter
	flag.StringVar(&migrationContext.AlterStatement, "alter", "add column newC9 varchar(24);", "alter statement (mandatory)")
	// todo
	//实际计算表行数，而不是估计它们(是为了更准确的进度估计)
	flag.BoolVar(&migrationContext.CountTableRows, "exact-rowcount", true, "actually count table rows as opposed to estimate them (results in more accurate progress estimation)")
	// todo
	//和--exact-rowcount参数搭配使用  true（默认值）:在行复制开始后同时计算行数，并在以后调整行估计值 false:首先计算行数，然后开始行复制
	flag.BoolVar(&migrationContext.ConcurrentCountTableRows, "concurrent-rowcount", true, "(with --exact-rowcount), when true (default): count rows after row-copy begins, concurrently, and adjust row estimate later on; when false: first count rows, then start row copy")
	// todo 在主库上迁移，copy数据时从主库表上select。 在从库上迁移，copy数据时从从库表上select
	//允许此迁移直接在主库上执行。建议在备库上执行
	flag.BoolVar(&migrationContext.AllowedRunningOnMaster, "allow-on-master", true, "allow this migration to run directly on master. Preferably it would run on a replica")
	// todo
	//显式允许在主主架构Mysql中运行
	flag.BoolVar(&migrationContext.AllowedMasterMaster, "allow-master-master", true, "explicitly allow running in a master-master setup")
	//允许gh ost基于具有可空列的唯一键进行迁移。只要不存在空值，就可以了。如果所选密钥中存在空值，则数据可能已损坏。使用风险自负！
	flag.BoolVar(&migrationContext.NullableUniqueKeyAllowed, "allow-nullable-unique-key", false, "allow gh-ost to migrate based on a unique key with nullable columns. As long as no NULL values exist, this should be OK. If NULL values exist in chosen key, data may be corrupted. Use at your own risk!")
	//如果“ALTER”语句重命名列，gh ost将注意到这一点并提供对重命名的解释。默认情况下，gh ost不会继续执行。这个标志证明了gh ost的解释是正确的
	flag.BoolVar(&migrationContext.ApproveRenamedColumns, "approve-renamed-columns", false, "in case your `ALTER` statement renames columns, gh-ost will note that and offer its interpretation of the rename. By default gh-ost does not proceed to execute. This flag approves that gh-ost's interpretation is correct")
	//如果“ALTER”语句重命名列，gh ost将注意到这一点并提供对重命名的解释。默认情况下，gh ost不会继续执行。此标志告诉gh ost跳过重命名的列，即将ghost认为重命名的列视为不相关的列。注意：可能会丢失列数据
	flag.BoolVar(&migrationContext.SkipRenamedColumns, "skip-renamed-columns", false, "in case your `ALTER` statement renames columns, gh-ost will note that and offer its interpretation of the rename. By default gh-ost does not proceed to execute. This flag tells gh-ost to skip the renamed columns, i.e. to treat what gh-ost thinks are renamed columns as unrelated columns. NOTE: you may lose column data")
	//显式地让gh ost知道您正在tungsten-replication的拓扑上运行（您可能还提供--assume-master-host参数）
	flag.BoolVar(&migrationContext.IsTungsten, "tungsten", false, "explicitly let gh-ost know that you are running on a tungsten-replication based topology (you are likely to also provide --assume-master-host)")
	//危险！此标志将迁移具有外键的表，并且不会在ghost表上创建外键，因此更改后的表将没有外键。这对于有意丢弃外键很有用
	flag.BoolVar(&migrationContext.DiscardForeignKeys, "discard-foreign-keys", false, "DANGER! This flag will migrate a table that has foreign keys and will NOT create foreign keys on the ghost table, thus your altered table will have NO foreign keys. This is useful for intentional dropping of foreign keys")
	//跳过外键检查
	flag.BoolVar(&migrationContext.SkipForeignKeyChecks, "skip-foreign-key-checks", false, "set to 'true' when you know for certain there are no foreign keys on your table, and wish to skip the time it takes for gh-ost to verify that")
	//跳过严格的sql模式
	flag.BoolVar(&migrationContext.SkipStrictMode, "skip-strict-mode", false, "explicitly tell gh-ost binlog applier not to enforce strict sql mode")
	//使用阿里云RDS
	flag.BoolVar(&migrationContext.AliyunRDS, "aliyun-rds", false, "set to 'true' when you execute on Aliyun RDS.")
	//如果使用的是GCP 要设置这个参数为true
	flag.BoolVar(&migrationContext.GoogleCloudPlatform, "gcp", false, "set to 'true' when you execute on a 1st generation Google Cloud Platform (GCP).")
	// todo execute参数为false表示预执行
	// todo 为ture表示真正执行
	//实际执行表变更或者数据迁移   默认情况下只做一些测试然后退出
	executeFlag := flag.Bool("execute", true, "actually execute the alter & migrate the table. Default is noop: do some tests and exit")
	//让迁移在备库上执行，而不是在主库上执行。迁移结束时，复制将停止，表将交换并立即交换还原。复制保持停止，您可以比较这两个表以确认
	flag.BoolVar(&migrationContext.TestOnReplica, "test-on-replica", false, "Have the migration run on a replica, not on the master. At the end of migration replication is stopped, and tables are swapped and immediately swap-revert. Replication remains stopped and you can compare the two tables for building trust")
	//启用--test on replica时，不要发出停止复制的命令（需要--test on replica）
	flag.BoolVar(&migrationContext.TestOnReplicaSkipReplicaStop, "test-on-replica-skip-replica-stop", false, "When --test-on-replica is enabled, do not issue commands stop replication (requires --test-on-replica)")
	//让迁移在备库上运行，而不是在主库上运行。这将在复制副本上执行完整迁移，包括切换（与--test-on-replica参数相反）
	flag.BoolVar(&migrationContext.MigrateOnReplica, "migrate-on-replica", false, "Have the migration run on a replica, not on the master. This will do the full migration on the replica including cut-over (as opposed to --test-on-replica)")
	//是否删除原表 默认不删除 因为删除原表的操作是一个耗时操作
	flag.BoolVar(&migrationContext.OkToDropTable, "ok-to-drop-table", false, "Shall the tool drop the old table at end of operation. DROPping tables can be a long locking operation, which is why I'm not doing it by default. I'm an online tool, yes?")

	//todo 是否删除上次执行在线DDL的old表  默认情况下 如果这样的表存在会panic
	flag.BoolVar(&migrationContext.InitiallyDropOldTable, "initially-drop-old-table", true, "Drop a possibly existing OLD table (remains from a previous run?) before beginning operation. Default is to panic and abort if such table exists")

	// todo 执行失败时会存在残留表，通过initially-drop-ghost-table = true 可以强制删除这个表
	//是否删除上次执行在线DDL的o残余表  默认情况下 如果这样的表存在会panic
	flag.BoolVar(&migrationContext.InitiallyDropGhostTable, "initially-drop-ghost-table", true, "Drop a possibly existing Ghost table (remains from a previous run?) before beginning operation. Default is to panic and abort if such table exists")
	//在旧表名中使用时间戳。这使得旧表名是唯一的，并且不存在冲突的交叉迁移
	flag.BoolVar(&migrationContext.TimestampOldTable, "timestamp-old-table", false, "Use a timestamp in old table name. This makes old table names unique and non conflicting cross migrations")
	// todo
	//重命名表是一步完成还是分成两步, value="atomic"
	cutOver := flag.String("cut-over", "default", "choose cut-over type (default|atomic, two-step)")
	//如果为true，则“unospone | cut-over”交互命令必须命名迁移的表
	flag.BoolVar(&migrationContext.ForceNamedCutOverCommand, "force-named-cut-over", false, "When true, the 'unpostpone|cut-over' interactive command must name the migrated table")
	//如果为true，则“panic”交互命令必须命名迁移的表
	flag.BoolVar(&migrationContext.ForceNamedPanicCommand, "force-named-panic", false, "When true, the 'panic' interactive command must name the migrated table")
	// todo
	//将bin log 格式转换为RBR格式
	flag.BoolVar(&migrationContext.SwitchToRowBinlogFormat, "switch-to-rbr", true, "let this tool automatically switch binary log format to 'ROW' on the replica, if needed. The format will NOT be switched back. I'm too scared to do that, and wish to protect you if you happen to execute another migration while this one is running")
	//如果确定MySQL 的bin log格式是ROW格式的话这个值可以设置为true
	flag.BoolVar(&migrationContext.AssumeRBR, "assume-rbr", false, "set to 'true' when you know for certain your server uses 'ROW' binlog_format. gh-ost is unable to tell, event after reading binlog_format, whether the replication process does indeed use 'ROW', and restarts replication to be certain RBR setting is applied. Such operation requires SUPER privileges which you might not have. Setting this flag avoids restarting replication and you can proceed to use gh-ost without SUPER privileges")
	//失败的切换尝试之间的间隔呈指数增长。等待间隔服从“指数后退最大间隔”的最大可配置值
	flag.BoolVar(&migrationContext.CutOverExponentialBackoff, "cut-over-exponential-backoff", false, "Wait exponentially longer intervals between failed cut-over attempts. Wait intervals obey a maximum configurable with 'exponential-backoff-max-interval').")
	//执行指数后退的各种操作时，两次尝试之间等待的最大秒数
	exponentialBackoffMaxInterval := flag.Int64("exponential-backoff-max-interval", 64, "Maximum number of seconds to wait between attempts when performing various operations with exponential backoff.")
	//每次迭代中要处理的行数 范围从100 - 100000
	chunkSize := flag.Int64("chunk-size", 1000, "amount of rows to handle in each iteration (allowed range: 100-100,000)")
	//要在单个事务中应用的DML事件的批处理大小
	dmlBatchSize := flag.Int64("dml-batch-size", 10, "batch size for DML events to apply in a single transaction (range 1-100)")
	// todo
	//默认重试次数
	defaultRetries := flag.Int64("default-retries", 60, "Default number of retries for various operations before panicking")
	//尝试切换时保留表锁的最大秒数（当锁超过超时时重试）
	cutOverLockTimeoutSeconds := flag.Int64("cut-over-lock-timeout-seconds", 3, "Max number of seconds to hold locks on tables while attempting to cut-over (retry attempted when lock exceeds timeout)")
	//每次chunk时间段的休眠时间，范围[0.0…100.0]。0：每个chunk时间段不休眠，即一个chunk接着一个chunk执行；1：每row-copy 1毫秒，则另外休眠1毫秒；0.7：每row-copy 10毫秒，则另外休眠7毫秒。
	niceRatio := flag.Float64("nice-ratio", 0, "force being 'nice', imply sleep time per chunk time; range: [0.0..100.0]. Example values: 0 is aggressive. 1: for every 1ms spent copying rows, sleep additional 1ms (effectively doubling runtime); 0.7: for every 10ms spend in a rowcopy chunk, spend 7ms sleeping immediately after")
	//限制操作的复制延迟
	maxLagMillis := flag.Int64("max-lag-millis", 1500, "replication lag at which to throttle operation")
	//已弃用。gh ost使用一个内部的、亚秒级的分辨率查询
	replicationLagQuery := flag.String("replication-lag-query", "", "Deprecated. gh-ost uses an internal, subsecond resolution query")
	// todo
	//要检查其延迟的备库列表 逗号分隔
	throttleControlReplicas := flag.String("throttle-control-replicas", "127.0.0.1:3308,127.0.0.1:3309", "List of replicas on which to check for lag; comma delimited. Example: myhost1.com:3306,myhost2.com,myhost3.com:3307")
	//是否限流  值为0表示不限流 大于0表示限流
	throttleQuery := flag.String("throttle-query", "", "when given, issued (every second) to check if operation should throttle. Expecting to return zero for no-throttle, >0 for throttle. Query is issued on the migrated server. Make sure this query is lightweight")
	//只要http请求返回的状态码不是200 就限流 确保它具有低延迟响应
	throttleHTTP := flag.String("throttle-http", "", "when given, gh-ost checks given URL via HEAD request; any response code other than 200 (OK) causes throttling; make sure it has low latency response")
	//在限流检查时忽略HTTP错误
	ignoreHTTPErrors := flag.Bool("ignore-http-errors", false, "ignore HTTP connection errors during throttle check")
	//间隔多久写入一次心跳数据
	heartbeatIntervalMillis := flag.Int64("heartbeat-interval-millis", 100, "how frequently would gh-ost inject a heartbeat value")
	//当这个文件存在时 操作会停止  这个文件的明明最好要和操作的表名相关
	flag.StringVar(&migrationContext.ThrottleFlagFile, "throttle-flag-file", "", "operation pauses when this file exists; hint: use a file that is specific to the table being altered")
	//这个文件存在的话操作会停止 保留默认值即可，用于限制多个gh ost操作
	flag.StringVar(&migrationContext.ThrottleAdditionalFlagFile, "throttle-additional-flag-file", "/tmp/gh-ost.throttle", "operation pauses when this file exists; hint: keep default, use for throttling multiple gh-ost operations")
	//当这个文件存在时，迁移将推迟交换表的最后阶段，并将继续同步ghost表。一旦文件被删除，切换/交换就可以执行了。
	flag.StringVar(&migrationContext.PostponeCutOverFlagFile, "postpone-cut-over-flag-file", "", "while this file exists, migration will postpone the final stage of swapping tables, and will keep on syncing the ghost table. Cut-over/swapping would be ready to perform the moment the file is deleted.")
	// todo
	//创建此文件时，gh ost将立即终止，而不进行清理
	flag.StringVar(&migrationContext.PanicFlagFile, "panic-flag-file", "/tmp/ghost.panic.flag", "when this file is created, gh-ost will immediately terminate, without cleanup")
	//强制删除现有的套接字文件。小心：这可能会删除正在运行的迁移的套接字文件！
	flag.BoolVar(&migrationContext.DropServeSocket, "initially-drop-socket-file", false, "Should gh-ost forcibly delete an existing socket file. Be careful: this might drop the socket file of a running migration!")
	//要服务的Unix套接字文件。默认值：启动时自动确定
	flag.StringVar(&migrationContext.ServeSocketFile, "serve-socket-file", "", "Unix socket file to serve on. Default: auto-determined and advertised upon startup")
	//TCP 端口 默认不启用
	flag.Int64Var(&migrationContext.ServeTCPPort, "serve-tcp-port", 0, "TCP port to serve on. Default: disabled")
	//找到钩子文件的目录（默认值：空，即钩子被禁用）。将执行在此路径上找到的符合钩子命名约定的钩子文件
	flag.StringVar(&migrationContext.HooksPath, "hooks-path", "", "directory where hook files are found (default: empty, ie. hooks disabled). Hook files found on this path, and conforming to hook naming conventions will be executed")
	//为方便起见，通过GH OST_hooks_提示将任意消息注入hooks
	flag.StringVar(&migrationContext.HooksHintMessage, "hooks-hint", "", "arbitrary message to be injected to hooks via GH_OST_HOOKS_HINT, for your convenience")
	//为了您的方便，可以通过GH OST_hooks_HINT_owner将所有者的任意名称注入hooks
	flag.StringVar(&migrationContext.HooksHintOwner, "hooks-hint-owner", "", "arbitrary name of owner to be injected to hooks via GH_OST_HOOKS_HINT_OWNER, for your convenience")
	//通过GH OST_hooks_HINT_令牌注入钩子的任意令牌，以方便您
	flag.StringVar(&migrationContext.HooksHintToken, "hooks-hint-token", "", "arbitrary token to be injected to hooks via GH_OST_HOOKS_HINT_TOKEN, for your convenience")
	//server id
	flag.UintVar(&migrationContext.ReplicaServerId, "replica-server-id", 99999, "server id used by gh-ost process. Default: 99999")
	// todo
	//超过最大负载 限制写
	maxLoad := flag.String("max-load", "Threads_running=25", "Comma delimited status-name=threshold. e.g: 'Threads_running=100,Threads_connected=500'. When status exceeds threshold, app throttles writes")
	// todo
	//超过最大值 应用panic 并退出
	criticalLoad := flag.String("critical-load", "Threads_running=1000", "Comma delimited status-name=threshold, same format as --max-load. When status exceeds threshold, app panics and quits")
	//0时，迁移在遇到临界负载时立即释放。当非零时，在给定的时间间隔后进行第二次检查，并且只有在第二次检查仍满足临界负载时迁移才会退出
	flag.Int64Var(&migrationContext.CriticalLoadIntervalMilliseconds, "critical-load-interval-millis", 0, "When 0, migration immediately bails out upon meeting critical-load. When non-zero, a second check is done after given interval, and migration only bails out if 2nd check still meets critical load")
	//当非零时，临界负载不会panic 和退出；相反，gh ost在指定的持续时间内进入休眠状态。它不会向任何服务器读/写任何内容
	flag.Int64Var(&migrationContext.CriticalLoadHibernateSeconds, "critical-load-hibernate-seconds", 0, "When nonzero, critical-load does not panic and bail out; instead, gh-ost goes into hibernate for the specified duration. It will not read/write anything to from/to any server")
	quiet := flag.Bool("quiet", false, "quiet")
	// todo
	verbose := flag.Bool("verbose", true, "verbose")
	debug := flag.Bool("debug", true, "debug mode (very verbose)")
	stack := flag.Bool("stack", true, "add stack trace upon error")
	help := flag.Bool("help", false, "Display usage")
	version := flag.Bool("version", false, "Print version & exit")

	//检查是否存在/支持另一个标志。这允许跨版本脚本。当所有其他提供的标志都存在时，以0退出，否则为非零。必须为需要值的标志提供（伪）值
	checkFlag := flag.Bool("check-flag", false, "Check if another flag exists/supported. This allows for cross-version scripting. Exits with 0 when all additional provided flags exist, nonzero otherwise. You must provide (dummy) values for flags that require a value. Example: gh-ost --check-flag --cut-over-lock-timeout-seconds --nice-ratio 0")
	//要在临时表上使用的表名前缀
	flag.StringVar(&migrationContext.ForceTmpTableName, "force-table-names", "", "table name prefix to be used on the temporary tables")
	flag.CommandLine.SetOutput(os.Stdout)

	flag.Parse()

	//参数不正确 结束程序
	if *checkFlag {
		return
	}
	if *help {
		//输出帮助信息(以及默认参数)
		fmt.Fprintf(os.Stdout, "Usage of gh-ost:\n")
		flag.PrintDefaults()
		return
	}
	if *version {
		//输入软件版本
		appVersion := AppVersion
		if appVersion == "" {
			appVersion = "unversioned"
		}
		fmt.Println(appVersion)
		return
	}

	//设置日志级别(默认)
	log.SetLevel(log.ERROR)
	//如果指定日志级别为INFO
	if *verbose {
		log.SetLevel(log.INFO)
	}
	//如果指定日志级别为DEBUG
	if *debug {
		log.SetLevel(log.DEBUG)
	}
	//设置是否打印堆栈信息
	if *stack {
		log.SetPrintStackTrace(*stack)
	}
	//日志别满屏刷(安静点！=把日志级别调高:))
	if *quiet {
		// Override!!
		log.SetLevel(log.ERROR)
	}
	//对必填项的检查 start
	if migrationContext.DatabaseName == "" {
		log.Fatalf("--database must be provided and database name must not be empty")
	}
	if migrationContext.OriginalTableName == "" {
		log.Fatalf("--table must be provided and table name must not be empty")
	}
	if migrationContext.AlterStatement == "" {
		log.Fatalf("--alter must be provided and statement must not be empty")
	}
	//对必填项的检查 end

	//互斥参数检查(两个参数不能同时使用) start
	migrationContext.Noop = !(*executeFlag)
	if migrationContext.AllowedRunningOnMaster && migrationContext.TestOnReplica {
		log.Fatalf("--allow-on-master and --test-on-replica are mutually exclusive")
	}
	if migrationContext.AllowedRunningOnMaster && migrationContext.MigrateOnReplica {
		log.Fatalf("--allow-on-master and --migrate-on-replica are mutually exclusive")
	}
	if migrationContext.MigrateOnReplica && migrationContext.TestOnReplica {
		log.Fatalf("--migrate-on-replica and --test-on-replica are mutually exclusive")
	}
	if migrationContext.SwitchToRowBinlogFormat && migrationContext.AssumeRBR {
		log.Fatalf("--switch-to-rbr and --assume-rbr are mutually exclusive")
	}

	//互斥参数检查(两个参数不能同时使用) end

	//两个参数必须搭配使用检查 start
	if migrationContext.TestOnReplicaSkipReplicaStop {
		if !migrationContext.TestOnReplica {
			log.Fatalf("--test-on-replica-skip-replica-stop requires --test-on-replica to be enabled")
		}
		log.Warning("--test-on-replica-skip-replica-stop enabled. We will not stop replication before cut-over. Ensure you have a plugin that does this.")
	}
	if migrationContext.CliMasterUser != "" && migrationContext.AssumeMasterHostname == "" {
		log.Fatalf("--master-user requires --assume-master-host")
	}
	if migrationContext.CliMasterPassword != "" && migrationContext.AssumeMasterHostname == "" {
		log.Fatalf("--master-password requires --assume-master-host")
	}
	if migrationContext.TLSCACertificate != "" && !migrationContext.UseTLS {
		log.Fatalf("--ssl-ca requires --ssl")
	}
	if migrationContext.TLSCertificate != "" && !migrationContext.UseTLS {
		log.Fatalf("--ssl-cert requires --ssl")
	}
	if migrationContext.TLSKey != "" && !migrationContext.UseTLS {
		log.Fatalf("--ssl-key requires --ssl")
	}
	if migrationContext.TLSAllowInsecure && !migrationContext.UseTLS {
		log.Fatalf("--ssl-allow-insecure requires --ssl")
	}
	//两个参数必须搭配使用检查 end

	//过时参数检查
	if *replicationLagQuery != "" {
		log.Warningf("--replication-lag-query is deprecated")
	}

	//判断表的重命名是一步完成还是分成两步完成，默认一步完成
	switch *cutOver {
	case "atomic", "default", "":
		migrationContext.CutOverType = base.CutOverAtomic
	case "two-step":
		migrationContext.CutOverType = base.CutOverTwoStep
	default:
		log.Fatalf("Unknown cut-over: %s", *cutOver)
	}
	//读取配置出错
	if err := migrationContext.ReadConfigFile(); err != nil {
		log.Fatale(err)
	}

	//读取要限流的实例信息并设置
	if err := migrationContext.ReadThrottleControlReplicaKeys(*throttleControlReplicas); err != nil {
		log.Fatale(err)
	}
	//读取最大负载
	if err := migrationContext.ReadMaxLoad(*maxLoad); err != nil {
		log.Fatale(err)
	}
	//读取并更细最大阈值
	if err := migrationContext.ReadCriticalLoad(*criticalLoad); err != nil {
		log.Fatale(err)
	}
	//如果ServeSocketFile文件没有指定 指定一个默认的
	if migrationContext.ServeSocketFile == "" {
		migrationContext.ServeSocketFile = fmt.Sprintf("/tmp/gh-ost.%s.%s.sock", migrationContext.DatabaseName, migrationContext.OriginalTableName)
	}

	//提示用户输入密码
	if *askPass {
		fmt.Println("Password:")
		//从控制台读取用户输入的密码
		bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatale(err)
		}
		migrationContext.CliPassword = string(bytePassword)
	}
	//设置心跳间隔
	migrationContext.SetHeartbeatIntervalMilliseconds(*heartbeatIntervalMillis)
	//设置是否间歇性迁移
	migrationContext.SetNiceRatio(*niceRatio)
	//设置每次迭代中要处理的行数
	migrationContext.SetChunkSize(*chunkSize)
	//设置在单个事务中应用的DML事件的批处理大小
	migrationContext.SetDMLBatchSize(*dmlBatchSize)
	//设置限制操作的复制延迟
	migrationContext.SetMaxLagMillisecondsThrottleThreshold(*maxLagMillis)
	//设置是否限流
	migrationContext.SetThrottleQuery(*throttleQuery)
	//设置只要http请求返回的状态码不是200 就限流 确保它具有低延迟响应
	migrationContext.SetThrottleHTTP(*throttleHTTP)
	//设置忽略HTTP错误
	migrationContext.SetIgnoreHTTPErrors(*ignoreHTTPErrors)
	//设置默认重试次数
	migrationContext.SetDefaultNumRetries(*defaultRetries)
	migrationContext.ApplyCredentials()
	//设置TLS出错
	if err := migrationContext.SetupTLS(); err != nil {
		log.Fatale(err)
	}
	//设置重命名表锁表超时时间出错
	if err := migrationContext.SetCutOverLockTimeoutSeconds(*cutOverLockTimeoutSeconds); err != nil {
		log.Errore(err)
	}
	//设置二进制回退最大时间间隔
	if err := migrationContext.SetExponentialBackoffMaxInterval(*exponentialBackoffMaxInterval); err != nil {
		log.Errore(err)
	}
	//打印启动成功的信息及软件版本
	log.Infof("starting gh-ost %+v", AppVersion)
	//读取/热加载配置文件
	acceptSignals(migrationContext)
	//创建一个迁移器
 	migrator := logic.NewMigrator(migrationContext)
	//开始迁移
	err := migrator.Migrate()
	if err != nil {
		migrator.ExecOnFailureHook()
		log.Fatale(err)
	}
	//迁移完成
	fmt.Fprintf(os.Stdout, "# Done\n")
	time.Sleep(30*time.Second)
}
