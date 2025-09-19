package mpool

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// worker 工作协程结构体，负责执行具体的任务
type worker struct {
	// poolID 所属协程池的ID
	poolID uint64

	// workerID 工作协程的唯一标识符
	workerID int

	// jobChannel 任务接收通道
	jobChannel chan Job

	// stop 停止信号通道
	stop chan struct{}

	// ctx 上下文，用于控制工作协程的生命周期
	ctx context.Context

	// logger 日志记录器
	logger *log.Logger

	// processedJobs 已处理的任务数量
	processedJobs uint64

	// isRunning 标记工作协程是否正在运行
	isRunning int32
}

// newWorker 创建一个新的工作协程
// poolID: 所属协程池ID
// workerID: 工作协程ID
// jobQueueLen: 任务队列长度
// ctx: 上下文
// logger: 日志记录器
func newWorker(poolID uint64, workerID int, jobQueueLen int, ctx context.Context, logger *log.Logger) *worker {
	return &worker{
		poolID:     poolID,
		workerID:   workerID,
		jobChannel: make(chan Job, jobQueueLen),
		stop:       make(chan struct{}),
		ctx:        ctx,
		logger:     logger,
	}
}

// start 启动工作协程，开始处理任务
// wg: 等待组，用于协程池的优雅关闭
func (w *worker) start(wg *sync.WaitGroup) {
	defer func() {
		// 恢复panic，确保工作协程不会因为任务panic而崩溃
		if r := recover(); r != nil {
			w.logger.Printf("工作协程 %d 发生panic: %v\n堆栈信息:\n%s",
				w.workerID, r, debug.Stack())
		}

		// 标记工作协程已停止
		atomic.StoreInt32(&w.isRunning, 0)

		w.logger.Printf("工作协程 %d 已停止，共处理任务 %d 个",
			w.workerID, atomic.LoadUint64(&w.processedJobs))

		// 通知等待组该协程已完成
		wg.Done()
	}()

	// 标记工作协程正在运行
	atomic.StoreInt32(&w.isRunning, 1)
	w.logger.Printf("工作协程 %d 开始运行", w.workerID)

	for {
		select {
		case job := <-w.jobChannel:
			// 处理正常任务
			w.executeJob(job)

		case <-w.stop:
			// 收到停止信号，处理剩余任务后退出
			w.logger.Printf("工作协程 %d 收到停止信号，开始处理剩余任务", w.workerID)
			w.processRemainingJobs()
			return

		case <-w.ctx.Done():
			// 上下文被取消，立即退出
			w.logger.Printf("工作协程 %d 上下文被取消，立即停止", w.workerID)
			return
		}
	}
}

// executeJob 执行单个任务
// job: 要执行的任务
func (w *worker) executeJob(job Job) {
	defer func() {
		// 恢复任务执行过程中的panic
		if r := recover(); r != nil {
			w.logger.Printf("工作协程 %d 执行任务时发生panic: %v\n堆栈信息:\n%s",
				w.workerID, r, debug.Stack())
		}

		// 增加已处理任务计数
		atomic.AddUint64(&w.processedJobs, 1)
	}()

	// 记录任务开始执行
	startTime := time.Now()

	// 执行任务
	err := job.Handle()

	// 记录任务执行结果
	duration := time.Since(startTime)
	if err != nil {
		w.logger.Printf("工作协程 %d 执行任务失败，耗时: %v, 错误: %v",
			w.workerID, duration, err)
	} else {
		w.logger.Printf("工作协程 %d 执行任务成功，耗时: %v",
			w.workerID, duration)
	}
}

// processRemainingJobs 处理剩余的任务
func (w *worker) processRemainingJobs() {
	remainingCount := len(w.jobChannel)
	w.logger.Printf("工作协程 %d 开始处理剩余的 %d 个任务", w.workerID, remainingCount)

	// 设置处理剩余任务的超时时间
	timeout := time.After(10 * time.Second)

	for {
		select {
		case job := <-w.jobChannel:
			w.executeJob(job)

		case <-timeout:
			// 超时，放弃处理剩余任务
			remaining := len(w.jobChannel)
			if remaining > 0 {
				w.logger.Printf("工作协程 %d 处理剩余任务超时，放弃处理剩余的 %d 个任务",
					w.workerID, remaining)
			}
			return

		default:
			// 没有更多任务，退出
			w.logger.Printf("工作协程 %d 已处理完所有剩余任务", w.workerID)
			return
		}
	}
}

// isActive 检查工作协程是否正在运行
func (w *worker) isActive() bool {
	return atomic.LoadInt32(&w.isRunning) == 1
}

// getProcessedJobsCount 获取已处理的任务数量
func (w *worker) getProcessedJobsCount() uint64 {
	return atomic.LoadUint64(&w.processedJobs)
}

// getQueueLength 获取当前任务队列长度
func (w *worker) getQueueLength() int {
	return len(w.jobChannel)
}
