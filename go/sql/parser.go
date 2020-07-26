/*
   Copyright 2016 GitHub Inc.
	 See https://github.com/github/gh-ost/blob/master/LICENSE
*/

package sql

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	//去除空格的正则表达式
	sanitizeQuotesRegexp = regexp.MustCompile("('[^']*')")
	//识别rename 列 类语句的正则表达式
	renameColumnRegexp   = regexp.MustCompile(`(?i)\bchange\s+(column\s+|)([\S]+)\s+([\S]+)\s+`)
	//识别drop类语句的正则表达式
	dropColumnRegexp     = regexp.MustCompile(`(?i)\bdrop\s+(column\s+|)([\S]+)$`)
	//识别rename 表 类语句的正则表达式
	renameTableRegexp    = regexp.MustCompile(`(?i)\brename\s+(to|as)\s+`)
)

type Parser struct {
	columnRenameMap map[string]string
	droppedColumns  map[string]bool
	isRenameTable   bool
}

func NewParser() *Parser {
	return &Parser{
		columnRenameMap: make(map[string]string),
		droppedColumns:  make(map[string]bool),
	}
}
//标记化所有的变更sql
//拆分出一个个完整的sql
func (this *Parser) tokenizeAlterStatement(alterStatement string) (tokens []string, err error) {
	//结束符号
	terminatingQuote := rune(0)
	f := func(c rune) bool {
		switch {
		case c == terminatingQuote:
			terminatingQuote = rune(0)
			return false
		case terminatingQuote != rune(0):
			return false
		case c == '\'':
			terminatingQuote = c
			return false
		case c == '(':
			terminatingQuote = ')'
			return false
		default:
			return c == ','
		}
	}

	tokens = strings.FieldsFunc(alterStatement, f)
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}
	return tokens, nil
}
//去掉单引号中的内容(单引号保留)
func (this *Parser) sanitizeQuotesFromAlterStatement(alterStatement string) (strippedStatement string) {
	strippedStatement = alterStatement
	strippedStatement = sanitizeQuotesRegexp.ReplaceAllString(strippedStatement, "''")
	return strippedStatement
}

//解析所有的标记
func (this *Parser) parseAlterToken(alterToken string) (err error) {
	{
		// rename
		//获取到所有rename 列类SQL语句
		allStringSubmatch := renameColumnRegexp.FindAllStringSubmatch(alterToken, -1)
		for _, submatch := range allStringSubmatch {
			//如果有引号返回引号中的内容
			//原始表名
			if unquoted, err := strconv.Unquote(submatch[2]); err == nil {
				submatch[2] = unquoted
			}
			//目标表名
			if unquoted, err := strconv.Unquote(submatch[3]); err == nil {
				submatch[3] = unquoted
			}
			//原始表名和目标表名构造map
			this.columnRenameMap[submatch[2]] = submatch[3]
		}
	}
	{
		// drop
		allStringSubmatch := dropColumnRegexp.FindAllStringSubmatch(alterToken, -1)
		for _, submatch := range allStringSubmatch {
			if unquoted, err := strconv.Unquote(submatch[2]); err == nil {
				submatch[2] = unquoted
			}
			this.droppedColumns[submatch[2]] = true
		}
	}
	{
		// rename table
		if renameTableRegexp.MatchString(alterToken) {
			this.isRenameTable = true
		}
	}
	return nil
}

//解析变更sql
func (this *Parser) ParseAlterStatement(alterStatement string) (err error) {
	alterTokens, _ := this.tokenizeAlterStatement(alterStatement)
	for _, alterToken := range alterTokens {
		//去掉单引号中的内容(单引号保留)
		alterToken = this.sanitizeQuotesFromAlterStatement(alterToken)
		//最终是获取到要将原始表名命名为新表名的一个map
		this.parseAlterToken(alterToken)
	}
	return nil
}

//把列A重命名为B
//获取这个['A']='B'的MAP
func (this *Parser) GetNonTrivialRenames() map[string]string {
	result := make(map[string]string)
	for column, renamed := range this.columnRenameMap {
		if column != renamed {
			result[column] = renamed
		}
	}
	return result
}
//是否包含重命名列名
func (this *Parser) HasNonTrivialRenames() bool {
	return len(this.GetNonTrivialRenames()) > 0
}
//已经重命名完成的列名
func (this *Parser) DroppedColumnsMap() map[string]bool {
	return this.droppedColumns
}

func (this *Parser) IsRenameTable() bool {
	return this.isRenameTable
}
