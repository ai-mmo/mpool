package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ai-mmo/mpool"
)

// 基本使用示例
func basicPoolExample() {
	fmt.Println("=== 基本协程池使用示例 ===")

	// 创建协程池：4个工作协程，每个协程队列长度为10
	pool := mpool.NewPool(4, 10)
	defer pool.Release()

	var wg sync.WaitGroup

	// 提交10个任务
	for i := 0; i < 10; i++ {
		wg.Add(1)
		taskID := i

		err := pool.AddJob(mpool.Job{
			WorkerID: -1, // -1表示随机分配工作协程
			Handle: func() error {
				defer wg.Done()

				// 模拟任务处理
				fmt.Printf("处理任务 %d，工作协程: %d\n", taskID, taskID%4)
				time.Sleep(time.Duration(100+taskID*50) * time.Millisecond)

				// 模拟部分任务失败
				if taskID%7 == 0 {
					return fmt.Errorf("任务 %d 处理失败", taskID)
				}

				fmt.Printf("任务 %d 处理完成\n", taskID)
				return nil
			},
		})

		if err != nil {
			fmt.Printf("添加任务 %d 失败: %v\n", taskID, err)
			wg.Done()
		}
	}

	// 等待所有任务完成
	wg.Wait()
	fmt.Println("所有任务处理完成")
}

// 带结果返回的任务示例
func jobWithResultExample() {
	fmt.Println("\n=== 带结果返回的任务示例 ===")

	pool := mpool.NewPool(2, 5)
	defer pool.Release()

	// 提交5个带结果的任务
	resultChans := make([]<-chan error, 5)

	for i := 0; i < 5; i++ {
		taskID := i
		resultChan := pool.AddJobWithResult(mpool.Job{
			WorkerID: taskID % 2, // 指定工作协程
			Handle: func() error {
				fmt.Printf("执行计算任务 %d\n", taskID)
				time.Sleep(time.Duration(200+taskID*100) * time.Millisecond)

				// 模拟计算结果
				if taskID%3 == 0 {
					return fmt.Errorf("计算任务 %d 出错", taskID)
				}

				fmt.Printf("计算任务 %d 完成\n", taskID)
				return nil
			},
		})
		resultChans[i] = resultChan
	}

	// 收集所有任务结果
	for i, resultChan := range resultChans {
		select {
		case err := <-resultChan:
			if err != nil {
				fmt.Printf("任务 %d 结果: 失败 - %v\n", i, err)
			} else {
				fmt.Printf("任务 %d 结果: 成功\n", i)
			}
		case <-time.After(2 * time.Second):
			fmt.Printf("任务 %d 结果: 超时\n", i)
		}
	}
}

// 指定工作协程示例
func specificWorkerExample() {
	fmt.Println("\n=== 指定工作协程示例 ===")

	pool := mpool.NewPool(3, 5)
	defer pool.Release()

	var wg sync.WaitGroup

	// 为每个工作协程分配特定任务
	for workerID := 0; workerID < 3; workerID++ {
		for taskNum := 0; taskNum < 3; taskNum++ {
			wg.Add(1)
			currentWorkerID := workerID
			currentTaskNum := taskNum

			err := pool.AddJob(mpool.Job{
				WorkerID: currentWorkerID, // 指定特定的工作协程
				Handle: func() error {
					defer wg.Done()

					fmt.Printf("工作协程 %d 处理任务 %d\n", currentWorkerID, currentTaskNum)
					time.Sleep(100 * time.Millisecond)

					return nil
				},
			})

			if err != nil {
				fmt.Printf("添加任务失败: %v\n", err)
				wg.Done()
			}
		}
	}

	wg.Wait()
	fmt.Println("指定工作协程任务完成")
}

// 超时控制示例
func timeoutExample() {
	fmt.Println("\n=== 超时控制示例 ===")

	pool := mpool.NewPool(2, 3)

	var wg sync.WaitGroup

	// 提交一些长时间运行的任务
	for i := 0; i < 3; i++ {
		wg.Add(1)
		taskID := i

		pool.AddJob(mpool.Job{
			WorkerID: -1,
			Handle: func() error {
				defer wg.Done()

				fmt.Printf("开始长时间任务 %d\n", taskID)
				time.Sleep(2 * time.Second) // 模拟长时间任务
				fmt.Printf("长时间任务 %d 完成\n", taskID)

				return nil
			},
		})
	}

	// 设置超时释放协程池
	go func() {
		time.Sleep(1 * time.Second)
		fmt.Println("开始超时释放协程池...")
		pool.ReleaseWithTimeout(500 * time.Millisecond)
	}()

	wg.Wait()
	fmt.Println("超时控制示例完成")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 运行各种示例
	basicPoolExample()
	jobWithResultExample()
	specificWorkerExample()
	timeoutExample()

	fmt.Println("\n=== 所有示例运行完成 ===")
}
