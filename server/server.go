package server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"sanji_s12/commands"
)

// ClientManager实例化，全局变量
// 考虑将ClientManager移到公共包中
var Manager = ClientManager{
	Broadcast:  make(chan []byte),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
	Clients:    make(map[*Client]bool),
}

// client连接的url格式：ws://192.168.1.186:9911/?devicetype=papp&key=faghjag
// 处理ws连接
func WSServer(res http.ResponseWriter, req *http.Request) {

	fmt.Println("execute here")

	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(res, req, nil)
	if err != nil {
		http.NotFound(res, req)
		return
	}

	queryForm, err := url.ParseQuery(req.URL.RawQuery)
	var (
		reqType string
		mac     string
		key     string
		ip      string
		exist   bool
	)

	//提取 url中 device_type
	if (len(queryForm["devicetype"]) != 0) && (queryForm["devicetype"][0] != "") {
		reqType = queryForm["devicetype"][0]
	} else {
		//dataJson := ResultErr(501, "field device_type is null ")
		_ = conn.WriteMessage(websocket.TextMessage, []byte("field device_type is null"))
		closeErr := conn.Close()
		if closeErr != nil {
			fmt.Println(closeErr)
		}
		return
	}

	//提取 url中 key
	if (len(queryForm["key"]) != 0) && (queryForm["key"][0] != "") {
		key = queryForm["key"][0]
	} else {
		//dataJson := ResultErr(501, "field device_type is null ")
		_ = conn.WriteMessage(websocket.TextMessage, []byte("field key is null"))
		closeErr := conn.Close()
		if closeErr != nil {
			fmt.Println(closeErr)
		}
		return
	}

	for _, key := range commands.PermissionKey {
		fmt.Println("permit key: ", key)
	}

	fmt.Println("url key: ", key)

	// 如果key在permissionKey中，则让设备进行连接
	exist = false
	for _, pKey := range commands.PermissionKey {
		if key == pKey {
			exist = true
		}
	}

	// 在permissionKey中没有找到
	if !exist {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Permission deny"))
		closeErr := conn.Close()
		if closeErr != nil {
			fmt.Println(closeErr)
		}
		return
	}

	// 实例化这个设备
	client := &Client{
		DeviceType:    reqType,
		Key:           key,
		Mac:           mac,
		Socket:        conn,
		IP:            ip,
		Read:          make(chan []byte),
		Write:         make(chan []byte),
		CloseChan:     make(chan struct{}, 1),
		BroadcastRecv: false,
	}

	//util.SmartPrint(client)

	Manager.Register <- client
	//
	//for c, _ := range Manager.Clients {
	//	fmt.Println(c)
	//}

	// 开启协程 对该设备进行收发数据
	go client.read()
	go client.write()
}
