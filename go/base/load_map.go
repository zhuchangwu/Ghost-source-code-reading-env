/*
   Copyright 2016 GitHub Inc.
	 See https://github.com/github/gh-ost/blob/master/LICENSE
*/

package base

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LoadMap is a mapping of status variable & threshold
// e.g. [Threads_connected: 100, Threads_running: 50]
type LoadMap map[string]int64

func NewLoadMap() LoadMap {
	result := make(map[string]int64)
	return result
}

// NewLoadMap parses a `--*-load` flag (e.g. `--max-load`), which is in multiple
// key-value format, such as:
//   'Threads_running=100,Threads_connected=500'
func ParseLoadMap(loadList string) (LoadMap, error) {
	result := NewLoadMap()
	//值为空 直接返回
	if loadList == "" {
		return result, nil
	}
	//按照逗号分割

	loadConditions := strings.Split(loadList, ",")
	for _, loadCondition := range loadConditions {
		//按照等号分割出key 和 value
		loadTokens := strings.Split(loadCondition, "=")
		//参数格式不正确
		if len(loadTokens) != 2 {
			return result, fmt.Errorf("Error parsing load condition: %s", loadCondition)
		}
		//key为空
		if loadTokens[0] == "" {
			return result, fmt.Errorf("Error parsing status variable in load condition: %s", loadCondition)
		}
		//把value转为int
		if n, err := strconv.ParseInt(loadTokens[1], 10, 0); err != nil {
			return result, fmt.Errorf("Error parsing numeric value in load condition: %s", loadCondition)
		} else {
			result[loadTokens[0]] = n
		}
	}
	//返回构建好的LoadMap
	return result, nil
}

// Duplicate creates a clone of this map
func (this *LoadMap) Duplicate() LoadMap {
	dup := make(map[string]int64)
	for k, v := range *this {
		dup[k] = v
	}
	return dup
}

// String() returns a string representation of this map
func (this *LoadMap) String() string {
	tokens := []string{}
	for key, val := range *this {
		token := fmt.Sprintf("%s=%d", key, val)
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return strings.Join(tokens, ",")
}
