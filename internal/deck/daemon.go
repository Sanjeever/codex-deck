package deck

import (
	"net"
	"path/filepath"
	"time"
)

const (
	controlSocketDir   = "app-server-control"
	controlSocketFile  = "app-server-control.sock"
	daemonProbeTimeout = 200 * time.Millisecond
)

// DaemonRunning 报告 CLI 共享后台服务是否在监听控制 socket。
// 只做连接探测，不发送请求，也不启动、停止或重启该进程。
func (c CLI) DaemonRunning() bool {
	conn, err := net.DialTimeout("unix", filepath.Join(c.Home, controlSocketDir, controlSocketFile), daemonProbeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
