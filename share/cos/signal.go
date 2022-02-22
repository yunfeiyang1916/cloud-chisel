package cos

import (
	"github.com/jpillora/sizestr"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// GoStats 在SIGUSR2上打印统计信息到标准输出(仅posix)
func GoStats() {
	//silence complaints from windows
	const SIGUSR2 = syscall.Signal(0x1f)
	time.Sleep(time.Second)
	c := make(chan os.Signal, 1)
	signal.Notify(c, SIGUSR2)
	for range c {
		memStats := runtime.MemStats{}
		runtime.ReadMemStats(&memStats)
		log.Printf("recieved SIGUSR2, go-routines: %d, go-memory-usage: %s",
			runtime.NumGoroutine(),
			sizestr.ToString(int64(memStats.Alloc)))
	}
}

//AfterSignal 返回一个通道，该通道将在给定的持续时间后关闭，或者直到接收到SIGHUP
func AfterSignal(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		select {
		case <-time.After(d):
		case <-sig:
		}
		signal.Stop(sig)
		close(ch)
	}()
	return ch
}
