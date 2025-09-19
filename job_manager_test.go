package mpool

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJob 测试用的任务结构体
type TestJob struct {
	id       uint64
	jobType  uint32
	data     string
	isReady  bool
	executed int64
}

func (j *TestJob) UID() uint64 {
	return j.id
}

func (j *TestJob) JobType() uint32 {
	return j.jobType
}

func (j *TestJob) IsServiceReady() bool {
	return j.isReady
}

func (j *TestJob) MarkExecuted() {
	atomic.StoreInt64(&j.executed, 1)
}

func (j *TestJob) IsExecuted() bool {
	return atomic.LoadInt64(&j.executed) == 1
}

const (
	TestJobTypeA = 1
	TestJobTypeB = 2
	TestJobTypeC = 3
)

// TestJobManagerBasicFunctionality 测试JobManager基本功能
func TestJobManagerBasicFunctionality(t *testing.T) {
	jm := NewJobManager(2, 10)
	defer jm.Close()

	var processedCount int64

	// 注册任务处理函数
	err := jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		testJob := job.(*TestJob)
		testJob.MarkExecuted()
		atomic.AddInt64(&processedCount, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	// 启动任务管理器
	err = jm.Start()
	require.NoError(t, err)

	// 提交任务
	jobs := []*TestJob{
		{id: 1, jobType: TestJobTypeA, data: "任务1", isReady: true},
		{id: 2, jobType: TestJobTypeA, data: "任务2", isReady: true},
		{id: 3, jobType: TestJobTypeA, data: "任务3", isReady: true},
	}

	for _, job := range jobs {
		err := jm.AddJob(job)
		require.NoError(t, err)
	}

	// 等待任务完成
	time.Sleep(500 * time.Millisecond)

	// 验证结果
	assert.Equal(t, int64(3), atomic.LoadInt64(&processedCount))
	for i, job := range jobs {
		assert.True(t, job.IsExecuted(), "任务 %d 应该被执行", i+1)
	}

	// 检查统计信息
	stats := jm.GetStats()
	assert.Equal(t, true, stats["isRunning"])
	assert.Equal(t, uint64(3), stats["addedJobs"])
	assert.Equal(t, uint64(3), stats["processedJobs"])
}

// TestJobManagerMultipleTypes 测试多种任务类型
func TestJobManagerMultipleTypes(t *testing.T) {
	jm := NewJobManager(3, 20)
	defer jm.Close()

	var typeACount, typeBCount int64

	// 注册不同类型的任务处理函数
	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		atomic.AddInt64(&typeACount, 1)
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	jm.RegisterJobHandler(TestJobTypeB, func(job Jober) error {
		atomic.AddInt64(&typeBCount, 1)
		time.Sleep(30 * time.Millisecond)
		return nil
	})

	jm.Start()

	// 提交不同类型的任务
	jobs := []*TestJob{
		{id: 1, jobType: TestJobTypeA, isReady: true},
		{id: 2, jobType: TestJobTypeB, isReady: true},
		{id: 3, jobType: TestJobTypeA, isReady: true},
		{id: 4, jobType: TestJobTypeB, isReady: true},
		{id: 5, jobType: TestJobTypeC, isReady: true}, // 未注册的类型
	}

	for _, job := range jobs {
		jm.AddJob(job)
	}

	time.Sleep(1 * time.Second)

	assert.Equal(t, int64(2), atomic.LoadInt64(&typeACount))
	assert.Equal(t, int64(2), atomic.LoadInt64(&typeBCount))

	stats := jm.GetStats()
	assert.Equal(t, uint64(5), stats["addedJobs"])
	assert.Equal(t, uint64(4), stats["processedJobs"]) // 只有4个任务被处理
}

// TestJobManagerServiceNotReady 测试服务未准备就绪的情况
func TestJobManagerServiceNotReady(t *testing.T) {
	jm := NewJobManager(2, 10)
	defer jm.Close()

	var processedCount int64

	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		atomic.AddInt64(&processedCount, 1)
		return nil
	})

	jm.Start()

	// 提交服务未准备就绪的任务
	jobs := []*TestJob{
		{id: 1, jobType: TestJobTypeA, isReady: true},  // 会被处理
		{id: 2, jobType: TestJobTypeA, isReady: false}, // 不会被处理
		{id: 3, jobType: TestJobTypeA, isReady: true},  // 会被处理
	}

	for _, job := range jobs {
		jm.AddJob(job)
	}

	time.Sleep(300 * time.Millisecond)

	// 只有2个任务应该被处理
	assert.Equal(t, int64(2), atomic.LoadInt64(&processedCount))
}

