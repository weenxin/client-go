# Cond

条件变量使用举例

1. Type.Get //调用接口
2. q.cond.L.Lock //抢锁
3. for len(q.queue) == 0 && !q.shuttingDown {q.cond.Wait() } //获取到锁，并且队列长度不为0
    -> t := runtime_notifyListAdd(&c.notify) // 将goroutine增加到对象的等待队列中
    -> c.L.Unlock() //释放锁，让可以下一次抢锁
    -> runtime_notifyListWait(&c.notify, t)，// 等待资源，等待通知，降低资源使用；
    -> c.L.Lock() //继续抢锁

