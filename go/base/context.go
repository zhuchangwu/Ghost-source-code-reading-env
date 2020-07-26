/*
   Copyright 2016 GitHub Inc.
	 See https://github.com/github/gh-ost/blob/master/LICENSE
*/

package base

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/satori/go.uuid"

	"gh-ost/go/mysql"
	"gh-ost/go/sql"

	"gopkg.in/gcfg.v1"
	gcfgscanner "gopkg.in/gcfg.v1/scanner"
)

// RowsEstimateMethod is the type of row number estimation
type RowsEstimateMethod string

const (
	TableStatusRowsEstimate RowsEstimateMethod = "TableStatusRowsEstimate"
	ExplainRowsEstimate     RowsEstimateMethod = "ExplainRowsEstimate"
	CountRowsEstimate       RowsEstimateMethod = "CountRowsEstimate"
)

type CutOver int

const (
	//重命名表名一步完成
	CutOverAtomic CutOver = iota
	//重命名表名分成两步
	CutOverTwoStep
)

type ThrottleReasonHint string

const (
	NoThrottleReasonHint                 ThrottleReasonHint = "NoThrottleReasonHint"
	UserCommandThrottleReasonHint        ThrottleReasonHint = "UserCommandThrottleReasonHint"
	LeavingHibernationThrottleReasonHint ThrottleReasonHint = "LeavingHibernationThrottleReasonHint"
)

const (
	HTTPStatusOK       = 200
	MaxEventsBatchSize = 1000
)

var (
	envVariableRegexp = regexp.MustCompile("[$][{](.*)[}]")
)

type ThrottleCheckResult struct {
	ShouldThrottle bool
	Reason         string
	ReasonHint     ThrottleReasonHint
}

func NewThrottleCheckResult(throttle bool, reason string, reasonHint ThrottleReasonHint) *ThrottleCheckResult {
	return &ThrottleCheckResult{
		ShouldThrottle: throttle,
		Reason:         reason,
		ReasonHint:     reasonHint,
	}
}

