package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sanji_s12/client"
	"sanji_s12/server"
	"syscall"
)

// S12 转发服务器
// 主要功能：
// 1 上线的时候，马上连接s10, 并上报当前的连接信息(report指令）
// 2 当有新的连接连到s12的时候，向s10上报
// 这是作为客户端的功能

// 作为服务端的功能：
// 对请求过来的连接进行管理
// 心跳检测功能，当s12与s10断开连接的时候能重新连接
// 转发数据功能
//

func main() {

	//commands.PermissionKey = append(commands.PermissionKey, "testkey1")
	//commands.PermissionKey = append(commands.PermissionKey, "testkey2")

	// 开启s12客户端，连接s10
	go client.WSClient()

	// 开启s12服务端，监听设备的连接,处理指令
	go server.Manager.Start()
	go server.Manager.HandleCommand()
	go server.Manager.Report()

	http.HandleFunc("/ws", server.WSServer)
	if err := http.ListenAndServe(":9911", nil); err != nil {
		fmt.Println("http server error: ", err.Error())
	}

	// 在主线程中阻塞，防止程序退出
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGKILL)
	//阻塞直到有信号传入
	fmt.Println("启动")
	//阻塞直至有信号传入
	s := <-c
	fmt.Println("退出信号", s)
}
