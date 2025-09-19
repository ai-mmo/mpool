package mpool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// 全局协程池ID计数器，用于区分不同的协程池实例
var poolID uint64

// Job 表示一个待执行的工作任务
type Job struct {
	// WorkerID 指定执行该任务的工作协程ID
	// 如果为-1，则随机分配给任何可用的工作协程
	// 如果指定具体ID，则会分配给对应的工作协程（取模运算）
	WorkerID int

	// Handle 任务的具体执行函数
	// 返回error表示任务执行结果，nil表示成功
	Handle func() error
}

// JobWithResult 带返回结果的任务
type JobWithResult struct {
	Job
	// ResultChan 用于接收任务执行结果的通道
	ResultChan chan error
}

// Pool 协程池主结构体，管理一组工作协程来并发执行任务
type Pool struct {
	// id 协程池的唯一标识符
	id uint64

	// dispatcher 任务分发器，负责将任务分配给工作协程
	dispatcher *dispatcher

	// wg 等待组，用于协程池的优雅关闭
	wg sync.WaitGroup

	// ctx 上下文，用于控制协程池的生命周期
	ctx context.Context

	// cancel 取消函数，用于停止协程池
	cancel context.CancelFunc

	// closed 标记协程池是否已关闭
	closed int32

	// logger 日志记录器
	logger *log.Logger
}

// NewPool 创建一个新的协程池
// numWorkers: 工作协程数量
// jobQueueLen: 每个工作协程的任务队列长度
// 返回: 协程池实例
func NewPool(numWorkers, jobQueueLen int) *Pool {
	if numWorkers <= 0 {
		numWorkers = 1
	}
	if jobQueueLen <= 0 {
		jobQueueLen = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &Pool{
		id:     atomic.AddUint64(&poolID, 1),
		ctx:    ctx,
		cancel: cancel,
		logger: log.New(log.Writer(), fmt.Sprintf("[Pool-%d] ", atomic.LoadUint64(&poolID)), log.LstdFlags),
	}

	pool.logger.Printf("创建协程池，工作协程数量: %d, 队列长度: %d", numWorkers, jobQueueLen)

	// 设置等待组计数
	pool.wg.Add(numWorkers)

	// 创建任务分发器
	pool.dispatcher = newDispatcher(numWorkers, jobQueueLen, pool.id, &pool.wg, pool.ctx, pool.logger)

	return pool
}

// AddJob 向协程池添加一个任务
// job: 要执行的任务
func (p *Pool) AddJob(job Job) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return fmt.Errorf("协程池已关闭，无法添加新任务")
	}

	return p.dispatcher.dispatch(job)
}

// AddJobWithResult 向协程池添加一个带结果返回的任务
// job: 要执行的任务
// 返回: 结果通道，可以从中接收任务执行结果
func (p *Pool) AddJobWithResult(job Job) <-chan error {
	resultChan := make(chan error, 1)

	if atomic.LoadInt32(&p.closed) == 1 {
		resultChan <- fmt.Errorf("协程池已关闭，无法添加新任务")
		close(resultChan)
		return resultChan
	}

	// 包装原始任务，添加结果返回逻辑
	wrappedJob := Job{
		WorkerID: job.WorkerID,
		Handle: func() error {
			defer close(resultChan)
			err := job.Handle()
			resultChan <- err
			return err
		},
	}

	if err := p.dispatcher.dispatch(wrappedJob); err != nil {
		resultChan <- err
		close(resultChan)
	}

	return resultChan
}

// Release 释放协程池资源，等待所有任务完成后关闭
func (p *Pool) Release() {
	p.ReleaseWithTimeout(30 * time.Second)
}

// ReleaseWithTimeout 在指定超时时间内释放协程池资源
// timeout: 等待超时时间
func (p *Pool) ReleaseWithTimeout(timeout time.Duration) {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		p.logger.Printf("协程池已经关闭")
		return
	}

	p.logger.Printf("开始释放协程池资源")

	// 关闭分发器，停止接收新任务
	p.dispatcher.close()

	// 等待所有工作协程完成，带超时控制
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Printf("所有工作协程已完成，协程池释放成功")
	case <-time.After(timeout):
		p.logger.Printf("等待超时(%v)，强制关闭协程池", timeout)
		p.cancel() // 强制取消所有协程
	}

	p.logger.Printf("协程池资源释放完成")
}
