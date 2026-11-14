package deck

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIDaemonRunning(t *testing.T) {
	home := t.TempDir()
	c := CLI{Home: home}
	if c.DaemonRunning() {
		t.Fatal("控制 socket 不存在时误报后台服务运行")
	}
	dir := filepath.Join(home, controlSocketDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, controlSocketFile))
	if err != nil {
		t.Skipf("当前平台无法创建 Unix socket: %v", err)
	}
	if !c.DaemonRunning() {
		t.Fatal("控制 socket 监听时未识别后台服务")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if c.DaemonRunning() {
		t.Fatal("控制 socket 关闭后仍报告后台服务运行")
	}
}
