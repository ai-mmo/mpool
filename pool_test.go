package mpool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPoolBasicFunctionality 测试协程池基本功能
func TestPoolBasicFunctionality(t *testing.T) {
	pool := NewPool(2, 5)
	defer pool.Release()

	var counter int64
	var wg sync.WaitGroup

	// 提交10个任务
	for i := 0; i < 10; i++ {
		wg.Add(1)
		err := pool.AddJob(Job{
			WorkerID: -1,
			Handle: func() error {
				defer wg.Done()
				atomic.AddInt64(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		})
		require.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int64(10), atomic.LoadInt64(&counter))
}

// TestPoolSpecificWorker 测试指定工作协程功能
func TestPoolSpecificWorker(t *testing.T) {
	pool := NewPool(3, 5)
	defer pool.Release()

	workerCounts := make([]int64, 3)
	var wg sync.WaitGroup

	// 为每个工作协程分配任务
	for i := 0; i < 9; i++ {
		wg.Add(1)
		workerID := i % 3
		err := pool.AddJob(Job{
			WorkerID: workerID,
			Handle: func() error {
				defer wg.Done()
				atomic.AddInt64(&workerCounts[workerID], 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		})
		require.NoError(t, err)
	}

	wg.Wait()

	// 验证每个工作协程都处理了3个任务
	for i, count := range workerCounts {
		assert.Equal(t, int64(3), count, "工作协程 %d 应该处理3个任务", i)
	}
}

// TestPoolWithResult 测试带结果返回的任务
func TestPoolWithResult(t *testing.T) {
	pool := NewPool(2, 3)
	defer pool.Release()

	// 提交带结果的任务
	resultChans := make([]<-chan error, 5)
	for i := 0; i < 5; i++ {
		taskID := i
		resultChan := pool.AddJobWithResult(Job{
			WorkerID: -1,
			Handle: func() error {
				time.Sleep(50 * time.Millisecond)
				if taskID%2 == 0 {
					return fmt.Errorf("任务 %d 失败", taskID)
				}
				return nil
			},
		})
		resultChans[i] = resultChan
	}

	// 收集结果
	successCount := 0
	errorCount := 0
	for i, resultChan := range resultChans {
		select {
		case err := <-resultChan:
			if err != nil {
				errorCount++
				t.Logf("任务 %d 失败: %v", i, err)
			} else {
				successCount++
				t.Logf("任务 %d 成功", i)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("任务 %d 超时", i)
		}
	}

	assert.Equal(t, 2, successCount, "应该有2个任务成功")
	assert.Equal(t, 3, errorCount, "应该有3个任务失败")
}

// TestPoolConcurrentAccess 测试并发访问
func TestPoolConcurrentAccess(t *testing.T) {
	pool := NewPool(4, 10)
	defer pool.Release()

	var counter int64
	var wg sync.WaitGroup

	// 启动多个goroutine并发提交任务
	numProducers := 5
	tasksPerProducer := 20

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				err := pool.AddJob(Job{
					WorkerID: -1,
					Handle: func() error {
						atomic.AddInt64(&counter, 1)
						time.Sleep(5 * time.Millisecond)
						return nil
					},
				})
				if err != nil {
					t.Logf("添加任务失败: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(2 * time.Second)

	finalCount := atomic.LoadInt64(&counter)
	t.Logf("最终处理任务数: %d", finalCount)
	assert.True(t, finalCount > 0, "应该处理了一些任务")
}

// TestPoolTimeout 测试超时释放
func TestPoolTimeout(t *testing.T) {
	pool := NewPool(2, 3)

	var wg sync.WaitGroup

	// 提交一些长时间运行的任务
	for i := 0; i < 3; i++ {
		wg.Add(1)
		err := pool.AddJob(Job{
			WorkerID: -1,
			Handle: func() error {
				defer wg.Done()
				time.Sleep(2 * time.Second) // 长时间任务
				return nil
			},
		})
		require.NoError(t, err)
	}

	// 测试超时释放
	start := time.Now()
	pool.ReleaseWithTimeout(500 * time.Millisecond)
	duration := time.Since(start)

	// 应该在超时时间内完成释放
	assert.True(t, duration < 1*time.Second, "释放应该在1秒内完成")

	wg.Wait() // 等待任务完成（可能被强制中断）
}

// TestPoolClosedState 测试关闭状态下的行为
func TestPoolClosedState(t *testing.T) {
	pool := NewPool(2, 3)
	pool.Release() // 立即关闭

	// 尝试向已关闭的协程池添加任务
	err := pool.AddJob(Job{
		WorkerID: -1,
		Handle: func() error {
			return nil
		},
	})

	assert.Error(t, err, "向已关闭的协程池添加任务应该返回错误")
	assert.Contains(t, err.Error(), "已关闭", "错误信息应该包含'已关闭'")
}

// TestPoolPanicRecovery 测试panic恢复
func TestPoolPanicRecovery(t *testing.T) {
	pool := NewPool(2, 5)
	defer pool.Release()

	var normalTaskCount int64
	var wg sync.WaitGroup

	// 提交一个会panic的任务
	wg.Add(1)
	err := pool.AddJob(Job{
		WorkerID: -1,
		Handle: func() error {
			defer wg.Done()
			panic("测试panic")
		},
	})
	require.NoError(t, err)

	// 提交一些正常任务
	for i := 0; i < 5; i++ {
		wg.Add(1)
		err := pool.AddJob(Job{
			WorkerID: -1,
			Handle: func() error {
				defer wg.Done()
				atomic.AddInt64(&normalTaskCount, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		})
		require.NoError(t, err)
	}

	wg.Wait()

	// 验证正常任务仍然能够执行
	assert.Equal(t, int64(5), atomic.LoadInt64(&normalTaskCount), "正常任务应该都能执行")
}

// BenchmarkPoolThroughput 性能基准测试
func BenchmarkPoolThroughput(b *testing.B) {
	pool := NewPool(4, 100)
	defer pool.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var wg sync.WaitGroup
			wg.Add(1)
			
			err := pool.AddJob(Job{
				WorkerID: -1,
				Handle: func() error {
					defer wg.Done()
					// 模拟轻量级任务
					time.Sleep(time.Microsecond)
					return nil
				},
			})
			
			if err == nil {
				wg.Wait()
			}
		}
	})
}

// BenchmarkPoolMemoryUsage 内存使用基准测试
func BenchmarkPoolMemoryUsage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		pool := NewPool(2, 10)
		
		// 提交一些任务
		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Add(1)
			pool.AddJob(Job{
				WorkerID: -1,
				Handle: func() error {
					defer wg.Done()
					return nil
				},
			})
		}
		
		wg.Wait()
		pool.Release()
	}
}
