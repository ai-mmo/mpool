package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ai-mmo/mpool"
)

// 示例任务结构体
type ExampleJob struct {
	id          uint64
	jobType     uint32
	data        string
	isReady     bool
	processTime time.Duration
}

// 实现Jober接口
func (j *ExampleJob) UID() uint64 {
	return j.id
}

func (j *ExampleJob) JobType() uint32 {
	return j.jobType
}

func (j *ExampleJob) IsServiceReady() bool {
	return j.isReady
}

// 任务类型常量
const (
	JobTypeCalculation  = 1 // 计算任务
	JobTypeIO           = 2 // IO任务
	JobTypeNotification = 3 // 通知任务
)

// JobManager基本使用示例
func jobManagerBasicExample() {
	fmt.Println("=== JobManager基本使用示例 ===")

	// 创建任务管理器：3个工作协程，每个队列长度50
	jm := mpool.NewJobManager(3, 50)
	defer jm.Close()

	// 注册不同类型的任务处理函数
	err := jm.RegisterJobHandler(JobTypeCalculation, func(job mpool.Jober) error {
		exJob := job.(*ExampleJob)
		fmt.Printf("处理计算任务 %d: %s\n", exJob.UID(), exJob.data)
		time.Sleep(exJob.processTime)

		// 模拟计算结果
		if exJob.UID()%5 == 0 {
			return fmt.Errorf("计算任务 %d 失败", exJob.UID())
		}

		fmt.Printf("计算任务 %d 完成\n", exJob.UID())
		return nil
	})
	if err != nil {
		log.Printf("注册计算任务处理函数失败: %v", err)
		return
	}

	err = jm.RegisterJobHandler(JobTypeIO, func(job mpool.Jober) error {
		exJob := job.(*ExampleJob)
		fmt.Printf("处理IO任务 %d: %s\n", exJob.UID(), exJob.data)
		time.Sleep(exJob.processTime)

		fmt.Printf("IO任务 %d 完成\n", exJob.UID())
		return nil
	})
	if err != nil {
		log.Printf("注册IO任务处理函数失败: %v", err)
		return
	}

	err = jm.RegisterJobHandler(JobTypeNotification, func(job mpool.Jober) error {
		exJob := job.(*ExampleJob)
		fmt.Printf("发送通知 %d: %s\n", exJob.UID(), exJob.data)
		time.Sleep(exJob.processTime)

		fmt.Printf("通知 %d 发送完成\n", exJob.UID())
		return nil
	})
	if err != nil {
		log.Printf("注册通知任务处理函数失败: %v", err)
		return
	}

	// 启动任务管理器
	if err := jm.Start(); err != nil {
		log.Printf("启动任务管理器失败: %v", err)
		return
	}

	// 提交不同类型的任务
	jobs := []*ExampleJob{
		{id: 1, jobType: JobTypeCalculation, data: "计算圆周率", isReady: true, processTime: 200 * time.Millisecond},
		{id: 2, jobType: JobTypeIO, data: "读取文件", isReady: true, processTime: 150 * time.Millisecond},
		{id: 3, jobType: JobTypeNotification, data: "发送邮件", isReady: true, processTime: 100 * time.Millisecond},
		{id: 4, jobType: JobTypeCalculation, data: "矩阵运算", isReady: false, processTime: 300 * time.Millisecond}, // 服务未准备
		{id: 5, jobType: JobTypeCalculation, data: "数据分析", isReady: true, processTime: 250 * time.Millisecond},
		{id: 6, jobType: JobTypeIO, data: "写入数据库", isReady: true, processTime: 180 * time.Millisecond},
	}

	// 提交任务
	for _, job := range jobs {
		if err := jm.AddJob(job); err != nil {
			fmt.Printf("添加任务 %d 失败: %v\n", job.UID(), err)
		}
	}

	// 等待一段时间让任务处理
	time.Sleep(2 * time.Second)

	// 打印统计信息
	stats := jm.GetStats()
	fmt.Printf("任务管理器统计: %+v\n", stats)
}

