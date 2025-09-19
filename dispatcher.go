package mpool

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// 分发器状态常量
const (
	dispatcherStatusOpen   int32 = 0 // 分发器开启状态
	dispatcherStatusClosed int32 = 1 // 分发器关闭状态
)

// dispatcher 任务分发器，负责将任务分配给工作协程
type dispatcher struct {
	// status 分发器状态，0表示开启，1表示关闭
	status int32

	// poolID 所属协程池ID
	poolID uint64

	// workerNum 工作协程数量
	workerNum int

	// workers 工作协程切片
	workers []*worker

	// ctx 上下文，用于控制分发器生命周期
	ctx context.Context

	// logger 日志记录器
	logger *log.Logger

	// dispatchedJobs 已分发的任务数量
	dispatchedJobs uint64

	// rand 随机数生成器，用于随机分配任务
	rand *rand.Rand
}

// newDispatcher 创建一个新的任务分发器
// numWorkers: 工作协程数量
// jobQueueLen: 每个工作协程的任务队列长度
// poolID: 协程池ID
// wg: 等待组
// ctx: 上下文
// logger: 日志记录器
func newDispatcher(numWorkers, jobQueueLen int, poolID uint64, wg *sync.WaitGroup, ctx context.Context, logger *log.Logger) *dispatcher {
	d := &dispatcher{
		status:    dispatcherStatusOpen,
		poolID:    poolID,
		workerNum: numWorkers,
		workers:   make([]*worker, numWorkers),
		ctx:       ctx,
		logger:    logger,
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	d.logger.Printf("创建任务分发器，工作协程数量: %d", numWorkers)

	// 创建并启动工作协程
	for index := 0; index < numWorkers; index++ {
		jobWorker := newWorker(poolID, index, jobQueueLen, ctx, logger)
		d.workers[index] = jobWorker

		// 启动工作协程
		go func(worker *worker) {
			defer func() {
				// 恢复panic，防止单个工作协程的panic影响整个分发器
				if err := recover(); err != nil {
					d.logger.Printf("工作协程启动时发生panic: %v\n堆栈信息:\n%s", err, debug.Stack())
				}
			}()

			worker.start(wg)
		}(jobWorker)
	}

	d.logger.Printf("任务分发器创建完成，所有工作协程已启动")
	return d
}

// dispatch 分发任务到工作协程
// job: 要分发的任务
func (d *dispatcher) dispatch(job Job) error {
	defer func() {
		if err := recover(); err != nil {
			d.logger.Printf("任务分发时发生panic: %v\n堆栈信息:\n%s", err, debug.Stack())
		}
	}()

	// 检查分发器状态
	if atomic.LoadInt32(&d.status) == dispatcherStatusClosed {
		return fmt.Errorf("任务分发器已关闭，无法分发任务")
	}

	// 增加已分发任务计数
	atomic.AddUint64(&d.dispatchedJobs, 1)

	// 选择目标工作协程
	var targetWorker *worker
	if job.WorkerID < 0 {
		// 随机选择工作协程
		index := d.rand.Intn(d.workerNum)
		targetWorker = d.workers[index]
		d.logger.Printf("随机分配任务到工作协程 %d", index)
	} else {
		// 指定工作协程
		index := job.WorkerID % d.workerNum
		targetWorker = d.workers[index]
		d.logger.Printf("指定分配任务到工作协程 %d", index)
	}

	// 尝试发送任务到工作协程
	select {
	case targetWorker.jobChannel <- job:
		d.logger.Printf("任务成功分发到工作协程 %d，当前队列长度: %d",
			targetWorker.workerID, len(targetWorker.jobChannel))
		return nil
	case <-d.ctx.Done():
		return fmt.Errorf("上下文已取消，任务分发失败")
	default:
		return fmt.Errorf("工作协程 %d 队列已满，任务分发失败", targetWorker.workerID)
	}
}

// close 关闭任务分发器
func (d *dispatcher) close() {
	defer d.logger.Printf("任务分发器关闭完成")

	// 检查是否已经关闭
	if !atomic.CompareAndSwapInt32(&d.status, dispatcherStatusOpen, dispatcherStatusClosed) {
		d.logger.Printf("任务分发器已经关闭")
		return
	}

	d.logger.Printf("开始关闭任务分发器，工作协程数量: %d", len(d.workers))

	// 向所有工作协程发送停止信号
	for i, worker := range d.workers {
		select {
		case worker.stop <- struct{}{}:
			d.logger.Printf("向工作协程 %d 发送停止信号成功", i)
		default:
			d.logger.Printf("向工作协程 %d 发送停止信号失败，通道可能已满", i)
		}
	}
}

// getStats 获取分发器统计信息
func (d *dispatcher) getStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["poolID"] = d.poolID
	stats["workerNum"] = d.workerNum
	stats["dispatchedJobs"] = atomic.LoadUint64(&d.dispatchedJobs)
	stats["status"] = atomic.LoadInt32(&d.status)

	// 获取每个工作协程的统计信息
	workerStats := make([]map[string]interface{}, len(d.workers))
	for i, worker := range d.workers {
		workerStats[i] = map[string]interface{}{
			"workerID":      worker.workerID,
			"processedJobs": worker.getProcessedJobsCount(),
			"queueLength":   worker.getQueueLength(),
			"isActive":      worker.isActive(),
		}
	}
	stats["workers"] = workerStats

	return stats
}
