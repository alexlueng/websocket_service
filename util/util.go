package util

import (
	"fmt"
	"reflect"
	"sync/atomic"
)

func SmartPrint(i interface{}){
	var kv = make(map[string]interface{})
	vValue := reflect.ValueOf(i)
	vType :=reflect.TypeOf(i)
	for i:=0; i < vValue.NumField(); i++{
		kv[vType.Field(i).Name] = vValue.Field(i)
	}
	fmt.Println("获取到数据:")
	for k,v := range kv {
		fmt.Print(k)
		fmt.Print(":")
		fmt.Print(v)
		fmt.Println()
	}
}

// cmdId 最大值，超出最大值则重置 1,000,000,000
var maxCmdId int64 = 10000000000

// cmdId 初始值为 1
var cmdId int64 = 1

func GetCmdId() int64 {
	var newCmdId int64
	// 把cmdId与cmdId最大值比较
	if atomic.LoadInt64(&cmdId) >= maxCmdId {
		// 超出或大于最大cmdId，重置cmdid
		atomic.CompareAndSwapInt64(&cmdId, cmdId, 1)
		newCmdId = 1
	} else {
		// cmdId 自增
		newCmdId = atomic.AddInt64(&cmdId, 1)
	}
	return newCmdId
}