// TestJobManagerPauseResume 测试暂停和恢复功能
func TestJobManagerPauseResume(t *testing.T) {
	jm := NewJobManager(2, 10)
	defer jm.Close()

	var processedCount int64

	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		atomic.AddInt64(&processedCount, 1)
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	jm.Start()

	// 提交一些任务
	for i := 1; i <= 3; i++ {
		job := &TestJob{id: uint64(i), jobType: TestJobTypeA, isReady: true}
		jm.AddJob(job)
	}

	// 等待一些任务开始处理
	time.Sleep(50 * time.Millisecond)

	// 暂停任务管理器
	jm.Pause()
	stats := jm.GetStats()
	assert.Equal(t, true, stats["isPaused"])

	// 在暂停状态下添加任务应该失败
	job := &TestJob{id: 4, jobType: TestJobTypeA, isReady: true}
	err := jm.AddJob(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "暂停")

	// 记录暂停时的处理数量
	pausedCount := atomic.LoadInt64(&processedCount)

	// 等待一段时间，确保暂停期间没有新任务被处理
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, pausedCount, atomic.LoadInt64(&processedCount))

	// 恢复任务管理器
	jm.Resume()
	stats = jm.GetStats()
	assert.Equal(t, false, stats["isPaused"])

	// 恢复后应该能添加任务
	job = &TestJob{id: 5, jobType: TestJobTypeA, isReady: true}
	err = jm.AddJob(job)
	assert.NoError(t, err)

	// 等待所有任务完成
	time.Sleep(500 * time.Millisecond)

	finalCount := atomic.LoadInt64(&processedCount)
	assert.True(t, finalCount >= 3, "应该处理至少3个任务")
}

// TestJobManagerConcurrentAccess 测试并发访问
func TestJobManagerConcurrentAccess(t *testing.T) {
	jm := NewJobManager(4, 50)
	defer jm.Close()

	var processedCount int64

	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		atomic.AddInt64(&processedCount, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	jm.Start()

	// 并发提交任务
	numProducers := 5
	tasksPerProducer := 10

	done := make(chan bool, numProducers)

	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			for i := 0; i < tasksPerProducer; i++ {
				job := &TestJob{
					id:      uint64(producerID*1000 + i),
					jobType: TestJobTypeA,
					isReady: true,
				}
				jm.AddJob(job)
			}
			done <- true
		}(p)
	}

	// 等待所有生产者完成
	for i := 0; i < numProducers; i++ {
		<-done
	}

	// 等待所有任务处理完成
	time.Sleep(2 * time.Second)

	finalCount := atomic.LoadInt64(&processedCount)
	expectedCount := int64(numProducers * tasksPerProducer)
	assert.Equal(t, expectedCount, finalCount, "应该处理所有提交的任务")

	stats := jm.GetStats()
	assert.Equal(t, uint64(expectedCount), stats["addedJobs"])
	assert.Equal(t, uint64(expectedCount), stats["processedJobs"])
}

// TestJobManagerErrorHandling 测试错误处理
func TestJobManagerErrorHandling(t *testing.T) {
	jm := NewJobManager(2, 10)
	defer jm.Close()

	var processedCount, errorCount int64

	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		testJob := job.(*TestJob)
		atomic.AddInt64(&processedCount, 1)
		
		// 模拟部分任务失败
		if testJob.UID()%2 == 0 {
			atomic.AddInt64(&errorCount, 1)
			return fmt.Errorf("任务 %d 处理失败", testJob.UID())
		}
		
		return nil
	})

	jm.Start()

	// 提交任务
	for i := 1; i <= 6; i++ {
		job := &TestJob{id: uint64(i), jobType: TestJobTypeA, isReady: true}
		jm.AddJob(job)
	}

	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int64(6), atomic.LoadInt64(&processedCount))
	assert.Equal(t, int64(3), atomic.LoadInt64(&errorCount)) // 偶数ID的任务失败
}

// TestJobManagerStartWithoutHandlers 测试没有注册处理函数时启动
func TestJobManagerStartWithoutHandlers(t *testing.T) {
	jm := NewJobManager(2, 10)
	defer jm.Close()

	// 尝试启动没有注册任何处理函数的任务管理器
	err := jm.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "没有注册任何任务处理函数")

	stats := jm.GetStats()
	assert.Equal(t, false, stats["isRunning"])
}

// BenchmarkJobManagerThroughput JobManager性能基准测试
func BenchmarkJobManagerThroughput(b *testing.B) {
	jm := NewJobManager(4, 100)
	defer jm.Close()

	jm.RegisterJobHandler(TestJobTypeA, func(job Jober) error {
		// 模拟轻量级处理
		time.Sleep(time.Microsecond)
		return nil
	})

	jm.Start()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var id uint64
		for pb.Next() {
			id++
			job := &TestJob{
				id:      id,
				jobType: TestJobTypeA,
				isReady: true,
			}
			jm.AddJob(job)
		}
	})
}