// MigrationContext has the general, global state of migration. It is used by
// all components throughout the migration process.
type MigrationContext struct {
	Uuid string

	DatabaseName      string
	OriginalTableName string
	AlterStatement    string

	CountTableRows           bool
	ConcurrentCountTableRows bool
	AllowedRunningOnMaster   bool
	AllowedMasterMaster      bool
	SwitchToRowBinlogFormat  bool
	AssumeRBR                bool
	SkipForeignKeyChecks     bool
	SkipStrictMode           bool
	NullableUniqueKeyAllowed bool
	//列名是否允许重命名
	ApproveRenamedColumns    bool
	SkipRenamedColumns       bool
	IsTungsten               bool
	DiscardForeignKeys       bool
	AliyunRDS                bool
	GoogleCloudPlatform      bool

	config            ContextConfig
	configMutex       *sync.Mutex
	ConfigFile        string
	CliUser           string
	CliPassword       string
	UseTLS            bool
	TLSAllowInsecure  bool
	TLSCACertificate  string
	TLSCertificate    string
	TLSKey            string
	CliMasterUser     string
	CliMasterPassword string

	HeartbeatIntervalMilliseconds       int64
	defaultNumRetries                   int64
	ChunkSize                           int64
	niceRatio                           float64
	MaxLagMillisecondsThrottleThreshold int64
	//要限流的实例
	throttleControlReplicaKeys          *mysql.InstanceKeyMap
	ThrottleFlagFile                    string
	ThrottleAdditionalFlagFile          string
	throttleQuery                       string
	throttleHTTP                        string
	IgnoreHTTPErrors                    bool
	ThrottleCommandedByUser             int64
	HibernateUntil                      int64
	maxLoad                             LoadMap
	criticalLoad                        LoadMap
	CriticalLoadIntervalMilliseconds    int64
	CriticalLoadHibernateSeconds        int64
	PostponeCutOverFlagFile             string
	CutOverLockTimeoutSeconds           int64
	CutOverExponentialBackoff           bool
	ExponentialBackoffMaxInterval       int64
	ForceNamedCutOverCommand            bool
	ForceNamedPanicCommand              bool
	PanicFlagFile                       string
	HooksPath                           string
	HooksHintMessage                    string
	HooksHintOwner                      string
	HooksHintToken                      string

	DropServeSocket bool
	ServeSocketFile string
	ServeTCPPort    int64

	Noop                         bool
	TestOnReplica                bool
	MigrateOnReplica             bool
	TestOnReplicaSkipReplicaStop bool
	OkToDropTable                bool
	InitiallyDropOldTable        bool
	InitiallyDropGhostTable      bool
	TimestampOldTable            bool // Should old table name include a timestamp
	CutOverType                  CutOver
	ReplicaServerId              uint

	Hostname                  string
	AssumeMasterHostname      string
	ApplierTimeZone           string
	TableEngine               string
	RowsEstimate              int64
	RowsDeltaEstimate         int64
	UsedRowsEstimateMethod    RowsEstimateMethod
	HasSuperPrivilege         bool
	OriginalBinlogFormat      string
	OriginalBinlogRowImage    string
	InspectorConnectionConfig *mysql.ConnectionConfig
	InspectorMySQLVersion     string
	ApplierConnectionConfig   *mysql.ConnectionConfig
	ApplierMySQLVersion       string
	StartTime                 time.Time
	RowCopyStartTime          time.Time
	RowCopyEndTime            time.Time
	LockTablesStartTime       time.Time
	RenameTablesStartTime     time.Time
	RenameTablesEndTime       time.Time
	//配置文件最后一次更新时间
	pointOfInterestTime        time.Time
	pointOfInterestTimeMutex   *sync.Mutex
	CurrentLag                 int64
	currentProgress            uint64
	ThrottleHTTPStatusCode     int64
	controlReplicasLagResult   mysql.ReplicationLagResult
	TotalRowsCopied            int64
	TotalDMLEventsApplied      int64
	DMLBatchSize               int64
	isThrottled                bool
	throttleReason             string
	throttleReasonHint         ThrottleReasonHint
	throttleGeneralCheckResult ThrottleCheckResult
	//限流互斥锁
	throttleMutex                          *sync.Mutex
	throttleHTTPMutex                      *sync.Mutex
	IsPostponingCutOver                    int64
	CountingRowsFlag                       int64
	AllEventsUpToLockProcessedInjectedFlag int64
	CleanupImminentFlag                    int64
	UserCommandedUnpostponeFlag            int64
	CutOverCompleteFlag                    int64
	InCutOverCriticalSectionFlag           int64
	//ghost中有众多的goroutine， 当有goroutine发生panic时，将error写入PanicAbort chan中，在migrator.go中的Migrator函数中，会单独开启一条协程消费这个chan
	PanicAbort                             chan error

	OriginalTableColumnsOnApplier *sql.ColumnList
	OriginalTableColumns          *sql.ColumnList
	OriginalTableVirtualColumns   *sql.ColumnList
	OriginalTableUniqueKeys       [](*sql.UniqueKey)
	GhostTableColumns             *sql.ColumnList
	GhostTableVirtualColumns      *sql.ColumnList
	GhostTableUniqueKeys          [](*sql.UniqueKey)
	UniqueKey                     *sql.UniqueKey
	SharedColumns                 *sql.ColumnList
	ColumnRenameMap               map[string]string
	//已经完成重命名的列名MAP
	DroppedColumnsMap             map[string]bool
	MappedSharedColumns           *sql.ColumnList
	MigrationRangeMinValues       *sql.ColumnValues
	MigrationRangeMaxValues       *sql.ColumnValues
	//迭代次数
	Iteration                        int64
	MigrationIterationRangeMinValues *sql.ColumnValues
	MigrationIterationRangeMaxValues *sql.ColumnValues
	ForceTmpTableName                string

	recentBinlogCoordinates mysql.BinlogCoordinates
}

type ContextConfig struct {
	Client struct {
		User     string
		Password string
	}
	Osc struct {
		Chunk_Size            int64
		Max_Lag_Millis        int64
		Replication_Lag_Query string
		Max_Load              string
	}
}

