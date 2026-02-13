# SAC Output SSE — 架构演进 Roadmap

## 目录

1. [当前方案：Subscriber Channel 模型](#1-当前方案subscriber-channel-模型)
2. [未来方案：Ring Buffer 广播模型](#2-未来方案ring-buffer-广播模型)
3. [Ring Buffer 核心概念详解](#3-ring-buffer-核心概念详解)
4. [两种方案对比](#4-两种方案对比)
5. [迁移时机与条件](#5-迁移时机与条件)

---

## 1. 当前方案：Subscriber Channel 模型

### 架构图

```
Redis Pub/Sub                    OutputHub                         SSE Clients
─────────────                    ─────────                         ───────────
                              ┌─────────────────┐
  sac:output:1:5 ──publish──▶ │  Start() loop   │
                              │                 │    ┌──── sub_A (chan 16) ──▶ Browser Tab 1
                              │  for sub in     │────┤
                              │    h.subs:      │    └──── sub_B (chan 16) ──▶ Browser Tab 2
                              │    sub.ch <- ev │
                              └─────────────────┘
```

### 工作原理

```go
// 每个 SSE 连接 = 一个 subscriber，持有独立的 buffered channel
type subscriber struct {
    ch  chan OutputEvent  // 容量 16，独立缓冲
    key string           // "userID:agentID"
}

// Redis 收到事件后，遍历所有 subscriber，逐个发送
for sub := range h.subs {
    if sub.key == key {
        select {
        case sub.ch <- event:  // 发送成功
        default:               // channel 满了，丢弃（慢读者保护）
        }
    }
}
```

### 优点
- 实现极简，容易理解和调试
- 每个连接独立，互不干扰
- 连接断开直接 delete，无泄漏风险

### 局限
- 每个连接独立拷贝事件数据，N 个连接 = N 份内存
- 广播时需要遍历所有 subscriber 逐个发送
- channel 满时直接丢弃，无法回溯历史事件

### 适用场景
- 单用户单 agent 单连接（当前 SAC 的使用模式）
- 连接数少（< 100）

---

## 2. 未来方案：Ring Buffer 广播模型

### 架构图

```
Redis Pub/Sub                    OutputHub                              SSE Clients
─────────────                    ─────────                              ───────────

                              ┌──────────────────────┐
  sac:output:1:5 ──publish──▶ │  agentSlot "1:5"     │
                              │  ┌──────────────────┐ │
                              │  │ Ring Buffer [64] │ │    Cursor A (pos=42) ──▶ Tab 1
                              │  │                  │ │    Cursor B (pos=40) ──▶ Tab 2
                              │  │ [38][39][40][41] │ │    Cursor C (pos=41) ──▶ Tab 3
                              │  │  ▲         ▲     │ │
                              │  │  │oldest   │head │ │    所有 cursor 共享同一份数据
                              │  └──────────────────┘ │    只是读取位置不同
                              │  notify: chan struct{} │
                              └──────────────────────┘
                                        │
                                   close(notify)  ◀── 写入时关闭旧 channel
                                        │              创建新 channel
                                        ▼              所有阻塞的 cursor 同时唤醒
                                  所有 cursor 唤醒
```

### 三个核心组件

#### 2.1 agentSlot — 共享的环形缓冲区

```go
type agentSlot struct {
    mu     sync.Mutex
    ring   [64]OutputEvent   // 固定大小数组，不是 slice，零 GC
    head   uint64            // 单调递增的写入位置
    notify chan struct{}      // 广播信号
    refs   int32             // 活跃 cursor 数量
}
```

为什么用固定数组而不是 slice？
- `[64]OutputEvent` 是值类型，内联在 struct 里，一次分配
- `[]OutputEvent` 是引用类型，需要额外的堆分配 + GC 追踪
- 对于已知上限的缓冲区，固定数组更高效

#### 2.2 Cursor — 轻量级读取游标

```go
type Cursor struct {
    slot *agentSlot   // 指向共享的 slot
    pos  uint64       // 下一个要读的位置
}
```

一个 Cursor 只有 16 字节（一个指针 + 一个 uint64）。
对比 channel 方案：每个 channel 需要 ~96 字节 + buffer 内存。

#### 2.3 notify channel — 零分配广播机制

这是整个设计最精妙的部分：

```go
// 写入时
func (s *agentSlot) push(e OutputEvent) {
    s.mu.Lock()
    s.ring[s.head % 64] = e   // 写入 ring
    s.head++                   // 推进写指针
    ch := s.notify             // 拿到当前的 notify channel
    s.notify = make(chan struct{})  // 替换为新的
    s.mu.Unlock()
    close(ch)                  // 关闭旧的 → 所有等待者同时唤醒
}

// 读取时
func (c *Cursor) Next(ctx context.Context) (OutputEvent, bool) {
    for {
        c.slot.mu.Lock()
        if c.pos < c.slot.head {
            // 有新数据，直接读
            e := c.slot.ring[c.pos % 64]
            c.pos++
            c.slot.mu.Unlock()
            return e, true
        }
        // 没有新数据，拿到 notify channel 准备等待
        notify := c.slot.notify
        c.slot.mu.Unlock()

        select {
        case <-ctx.Done():
            return OutputEvent{}, false  // 连接断开
        case <-notify:
            // 被唤醒，回到 for 循环读数据
        }
    }
}
```

**为什么 `close(chan)` 是零分配广播？**

Go 的 channel 有一个特性：`close(ch)` 会让所有阻塞在 `<-ch` 上的 goroutine 同时返回。
这意味着：
- 1 个 writer + 1000 个 reader → 一次 `close()` 唤醒全部
- 不需要遍历 reader 列表
- 不需要为每个 reader 分配独立的通知 channel
- 时间复杂度 O(1)（从 writer 视角），Go runtime 内部唤醒是 O(N) 但极快

对比其他广播方式：
| 方式 | 写入复杂度 | 额外分配 |
|------|-----------|---------|
| 遍历 channel 发送 | O(N) | 每个 reader 一个 channel |
| sync.Cond.Broadcast | O(N) | 无，但需要持锁 |
| close(chan) + replace | O(1)* | 每次写入一个新 chan（极小） |

---

## 3. Ring Buffer 核心概念详解

### 3.1 取模寻址 — 为什么 size 必须是 2 的幂

```
ring size = 64 (二进制: 1000000)

head = 67 时，实际写入位置:
  67 % 64 = 3     ← 取模运算
  67 & 63 = 3     ← 位与运算（等价，但更快）

因为 64 = 2^6，所以 64-1 = 63 = 0b00111111
任何数 & 63 等价于 % 64，但 CPU 执行位与只需 1 个时钟周期
```

这就是为什么 ring buffer 的大小通常选 2 的幂：编译器会自动把 `%` 优化成 `&`。

### 3.2 慢读者追赶

```
Ring Buffer (size=8):
写入位置 head = 15

  [8] [9] [10] [11] [12] [13] [14] [ ]
   ▲                              ▲
   │oldest available              │head (next write)

Cursor A: pos = 13  → 正常，读 [13][14] 即可
Cursor B: pos = 5   → 落后太多！pos 5 的数据已被覆盖

处理方式：
  if head - pos > ringSize {
      pos = head - ringSize  // 跳到最旧的可用位置
  }
  // Cursor B: pos 从 5 跳到 7 (= 15 - 8)
  // 丢失了 [5][6] 但不会读到脏数据
```

### 3.3 生命周期管理 — Grace Period

这是我们回退到简单方案的原因。原始实现的问题：

```
时间线：
  T0: 用户刷新页面
  T1: 旧 SSE 断开 → unsub() → refs 降到 0 → slot 被删除
  T2: Redis 推送事件 → slot == nil → 事件丢失！
  T3: 新 SSE 连接 → 创建全新 slot (head=0) → 错过了 T2 的事件
```

正确的做法是加 grace period：

```go
// unsub 时不立即删除，而是启动一个定时器
return cursor, func() {
    if atomic.AddInt32(&slot.refs, -1) == 0 {
        time.AfterFunc(30*time.Second, func() {
            h.mu.Lock()
            if atomic.LoadInt32(&slot.refs) == 0 {
                delete(h.slots, key)  // 30秒内没人重连，才真正删除
            }
            h.mu.Unlock()
        })
    }
}
```

这样刷新页面时：
```
T0: 刷新 → 旧连接断开 → refs=0 → 启动 30s 定时器（slot 保留）
T1: Redis 事件到达 → slot 存在 → 事件写入 ring buffer ✓
T2: 新连接建立 → 复用同一个 slot → refs=1 → 取消清理
T3: 新 cursor 从当前 head 开始 → 不会读到旧事件，但也不会丢失 T1 之后的事件
    前端 onReconnect 回调会触发 loadRootFiles() 补偿断连期间的变更 ✓
```

---

## 4. 两种方案对比

| 维度 | Channel 模型 (当前) | Ring Buffer 模型 (未来) |
|------|---------------------|------------------------|
| 代码量 | ~60 行 | ~120 行 |
| 每连接内存 | ~96B + 16×event | 16B (一个 Cursor) |
| 事件存储 | N 份拷贝 (N=连接数) | 1 份共享 |
| 广播方式 | 遍历发送 O(N) | close(chan) O(1) |
| 历史回溯 | 不支持 | 支持 (ring 内) |
| 慢读者 | 丢弃 | 自动追赶到最旧可用 |
| 实现复杂度 | 低 | 中 |
| 适合规模 | < 100 连接/agent | 1000+ 连接/agent |

### 什么时候该切换？

当以下任一条件成立时：
- 同一个 agent 经常有 5+ 个并发 SSE 连接（多标签页、多设备）
- 需要支持"重连后补发最近 N 条事件"的能力
- 内存 profiling 显示 subscriber channel 占用显著

---

## 5. 迁移时机与条件

### Phase 1 — 当前 (v0.0.27)
- ✅ 简单 channel 模型
- ✅ 单用户单连接场景
- ✅ 前端 onReconnect 补偿机制

### Phase 2 — 未来
- 引入 ring buffer + grace period
- 支持多标签页共享事件流
- 可选：cursor 从 `head - N` 开始，重连时自动补发最近事件
- 可选：slot LRU 淘汰，避免内存无限增长
