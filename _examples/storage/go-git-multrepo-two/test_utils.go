package main

import (
	"context"
	"time"
	"go-git-multrepo-two/multirepo"
)

// checkRedisAvailable 检查 Redis 是否可用
func checkRedisAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb := multirepo.NewMultiRepoTxWithRedis("localhost:6379", "")
	if closer, ok := rdb.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	// 尝试执行一个简单的 PING 命令
	// 由于我们的接口没有直接暴露 Redis 客户端，我们尝试获取锁来判断
	tx, ok := rdb.(*multirepo.MultiRepoTxStorage)
	if !ok {
		return false
	}

	err := tx.AcquireRepoLock(ctx, "test-redis-check")
	if err != nil {
		return false
	}

	// 释放锁
	tx.ReleaseRepoLock(ctx, "test-redis-check")
	return true
}