func NewMigrationContext() *MigrationContext {
	return &MigrationContext{
		//唯一标识这次迁移
		Uuid:                                uuid.NewV4().String(),
		//默认重试次数是60
		defaultNumRetries:                   60,
		//每次迭代中默认处理的行数是1000行
		ChunkSize:                           1000,
		//连接mysql的最小配置文件(备库？)
		InspectorConnectionConfig:           mysql.NewConnectionConfig(),
		//连接mysql的最小配置文件(主？)
		ApplierConnectionConfig:             mysql.NewConnectionConfig(),
		//超过1500毫秒的延迟就进行限流
		MaxLagMillisecondsThrottleThreshold: 1500,
		//对表进行重命名最长锁表时间为3秒 // todo 失败了会怎样
		CutOverLockTimeoutSeconds:           3,
		//要在单个事务中应用的DML事件的批处理大小默认为10
		DMLBatchSize:                        10,
		//最大负载
		maxLoad:                             NewLoadMap(),
		//最大值
		criticalLoad:                        NewLoadMap(),
		//限流互斥锁
		throttleMutex:                       &sync.Mutex{},
		//HTTP限流互斥锁
		throttleHTTPMutex:                   &sync.Mutex{},
		//要限流的实例信息
		throttleControlReplicaKeys:          mysql.NewInstanceKeyMap(),
		//配置文件修改互斥锁
		configMutex:                         &sync.Mutex{},
		//配置更新时间互斥锁
		pointOfInterestTimeMutex:            &sync.Mutex{},
		//重命名列表名MAP
		ColumnRenameMap:                     make(map[string]string),
		//panic 退出
		PanicAbort:                          make(chan error),
	}
}

func getSafeTableName(baseName string, suffix string) string {
	name := fmt.Sprintf("_%s_%s", baseName, suffix)
	if len(name) <= mysql.MaxTableNameLength {
		return name
	}
	extraCharacters := len(name) - mysql.MaxTableNameLength
	return fmt.Sprintf("_%s_%s", baseName[0:len(baseName)-extraCharacters], suffix)
}

// GetGhostTableName generates the name of ghost table, based on original table name
// or a given table name
func (this *MigrationContext) GetGhostTableName() string {
	if this.ForceTmpTableName != "" {
		return getSafeTableName(this.ForceTmpTableName, "gho")
	} else {
		return getSafeTableName(this.OriginalTableName, "gho")
	}
}

// GetOldTableName generates the name of the "old" table, into which the original table is renamed.
func (this *MigrationContext) GetOldTableName() string {
	var tableName string
	if this.ForceTmpTableName != "" {
		tableName = this.ForceTmpTableName
	} else {
		tableName = this.OriginalTableName
	}

	if this.TimestampOldTable {
		t := this.StartTime
		timestamp := fmt.Sprintf("%d%02d%02d%02d%02d%02d",
			t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second())
		return getSafeTableName(tableName, fmt.Sprintf("%s_del", timestamp))
	}
	return getSafeTableName(tableName, "del")
}

// GetChangelogTableName generates the name of changelog table, based on original table name
// or a given table name.
func (this *MigrationContext) GetChangelogTableName() string {
	if this.ForceTmpTableName != "" {
		return getSafeTableName(this.ForceTmpTableName, "ghc")
	} else {
		return getSafeTableName(this.OriginalTableName, "ghc")
	}
}

// GetVoluntaryLockName returns a name of a voluntary lock to be used throughout
// the swap-tables process.
func (this *MigrationContext) GetVoluntaryLockName() string {
	return fmt.Sprintf("%s.%s.lock", this.DatabaseName, this.OriginalTableName)
}

// RequiresBinlogFormatChange is `true` when the original binlog format isn't `ROW`
func (this *MigrationContext) RequiresBinlogFormatChange() bool {
	return this.OriginalBinlogFormat != "ROW"
}

// GetApplierHostname is a safe access method to the applier hostname
func (this *MigrationContext) GetApplierHostname() string {
	if this.ApplierConnectionConfig == nil {
		return ""
	}
	if this.ApplierConnectionConfig.ImpliedKey == nil {
		return ""
	}
	return this.ApplierConnectionConfig.ImpliedKey.Hostname
}

// GetInspectorHostname is a safe access method to the inspector hostname
func (this *MigrationContext) GetInspectorHostname() string {
	if this.InspectorConnectionConfig == nil {
		return ""
	}
	if this.InspectorConnectionConfig.ImpliedKey == nil {
		return ""
	}
	return this.InspectorConnectionConfig.ImpliedKey.Hostname
}

// InspectorIsAlsoApplier is `true` when the both inspector and applier are the
// same database instance. This would be true when running directly on master or when
// testing on replica.
func (this *MigrationContext) InspectorIsAlsoApplier() bool {
	return this.InspectorConnectionConfig.Equals(this.ApplierConnectionConfig)
}

// HasMigrationRange tells us whether there's a range to iterate for copying rows.
// It will be `false` if the table is initially empty
func (this *MigrationContext) HasMigrationRange() bool {
	return this.MigrationRangeMinValues != nil && this.MigrationRangeMaxValues != nil
}

