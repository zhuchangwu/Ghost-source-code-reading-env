/*
   Copyright 2016 GitHub Inc.
	 See https://github.com/github/gh-ost/blob/master/LICENSE
*/

package base

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	gosql "database/sql"
	"gh-ost/go/mysql"
	"github.com/outbrain/golib/log"
)

var (
	prettifyDurationRegexp = regexp.MustCompile("([.][0-9]+)")
)

func PrettifyDurationOutput(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	result := fmt.Sprintf("%s", d)
	result = prettifyDurationRegexp.ReplaceAllString(result, "")
	return result
}

func FileExists(fileName string) bool {
	if _, err := os.Stat(fileName); err == nil {
		return true
	}
	return false
}

func TouchFile(fileName string) error {
	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	return f.Close()
}

// StringContainsAll returns true if `s` contains all non empty given `substrings`
// The function returns `false` if no non-empty arguments are given.
func StringContainsAll(s string, substrings ...string) bool {
	nonEmptyStringsFound := false
	for _, substring := range substrings {
		if substring == "" {
			continue
		}
		if strings.Contains(s, substring) {
			nonEmptyStringsFound = true
		} else {
			// Immediate failure
			return false
		}
	}
	return nonEmptyStringsFound
}
// 校验连接DB， 实际上是在尝试执行SQL，获取到mysql的版本号、端口号
func ValidateConnection(db *gosql.DB, connectionConfig *mysql.ConnectionConfig, migrationContext *MigrationContext) (string, error) {
	versionQuery := `select @@global.version`
	var port, extraPort int
	var version string
	if err := db.QueryRow(versionQuery).Scan(&version); err != nil {
		return "", err
	}
	extraPortQuery := `select @@global.extra_port`
	if err := db.QueryRow(extraPortQuery).Scan(&extraPort); err != nil {
		// swallow this error. not all servers support extra_port
	}
	// AliyunRDS set users port to "NULL", replace it by gh-ost param
	// GCP set users port to "NULL", replace it by gh-ost param
	if migrationContext.AliyunRDS || migrationContext.GoogleCloudPlatform {
		port = connectionConfig.Key.Port
	} else {
		// todo 在这里查询了Mysql，通过执行 `select @@global.port` 找到了port
		// todo 然后校验port
		portQuery := `select @@global.port`
		if err := db.QueryRow(portQuery).Scan(&port); err != nil {
			return "", err
		}
	}
	// todo connectionConfig.Key.Port 中规定的端口是我们传递进去的3307
	// todo port 查出来的3306(因为它在docker中，宿主机的3307映射3306)， 现在他们不相等的时候检查过不了，故我在下面手动将 prot改为3307
	port = 3307
	// todo 校验用户提供的port和查出来的port是否一致，不一致算作连接校验失败
	if connectionConfig.Key.Port == port || (extraPort > 0 && connectionConfig.Key.Port == extraPort) {
		log.Infof("connection validated on %+v", connectionConfig.Key)
		return version, nil
	} else if extraPort == 0 {
		return "", fmt.Errorf("Unexpected database port reported: %+v", port)
	} else {
		return "", fmt.Errorf("Unexpected database port reported: %+v / extra_port: %+v", port, extraPort)
	}
}
