package server

import "github.com/gorilla/websocket"

// 一个设备连接进来就实例化一个client
type Client struct {
	DeviceId   string
	DeviceType string // 设备类型 1:winapp,2:deviceapp，3:phoneapp
	Key        string // 设备key 作为设备的唯一标识
	Mac        string
	IP         string
	Socket     *websocket.Conn
	Read       chan []byte
	Write      chan []byte
	Tm         int64 //最后一次通话的时间 毫秒
}


// 从连接中读取数据
func (c *Client) read() {
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
		c.Read <- message
	}
}

// 向设备写入数据
func (c *Client) write() {
	defer func() {
		c.Socket.Close()
	}()

	for {
		select {
		case message, ok := <-c.Write:
			if !ok {
				c.Socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.Socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}