func (this *MigrationContext) SetCutOverLockTimeoutSeconds(timeoutSeconds int64) error {
	//如果设置的超时时间小于1秒返回错误信息
	if timeoutSeconds < 1 {
		return fmt.Errorf("Minimal timeout is 1sec. Timeout remains at %d", this.CutOverLockTimeoutSeconds)
	}
	//如果设置的超时时间大于10秒 返回错误信息
	if timeoutSeconds > 10 {
		return fmt.Errorf("Maximal timeout is 10sec. Timeout remains at %d", this.CutOverLockTimeoutSeconds)
	}
	//合法的超时时间范围是[1,10)
	this.CutOverLockTimeoutSeconds = timeoutSeconds
	return nil
}

func (this *MigrationContext) SetExponentialBackoffMaxInterval(intervalSeconds int64) error {
	//间隔小于2 返回报错信息
	if intervalSeconds < 2 {
		return fmt.Errorf("Minimal maximum interval is 2sec. Timeout remains at %d", this.ExponentialBackoffMaxInterval)
	}
	//只要大于等于2就行
	this.ExponentialBackoffMaxInterval = intervalSeconds
	return nil
}

func (this *MigrationContext) SetDefaultNumRetries(retries int64) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	if retries > 0 {
		this.defaultNumRetries = retries
	}
}

func (this *MigrationContext) MaxRetries() int64 {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	retries := this.defaultNumRetries
	return retries
}

func (this *MigrationContext) IsTransactionalTable() bool {
	switch strings.ToLower(this.TableEngine) {
	case "innodb":
		{
			return true
		}
	case "tokudb":
		{
			return true
		}
	}
	return false
}

// ElapsedTime returns time since very beginning of the process
func (this *MigrationContext) ElapsedTime() time.Duration {
	return time.Since(this.StartTime)
}

// MarkRowCopyStartTime
func (this *MigrationContext) MarkRowCopyStartTime() {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.RowCopyStartTime = time.Now()
}

// ElapsedRowCopyTime returns time since starting to copy chunks of rows
func (this *MigrationContext) ElapsedRowCopyTime() time.Duration {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	if this.RowCopyStartTime.IsZero() {
		// Row copy hasn't started yet
		return 0
	}

	if this.RowCopyEndTime.IsZero() {
		return time.Since(this.RowCopyStartTime)
	}
	return this.RowCopyEndTime.Sub(this.RowCopyStartTime)
}

// ElapsedRowCopyTime returns time since starting to copy chunks of rows
func (this *MigrationContext) MarkRowCopyEndTime() {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.RowCopyEndTime = time.Now()
}

func (this *MigrationContext) GetCurrentLagDuration() time.Duration {
	return time.Duration(atomic.LoadInt64(&this.CurrentLag))
}

func (this *MigrationContext) GetProgressPct() float64 {
	return math.Float64frombits(atomic.LoadUint64(&this.currentProgress))
}

func (this *MigrationContext) SetProgressPct(progressPct float64) {
	atomic.StoreUint64(&this.currentProgress, math.Float64bits(progressPct))
}

// math.Float64bits([f=0..100])

// GetTotalRowsCopied returns the accurate number of rows being copied (affected)
// This is not exactly the same as the rows being iterated via chunks, but potentially close enough
func (this *MigrationContext) GetTotalRowsCopied() int64 {
	return atomic.LoadInt64(&this.TotalRowsCopied)
}

func (this *MigrationContext) GetIteration() int64 {
	return atomic.LoadInt64(&this.Iteration)
}

func (this *MigrationContext) MarkPointOfInterest() int64 {
	this.pointOfInterestTimeMutex.Lock()
	defer this.pointOfInterestTimeMutex.Unlock()

	this.pointOfInterestTime = time.Now()
	return atomic.LoadInt64(&this.Iteration)
}

func (this *MigrationContext) TimeSincePointOfInterest() time.Duration {
	this.pointOfInterestTimeMutex.Lock()
	defer this.pointOfInterestTimeMutex.Unlock()

	return time.Since(this.pointOfInterestTime)
}

