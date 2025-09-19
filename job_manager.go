package mpool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Jober 任务接口，定义了任务必须实现的方法
type Jober interface {
	// UID 返回任务的唯一标识符
	UID() uint64

	// JobType 返回任务类型
	JobType() uint32

	// IsServiceReady 检查服务是否准备就绪
	IsServiceReady() bool
}

// 默认任务通道长度
const (
	DefaultChannelLength = 10000
)

// JobFn 任务处理函数类型
type JobFn func(Jober) error

// JobManager 高级任务管理器，提供类型化的任务处理功能
type JobManager struct {
	// isPaused 暂停状态标志
	isPaused int32

	// isRunning 运行状态标志
	isRunning int32

	// type2Fns 任务类型到处理函数的映射
	type2Fns map[uint32]JobFn

	// jobChan 任务接收通道
	jobChan chan Jober

	// ctx 上下文，用于控制生命周期
	ctx context.Context

	// cancel 取消函数
	cancel context.CancelFunc

	// pool 底层协程池
	pool *Pool

	// addedJobs 已添加的任务数量
	addedJobs uint64

	// processedJobs 已处理的任务数量
	processedJobs uint64

	// logger 日志记录器
	logger *log.Logger

	// processingWg 处理协程的等待组
	processingWg sync.WaitGroup
}

// NewJobManager 创建一个新的任务管理器
// workerNum: 工作协程数量
// jobNum: 每个工作协程的任务队列长度
func NewJobManager(workerNum, jobNum uint32) *JobManager {
	if workerNum == 0 {
		workerNum = 4
	}
	if jobNum == 0 {
		jobNum = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	jm := &JobManager{
		type2Fns: make(map[uint32]JobFn),
		jobChan:  make(chan Jober, DefaultChannelLength),
		ctx:      ctx,
		cancel:   cancel,
		pool:     NewPool(int(workerNum), int(jobNum)),
		logger:   log.New(log.Writer(), "[JobManager] ", log.LstdFlags),
	}

	jm.logger.Printf("创建任务管理器，工作协程数量: %d, 队列长度: %d", workerNum, jobNum)
	return jm
}

// Close 关闭任务管理器
func (jm *JobManager) Close() {
	jm.logger.Printf("开始关闭任务管理器")
	defer jm.logger.Printf("任务管理器关闭完成")

	// 设置为非运行状态
	atomic.StoreInt32(&jm.isRunning, 0)

	// 关闭底层协程池
	jm.pool.Release()

	// 取消上下文
	jm.cancel()

	// 等待处理协程结束
	jm.processingWg.Wait()
}

// RegisterJobHandler 注册任务处理函数
// jobType: 任务类型
// fn: 处理函数
func (jm *JobManager) RegisterJobHandler(jobType uint32, fn JobFn) error {
	if atomic.LoadInt32(&jm.isRunning) == 1 {
		return fmt.Errorf("任务管理器正在运行，无法注册新的处理函数")
	}

	if _, exists := jm.type2Fns[jobType]; exists {
		jm.logger.Printf("任务类型 %d 的处理函数已存在，将被覆盖", jobType)
	}

	jm.type2Fns[jobType] = fn
	jm.logger.Printf("注册任务类型 %d 的处理函数", jobType)
	return nil
}

// Start 启动任务管理器
func (jm *JobManager) Start() error {
	if !atomic.CompareAndSwapInt32(&jm.isRunning, 0, 1) {
		return fmt.Errorf("任务管理器已经在运行")
	}

	if len(jm.type2Fns) == 0 {
		atomic.StoreInt32(&jm.isRunning, 0)
		return fmt.Errorf("没有注册任何任务处理函数")
	}

	jm.logger.Printf("启动任务管理器，已注册 %d 种任务类型", len(jm.type2Fns))

	// 启动任务处理协程
	jm.processingWg.Add(1)
	go jm.processJobs()

	return nil
}

// AddJob 添加任务到管理器
// job: 要添加的任务
func (jm *JobManager) AddJob(job Jober) error {
	if atomic.LoadInt32(&jm.isRunning) == 0 {
		return fmt.Errorf("任务管理器未运行")
	}

	if atomic.LoadInt32(&jm.isPaused) == 1 {
		return fmt.Errorf("任务管理器已暂停")
	}

	atomic.AddUint64(&jm.addedJobs, 1)
	jm.logger.Printf("添加任务，UID: %d, 类型: %d, 总添加: %d",
		job.UID(), job.JobType(), atomic.LoadUint64(&jm.addedJobs))

	select {
	case jm.jobChan <- job:
		jm.logger.Printf("任务添加成功，UID: %d, 当前队列长度: %d", job.UID(), len(jm.jobChan))
		return nil
	case <-jm.ctx.Done():
		return fmt.Errorf("任务管理器已关闭")
	default:
		return fmt.Errorf("任务队列已满，无法添加任务")
	}
}

// Pause 暂停任务处理
func (jm *JobManager) Pause() {
	if atomic.CompareAndSwapInt32(&jm.isPaused, 0, 1) {
		jm.logger.Printf("任务管理器已暂停")
	}
}

// Resume 恢复任务处理
func (jm *JobManager) Resume() {
	if atomic.CompareAndSwapInt32(&jm.isPaused, 1, 0) {
		jm.logger.Printf("任务管理器已恢复")
	}
}

// GetStats 获取统计信息
func (jm *JobManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"isRunning":       atomic.LoadInt32(&jm.isRunning) == 1,
		"isPaused":        atomic.LoadInt32(&jm.isPaused) == 1,
		"addedJobs":       atomic.LoadUint64(&jm.addedJobs),
		"processedJobs":   atomic.LoadUint64(&jm.processedJobs),
		"queueLength":     len(jm.jobChan),
		"registeredTypes": len(jm.type2Fns),
	}
}

