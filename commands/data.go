package commands

// 全局变量
var S12Key string
var PermissionKey []string

var InCmdChan = make(chan Cmd, 100)  // 放置指令的管道
var OutCmdChan = make(chan Cmd, 100) // 读取指令的管道



// 首先确定数据结构
// websocket 发送的指令数据格式
type Cmd struct {
	CmdId       int64  `json:"cmdid"`
	Cmd         string `json:"cmd"`
	Role        string `json:"role"`    //client 表示作为客户端，server 表示作为服务端
	Forward     string `json:"forward"` // 值为空表示是给自己的指令，否则是目标的唯一id，S1需要对其转发
	Url         string `json:"url"`
	From        string `json:"from"` // 从 from 过来的数据
	To          string `json:"to"`   // 发给 To
	Rtmp        string `json:"rtmp"`
	File        string `json:"file"` //如果有File值就要保存文件，此指令不中断之前的操作。
	Data        string `json:"data"` //
	FromConnKey string               // 发送方的连接，这个字段自用
}

type ReportData struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// 设备上线，下线的数据格式
type ConnectStatus struct {
	ID        int64  `json:"id"` // 设备ID
	Key       string `json:"key"`
	IP        string `json:"ip"`
	Duration  int64  `json:"duration"`   // 连接时长
	LastAlive int64  `json:"last_alive"` //最后活跃时间
	Status    string `json:"status"`     // up down
}

// 连接情况
type AllConnections struct {
	ConnectionKeys []string `json:"connection_keys"`
	Total          int64    `json:"total"`  // 连接总数
	Active         int64    `json:"active"` // 正在接发数据的连接

	// 所有的路由信息
}

// 服务器状态
type ServerStatus struct {
	CpuInfo  int64 `json:"cpu_info"`
	DiskInfo int64 `json:"disk_info"`
}

