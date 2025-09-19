# Go协程池 (Goroutine Pool)

一个高性能、功能丰富的Go协程池实现，提供了基础协程池和高级任务管理器两种使用方式。

## 特性

- 🚀 **高性能**: 预创建固定数量的工作协程，避免频繁创建销毁协程的开销
- 🎯 **灵活分配**: 支持随机分配和指定工作协程两种任务分配方式
- 🛡️ **安全可靠**: 内置panic恢复机制，单个任务异常不会影响整个协程池
- 📊 **统计监控**: 提供详细的运行统计信息和状态监控
- ⏰ **超时控制**: 支持优雅关闭和超时强制关闭
- 🔄 **任务管理**: 高级JobManager支持类型化任务处理、暂停恢复等功能
- 📝 **完整日志**: 核心流程包含详细的中文日志输出

## 安装

```bash
go get github.com/mpool/goroutine-pool
```

## 快速开始

### 基础协程池使用

```go
package main

import (
    "fmt"
    "sync"
    "time"
    
    "github.com/mpool/goroutine-pool"
)

func main() {
    // 创建协程池：4个工作协程，每个队列长度10
    pool := mpool.NewPool(4, 10)
    defer pool.Release()
    
    var wg sync.WaitGroup
    
    // 提交任务
    for i := 0; i < 10; i++ {
        wg.Add(1)
        taskID := i
        
        err := pool.AddJob(mpool.Job{
            WorkerID: -1, // -1表示随机分配
            Handle: func() error {
                defer wg.Done()
                fmt.Printf("处理任务 %d\n", taskID)
                time.Sleep(100 * time.Millisecond)
                return nil
            },
        })
        
        if err != nil {
            fmt.Printf("添加任务失败: %v\n", err)
            wg.Done()
        }
    }
    
    wg.Wait()
    fmt.Println("所有任务完成")
}
```

### 高级任务管理器使用

```go
package main

import (
    "fmt"
    "time"
    
    "github.com/mpool/goroutine-pool"
)

// 实现Jober接口
type MyJob struct {
    id      uint64
    jobType uint32
    data    string
}

func (j *MyJob) UID() uint64        { return j.id }
func (j *MyJob) JobType() uint32    { return j.jobType }
func (j *MyJob) IsServiceReady() bool { return true }

const (
    JobTypeCalculation = 1
    JobTypeIO          = 2
)

func main() {
    // 创建任务管理器
    jm := mpool.NewJobManager(3, 50)
    defer jm.Close()
    
    // 注册任务处理函数
    jm.RegisterJobHandler(JobTypeCalculation, func(job mpool.Jober) error {
        myJob := job.(*MyJob)
        fmt.Printf("处理计算任务: %s\n", myJob.data)
        time.Sleep(200 * time.Millisecond)
        return nil
    })
    
    jm.RegisterJobHandler(JobTypeIO, func(job mpool.Jober) error {
        myJob := job.(*MyJob)
        fmt.Printf("处理IO任务: %s\n", myJob.data)
        time.Sleep(100 * time.Millisecond)
        return nil
    })
    
    // 启动任务管理器
    jm.Start()
    
    // 提交任务
    jobs := []*MyJob{
        {id: 1, jobType: JobTypeCalculation, data: "计算圆周率"},
        {id: 2, jobType: JobTypeIO, data: "读取文件"},
        {id: 3, jobType: JobTypeCalculation, data: "数据分析"},
    }
    
    for _, job := range jobs {
        jm.AddJob(job)
    }
    
    // 等待任务完成
    time.Sleep(2 * time.Second)
    
    // 查看统计信息
    stats := jm.GetStats()
    fmt.Printf("统计信息: %+v\n", stats)
}
```

## API文档

### Pool (基础协程池)

#### 创建协程池
```go
func NewPool(numWorkers, jobQueueLen int) *Pool
```
- `numWorkers`: 工作协程数量
- `jobQueueLen`: 每个工作协程的任务队列长度

#### 添加任务
```go
func (p *Pool) AddJob(job Job) error
```

#### 添加带结果的任务
```go
func (p *Pool) AddJobWithResult(job Job) <-chan error
```

#### 释放资源
```go
func (p *Pool) Release()
func (p *Pool) ReleaseWithTimeout(timeout time.Duration)
```

### JobManager (高级任务管理器)

#### 创建任务管理器
```go
func NewJobManager(workerNum, jobNum uint32) *JobManager
```

#### 注册任务处理函数
```go
func (jm *JobManager) RegisterJobHandler(jobType uint32, fn JobFn) error
```

#### 启动和关闭
```go
func (jm *JobManager) Start() error
func (jm *JobManager) Close()
```

#### 任务控制
```go
func (jm *JobManager) AddJob(job Jober) error
func (jm *JobManager) Pause()
func (jm *JobManager) Resume()
```

#### 统计信息
```go
func (jm *JobManager) GetStats() map[string]interface{}
```

## 最佳实践

### 1. 协程池大小选择
- **CPU密集型任务**: 协程数量 = CPU核心数
- **IO密集型任务**: 协程数量 = CPU核心数 * 2-4
- **混合型任务**: 根据实际测试调整

### 2. 队列长度设置
- 队列过小: 可能导致任务提交失败
- 队列过大: 占用过多内存，关闭时等待时间长
- 建议: 根据任务处理速度和提交频率设置合理值

### 3. 错误处理
```go
// 总是检查任务添加是否成功
if err := pool.AddJob(job); err != nil {
    log.Printf("添加任务失败: %v", err)
    // 处理失败情况
}
```

### 4. 优雅关闭
```go
// 使用defer确保资源释放
defer pool.Release()

// 或者使用超时控制
defer pool.ReleaseWithTimeout(30 * time.Second)
```

### 5. 任务设计原则
- 任务应该是无状态的
- 避免在任务中进行长时间阻塞操作
- 合理设置任务超时时间
- 任务函数应该处理panic情况

## 性能特征

- **内存占用**: 固定的协程数量，内存占用可预测
- **延迟**: 任务提交到执行的延迟极低
- **吞吐量**: 支持高并发任务提交和处理
- **扩展性**: 支持动态调整任务处理策略

## 注意事项

1. **协程池生命周期**: 确保在程序结束前调用`Release()`方法
2. **任务队列满**: 当队列满时，`AddJob`会立即返回错误
3. **panic处理**: 框架会自动恢复任务中的panic，但建议在任务中主动处理
4. **资源清理**: 任务中使用的资源需要自行清理
5. **并发安全**: 所有API都是并发安全的

## 示例代码

更多详细示例请查看 `examples/` 目录：
- `basic_usage.go`: 基础协程池使用示例
- `job_manager_usage.go`: 高级任务管理器示例

## 许可证

MIT License
