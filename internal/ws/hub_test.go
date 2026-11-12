package ws

import (
	"sync"
	"testing"
)

// TestHubConcurrent 并发注册/推送/注销，验证 Hub 的并发安全。
// 运行方式：go test ./internal/ws/ -race -v
func TestHubConcurrentRegisterSendUnregister(t *testing.T) {
	h := NewHub()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 10 个用户，每人多个连接（模拟多端登录）
			c := &Client{UserID: uint64(i % 10), send: make(chan []byte, 8)}
			h.Register(c)
			h.SendToUser(c.UserID, []byte("hi"))
			h.Unregister(c)
		}(i)
	}
	wg.Wait()

	if got := h.OnlineCount(); got != 0 {
		t.Fatalf("期望 0 个在线连接，实际 %d", got)
	}
}

// TestHubMultiDevice 验证一个用户多端在线时都能收到消息。
func TestHubMultiDevice(t *testing.T) {
	h := NewHub()
	c1 := &Client{UserID: 1, send: make(chan []byte, 1)}
	c2 := &Client{UserID: 1, send: make(chan []byte, 1)}
	h.Register(c1)
	h.Register(c2)

	if !h.IsOnline(1) {
		t.Fatal("用户应在线")
	}
	h.SendToUser(1, []byte("msg"))

	select {
	case <-c1.send:
	default:
		t.Fatal("连接 1 应收到消息")
	}
	select {
	case <-c2.send:
	default:
		t.Fatal("连接 2 应收到消息")
	}
}

// TestHubCloseAll 验证进程退出时能清空所有连接。
func TestHubCloseAll(t *testing.T) {
	h := NewHub()
	c := &Client{UserID: 1, send: make(chan []byte, 1)}
	h.Register(c)

	h.CloseAll()

	if got := h.OnlineCount(); got != 0 {
		t.Fatalf("CloseAll 后应为 0，实际 %d", got)
	}
}

// TestEnqueueNonBlocking 验证慢消费者不会阻塞推送方。
func TestEnqueueNonBlocking(t *testing.T) {
	c := &Client{UserID: 1, send: make(chan []byte, 1)}
	c.Enqueue([]byte("first")) // 填满
	done := make(chan struct{})
	go func() {
		c.Enqueue([]byte("second")) // 队列满 → 应该立即返回（内部会 Close）
		close(done)
	}()
	<-done // 如果 Enqueue 阻塞，这里会死锁
}