func (this *MigrationContext) SetHeartbeatIntervalMilliseconds(heartbeatIntervalMilliseconds int64) {
	if heartbeatIntervalMilliseconds < 100 {
		heartbeatIntervalMilliseconds = 100
	}
	if heartbeatIntervalMilliseconds > 1000 {
		heartbeatIntervalMilliseconds = 1000
	}
	this.HeartbeatIntervalMilliseconds = heartbeatIntervalMilliseconds
}

func (this *MigrationContext) SetMaxLagMillisecondsThrottleThreshold(maxLagMillisecondsThrottleThreshold int64) {
	if maxLagMillisecondsThrottleThreshold < 100 {
		maxLagMillisecondsThrottleThreshold = 100
	}
	atomic.StoreInt64(&this.MaxLagMillisecondsThrottleThreshold, maxLagMillisecondsThrottleThreshold)
}

func (this *MigrationContext) SetChunkSize(chunkSize int64) {
	if chunkSize < 100 {
		chunkSize = 100
	}
	if chunkSize > 100000 {
		chunkSize = 100000
	}
	atomic.StoreInt64(&this.ChunkSize, chunkSize)
}

func (this *MigrationContext) SetDMLBatchSize(batchSize int64) {
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > MaxEventsBatchSize {
		batchSize = MaxEventsBatchSize
	}
	atomic.StoreInt64(&this.DMLBatchSize, batchSize)
}

func (this *MigrationContext) SetThrottleGeneralCheckResult(checkResult *ThrottleCheckResult) *ThrottleCheckResult {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.throttleGeneralCheckResult = *checkResult
	return checkResult
}

func (this *MigrationContext) GetThrottleGeneralCheckResult() *ThrottleCheckResult {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	result := this.throttleGeneralCheckResult
	return &result
}

func (this *MigrationContext) SetThrottled(throttle bool, reason string, reasonHint ThrottleReasonHint) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.isThrottled = throttle
	this.throttleReason = reason
	this.throttleReasonHint = reasonHint
}

func (this *MigrationContext) IsThrottled() (bool, string, ThrottleReasonHint) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	// we don't throttle when cutting over. We _do_ throttle:
	// - during copy phase
	// - just before cut-over
	// - in between cut-over retries
	// When cutting over, we need to be aggressive. Cut-over holds table locks.
	// We need to release those asap.
	if atomic.LoadInt64(&this.InCutOverCriticalSectionFlag) > 0 {
		return false, "critical section", NoThrottleReasonHint
	}
	return this.isThrottled, this.throttleReason, this.throttleReasonHint
}

func (this *MigrationContext) GetThrottleQuery() string {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	var query = this.throttleQuery
	return query
}

func (this *MigrationContext) SetThrottleQuery(newQuery string) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	this.throttleQuery = newQuery
}

func (this *MigrationContext) GetThrottleHTTP() string {
	this.throttleHTTPMutex.Lock()
	defer this.throttleHTTPMutex.Unlock()

	var throttleHTTP = this.throttleHTTP
	return throttleHTTP
}

func (this *MigrationContext) SetThrottleHTTP(throttleHTTP string) {
	this.throttleHTTPMutex.Lock()
	defer this.throttleHTTPMutex.Unlock()

	this.throttleHTTP = throttleHTTP
}

func (this *MigrationContext) SetIgnoreHTTPErrors(ignoreHTTPErrors bool) {
	this.throttleHTTPMutex.Lock()
	defer this.throttleHTTPMutex.Unlock()

	this.IgnoreHTTPErrors = ignoreHTTPErrors
}

func (this *MigrationContext) GetMaxLoad() LoadMap {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	return this.maxLoad.Duplicate()
}

func (this *MigrationContext) GetCriticalLoad() LoadMap {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	return this.criticalLoad.Duplicate()
}

func (this *MigrationContext) GetNiceRatio() float64 {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	return this.niceRatio
}

func (this *MigrationContext) SetNiceRatio(newRatio float64) {
	if newRatio < 0.0 {
		newRatio = 0.0
	}
	if newRatio > 100.0 {
		newRatio = 100.0
	}

	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.niceRatio = newRatio
}

func (this *MigrationContext) GetRecentBinlogCoordinates() mysql.BinlogCoordinates {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	return this.recentBinlogCoordinates
}

func (this *MigrationContext) SetRecentBinlogCoordinates(coordinates mysql.BinlogCoordinates) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	this.recentBinlogCoordinates = coordinates
}