// 暂停和恢复示例
func jobManagerPauseResumeExample() {
	fmt.Println("\n=== JobManager暂停和恢复示例 ===")

	jm := mpool.NewJobManager(2, 20)
	defer jm.Close()

	// 注册任务处理函数
	jm.RegisterJobHandler(JobTypeCalculation, func(job mpool.Jober) error {
		exJob := job.(*ExampleJob)
		fmt.Printf("处理任务 %d: %s\n", exJob.UID(), exJob.data)
		time.Sleep(exJob.processTime)
		fmt.Printf("任务 %d 完成\n", exJob.UID())
		return nil
	})

	// 启动任务管理器
	jm.Start()

	// 提交一些任务
	for i := 1; i <= 5; i++ {
		job := &ExampleJob{
			id:          uint64(i),
			jobType:     JobTypeCalculation,
			data:        fmt.Sprintf("任务-%d", i),
			isReady:     true,
			processTime: 300 * time.Millisecond,
		}
		jm.AddJob(job)
	}

	// 等待一段时间后暂停
	time.Sleep(500 * time.Millisecond)
	fmt.Println("暂停任务管理器...")
	jm.Pause()

	// 在暂停状态下尝试添加更多任务
	for i := 6; i <= 8; i++ {
		job := &ExampleJob{
			id:          uint64(i),
			jobType:     JobTypeCalculation,
			data:        fmt.Sprintf("暂停期间任务-%d", i),
			isReady:     true,
			processTime: 200 * time.Millisecond,
		}
		if err := jm.AddJob(job); err != nil {
			fmt.Printf("暂停期间添加任务失败: %v\n", err)
		}
	}

	// 等待一段时间后恢复
	time.Sleep(1 * time.Second)
	fmt.Println("恢复任务管理器...")
	jm.Resume()

	// 恢复后添加更多任务
	for i := 9; i <= 10; i++ {
		job := &ExampleJob{
			id:          uint64(i),
			jobType:     JobTypeCalculation,
			data:        fmt.Sprintf("恢复后任务-%d", i),
			isReady:     true,
			processTime: 150 * time.Millisecond,
		}
		jm.AddJob(job)
	}

	// 等待所有任务完成
	time.Sleep(3 * time.Second)

	// 打印最终统计
	stats := jm.GetStats()
	fmt.Printf("最终统计: %+v\n", stats)
}

// 高并发任务处理示例
func jobManagerHighConcurrencyExample() {
	fmt.Println("\n=== JobManager高并发示例 ===")

	jm := mpool.NewJobManager(5, 100)
	defer jm.Close()

	// 注册任务处理函数
	jm.RegisterJobHandler(JobTypeCalculation, func(job mpool.Jober) error {
		exJob := job.(*ExampleJob)
		// 模拟CPU密集型任务
		start := time.Now()
		for time.Since(start) < exJob.processTime {
			// 模拟计算
		}
		fmt.Printf("高并发任务 %d 完成\n", exJob.UID())
		return nil
	})

	jm.Start()

	// 使用多个goroutine并发提交任务
	var wg sync.WaitGroup
	numProducers := 3
	tasksPerProducer := 20

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			for i := 0; i < tasksPerProducer; i++ {
				job := &ExampleJob{
					id:          uint64(producerID*1000 + i),
					jobType:     JobTypeCalculation,
					data:        fmt.Sprintf("生产者%d-任务%d", producerID, i),
					isReady:     true,
					processTime: time.Duration(50+i*10) * time.Millisecond,
				}

				if err := jm.AddJob(job); err != nil {
					fmt.Printf("生产者 %d 添加任务 %d 失败: %v\n", producerID, i, err)
				}
			}

			fmt.Printf("生产者 %d 完成任务提交\n", producerID)
		}(p)
	}

	// 定期打印统计信息
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for i := 0; i < 10; i++ {
			<-ticker.C
			stats := jm.GetStats()
			fmt.Printf("实时统计[%d]: 已添加=%d, 已处理=%d, 队列长度=%d\n",
				i+1, stats["addedJobs"], stats["processedJobs"], stats["queueLength"])
		}
	}()

	// 等待所有生产者完成
	wg.Wait()

	// 等待所有任务处理完成
	time.Sleep(5 * time.Second)

	// 打印最终统计
	finalStats := jm.GetStats()
	fmt.Printf("高并发测试最终统计: %+v\n", finalStats)
}

func main1() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 运行各种JobManager示例
	jobManagerBasicExample()
	jobManagerPauseResumeExample()
	jobManagerHighConcurrencyExample()

	fmt.Println("\n=== 所有JobManager示例运行完成 ===")
}
