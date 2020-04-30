package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"sanji_s12/models"
)

// 连接管理器
// 这个管理器，需要处理各种指令
type ClientManager struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client // 设备连接
	Unregister chan *Client // 设备下线
}

// 设备连接
type Client struct {
	DeviceId   string
	DeviceType string // 设备类型 1:winapp,2:deviceapp，3:phoneapp
	Key        string // 设备key 作为设备的唯一标识
	Mac        string
	IP         string
	Socket     *websocket.Conn
	Send       chan []byte
	Tm         int64 //最后一次通话的时间 毫秒
}

// 连接情况
type Connection struct {
	ReadClient     Client
	WriteClient    Client
	DisconnectChan chan struct{}
}

type Message struct {
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content,omitempty"`
}

// ClientManager实例化，全局变量
// 考虑将ClientManager移到公共包中
var Manager = ClientManager{
	Broadcast:  make(chan []byte),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
	Clients:    make(map[*Client]bool),
}

// 开始处理连接
func (manager *ClientManager) Start() {
	for {
		select {
		case conn := <-manager.Register:
			manager.Clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "/A new socket has connected."})
			manager.send(jsonMessage, conn)
		case conn := <-manager.Unregister:
			if _, ok := manager.Clients[conn]; ok {
				close(conn.Send)
				delete(manager.Clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket has disconnected."})
				manager.send(jsonMessage, conn)
			}
		case message := <-manager.Broadcast:
			for conn := range manager.Clients {
				select {
				case conn.Send <- message:
				default:
					close(conn.Send)
					delete(manager.Clients, conn)
				}
			}
		}
	}
}

var DisconnectChan chan struct{}

// 为两个设备建立连接
// 在连接列表中找出两个key, 从fromKey中读取数据，然后写到toKey中
// 这个连接可能需要封装一下
func (manager *ClientManager) Connect(fromKey string, toKey string) {

	var ReadClient, WriteClient Client

	for client, _ := range manager.Clients {
		if client.Key == fromKey {
			ReadClient = *client
		}
		if client.Key == toKey {
			WriteClient = *client
		}
	}

	// 先处理最简单的情况
	DisconnectChan := make(chan struct{}, 1)
	for {
		select {
		// 如果从breakChan中读出了数据，说明收到了Disconnect指令，就中断该管道
		case <-DisconnectChan:
			break
		default:
			data := ReadClient.read()
			WriteClient.Send <- data
		}

	}

}

// 断开连接
func (manager *ClientManager) Disconnect(fromKey, toKey string) {

}

// 重置连接
func (manager *ClientManager) Reset(data string) {

}

// 关闭连接
func (manager *ClientManager) Close(data string) {

}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.Clients {
		if conn != ignore {
			conn.Send <- message
		}
	}
}

// 从连接中读取数据
func (c *Client) read() []byte {
	defer func() {
		Manager.Unregister <- c
		c.Socket.Close()
	}()

	for {
		_, message, err := c.Socket.ReadMessage()
		if err != nil {
			Manager.Unregister <- c
			c.Socket.Close()
			break
		}
		return message
	}
	return nil
}

// 向设备写入数据
func (c *Client) write() {
	defer func() {
		c.Socket.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.Socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}

// 处理ws连接
func WSServer(res http.ResponseWriter, req *http.Request) {

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
		reqType = queryForm["key"][0]
	} else {
		//dataJson := ResultErr(501, "field device_type is null ")
		_ = conn.WriteMessage(websocket.TextMessage, []byte("field key is null"))
		closeErr := conn.Close()
		if closeErr != nil {
			fmt.Println(closeErr)
		}
		return
	}

	// 如果key在permissionKey中，刚让设备进行连接
	exist = false
	for _, pKey := range models.PermissionKey {
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

	client := &Client{
		DeviceType: reqType,
		Key:        key,
		Mac:        mac,
		Socket:     conn,
		IP:         ip,
		Send:       make(chan []byte),
	}

	Manager.Register <- client

	go client.read()
	go client.write()
}