// processJobs 处理任务的主循环
func (jm *JobManager) processJobs() {
	defer jm.processingWg.Done()
	jm.logger.Printf("任务处理协程启动")

	for {
		select {
		case job := <-jm.jobChan:
			if atomic.LoadInt32(&jm.isPaused) == 0 {
				jm.handleJob(job)
			} else {
				// 暂停状态下，将任务放回队列
				select {
				case jm.jobChan <- job:
				default:
					jm.logger.Printf("暂停状态下任务队列已满，丢弃任务 UID: %d", job.UID())
				}
				time.Sleep(100 * time.Millisecond) // 暂停时稍作等待
			}

		case <-jm.ctx.Done():
			jm.logger.Printf("收到关闭信号，开始处理剩余任务")
			jm.handleRemainingJobs()
			return
		}
	}
}

// handleJob 处理单个任务
func (jm *JobManager) handleJob(job Jober) {
	defer func() {
		if r := recover(); r != nil {
			jm.logger.Printf("处理任务时发生panic，UID: %d, 错误: %v", job.UID(), r)
		}
	}()

	// 检查是否有对应的处理函数
	fn, exists := jm.type2Fns[job.JobType()]
	if !exists {
		jm.logger.Printf("未找到任务类型 %d 的处理函数，UID: %d", job.JobType(), job.UID())
		return
	}

	// 检查服务是否准备就绪
	if !job.IsServiceReady() {
		jm.logger.Printf("服务未准备就绪，跳过任务，UID: %d, 类型: %d", job.UID(), job.JobType())
		return
	}

	// 提交任务到协程池
	err := jm.pool.AddJob(Job{
		WorkerID: int(job.UID()),
		Handle: func() error {
			startTime := time.Now()
			err := fn(job)
			duration := time.Since(startTime)

			atomic.AddUint64(&jm.processedJobs, 1)

			if err != nil {
				jm.logger.Printf("任务处理失败，UID: %d, 耗时: %v, 错误: %v",
					job.UID(), duration, err)
			} else {
				jm.logger.Printf("任务处理成功，UID: %d, 耗时: %v", job.UID(), duration)
			}

			return err
		},
	})

	if err != nil {
		jm.logger.Printf("提交任务到协程池失败，UID: %d, 错误: %v", job.UID(), err)
	}
}

// handleRemainingJobs 处理剩余任务
func (jm *JobManager) handleRemainingJobs() {
	remainingCount := len(jm.jobChan)
	jm.logger.Printf("开始处理剩余的 %d 个任务", remainingCount)

	timeout := time.After(30 * time.Second)

	for {
		select {
		case job := <-jm.jobChan:
			jm.handleJob(job)

		case <-timeout:
			remaining := len(jm.jobChan)
			if remaining > 0 {
				jm.logger.Printf("处理剩余任务超时，放弃处理剩余的 %d 个任务", remaining)
			}
			return

		default:
			jm.logger.Printf("所有剩余任务处理完成")
			return
		}
	}
}