// ReadMaxLoad parses the `--max-load` flag, which is in multiple key-value format,
// such as: 'Threads_running=100,Threads_connected=500'
// It only applies changes in case there's no parsing error.
func (this *MigrationContext) ReadMaxLoad(maxLoadList string) error {
	loadMap, err := ParseLoadMap(maxLoadList)
	//解析出错直接返回错误
	if err != nil {
		return err
	}
	//加锁
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	//更新loadMap
	this.maxLoad = loadMap
	return nil
}

// ReadMaxLoad parses the `--max-load` flag, which is in multiple key-value format,
// such as: 'Threads_running=100,Threads_connected=500'
// It only applies changes in case there's no parsing error.
func (this *MigrationContext) ReadCriticalLoad(criticalLoadList string) error {
	//解析
	loadMap, err := ParseLoadMap(criticalLoadList)
	//解析出错直接返回
	if err != nil {
		return err
	}
	//加锁
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	//更新criticalLoad
	this.criticalLoad = loadMap
	return nil
}

func (this *MigrationContext) GetControlReplicasLagResult() mysql.ReplicationLagResult {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	lagResult := this.controlReplicasLagResult
	return lagResult
}

func (this *MigrationContext) SetControlReplicasLagResult(lagResult *mysql.ReplicationLagResult) {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()
	if lagResult == nil {
		this.controlReplicasLagResult = *mysql.NewNoReplicationLagResult()
	} else {
		this.controlReplicasLagResult = *lagResult
	}
}

func (this *MigrationContext) GetThrottleControlReplicaKeys() *mysql.InstanceKeyMap {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	keys := mysql.NewInstanceKeyMap()
	keys.AddKeys(this.throttleControlReplicaKeys.GetInstanceKeys())
	return keys
}

// throttleControlReplicas Example: myhost1.com:3306,myhost2.com,myhost3.com:3307
func (this *MigrationContext) ReadThrottleControlReplicaKeys(throttleControlReplicas string) error {
	keys := mysql.NewInstanceKeyMap()
	if err := keys.ReadCommaDelimitedList(throttleControlReplicas); err != nil {
		return err
	}
	//开启限流互斥锁
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	this.throttleControlReplicaKeys = keys
	return nil
}

func (this *MigrationContext) AddThrottleControlReplicaKey(key mysql.InstanceKey) error {
	this.throttleMutex.Lock()
	defer this.throttleMutex.Unlock()

	this.throttleControlReplicaKeys.AddKey(key)
	return nil
}

// ApplyCredentials sorts out the credentials between the config file and the CLI flags
func (this *MigrationContext) ApplyCredentials() {
	this.configMutex.Lock()
	defer this.configMutex.Unlock()

	if this.config.Client.User != "" {
		this.InspectorConnectionConfig.User = this.config.Client.User
	}
	if this.CliUser != "" {
		// Override
		this.InspectorConnectionConfig.User = this.CliUser
	}
	if this.config.Client.Password != "" {
		this.InspectorConnectionConfig.Password = this.config.Client.Password
	}
	if this.CliPassword != "" {
		// Override
		this.InspectorConnectionConfig.Password = this.CliPassword
	}
}

func (this *MigrationContext) SetupTLS() error {
	//如果使用TLS
	if this.UseTLS {
		return this.InspectorConnectionConfig.UseTLS(this.TLSCACertificate, this.TLSCertificate, this.TLSKey, this.TLSAllowInsecure)
	}
	return nil
}

// ReadConfigFile attempts to read the config file, if it exists
//如果配置文件存在的话读取配置文件
func (this *MigrationContext) ReadConfigFile() error {
	//读取配置文件要加互斥锁
	this.configMutex.Lock()
	defer this.configMutex.Unlock()

	//配置文件为空 直接返回
	if this.ConfigFile == "" {
		return nil
	}
	//解析模式
	gcfg.RelaxedParserMode = true
	gcfgscanner.RelaxedScannerMode = true
	//读取配置文件出错
	if err := gcfg.ReadFileInto(&this.config, this.ConfigFile); err != nil {
		return fmt.Errorf("Error reading config file %s. Details: %s", this.ConfigFile, err.Error())
	}

	// We accept user & password in the form "${SOME_ENV_VARIABLE}" in which case we pull
	// the given variable from os env
	if submatch := envVariableRegexp.FindStringSubmatch(this.config.Client.User); len(submatch) > 1 {
		this.config.Client.User = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(this.config.Client.Password); len(submatch) > 1 {
		this.config.Client.Password = os.Getenv(submatch[1])
	}

	return nil
}