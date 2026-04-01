# 多仓库事务合并（带 Redis 分布式锁）

本项目实现了多 Git 仓库的事务性合并操作，支持两阶段提交和分布式锁，确保在多个程序并发执行时不会出现冲突。

## 功能特性

1. **多仓库事务管理**：支持将多个仓库的合并操作放在一个事务中
2. **两阶段提交**：Prepare 和 Commit 阶段，确保原子性
3. **回滚支持**：支持已提交和未提交事务的回滚
4. **分布式锁**：使用 Redis 锁防止并发冲突
5. **精确备份**：只备份需要合并的目标分支

## 使用方法

### 基本用法

```go
// 创建带 Redis 锁的事务
tx := multirepo.NewMultiRepoTxWithRedis("localhost:6379", "")
defer tx.Close()

// 添加仓库到事务，指定需要备份的目标分支
txStorer, err := tx.AddRepoWithBranch(repoName, storer, repoPath, targetBranch)
if err != nil {
    return err
}

// 执行合并操作...

// 准备阶段
if err := tx.Prepare(ctx); err != nil {
    tx.Rollback(ctx)
    return err
}

// 提交阶段（会自动获取和释放 Redis 锁）
if err := tx.Commit(ctx); err != nil {
    tx.Rollback(ctx)
    return err
}
```

### Redis 锁机制

- 每个仓库在 Commit 前会获取一个分布式锁
- 锁的 key 格式：`repo_lock:{repoID}`
- 锁的过期时间：30 秒
- 如果获取锁失败，说明有其他进程正在处理该仓库

### 并发控制

当多个进程同时尝试合并同一个仓库时：

1. 第一个进程成功获取锁，继续执行
2. 其他进程获取锁失败，返回错误
3. 第一个进程完成提交后自动释放锁

## 配置

### Redis 配置

默认连接 `localhost:6379`，可以通过以下方式自定义：

```go
tx := multirepo.NewMultiRepoTxWithRedis("redis.example.com:6379", "password")
```

### 锁的超时时间

当前锁的超时时间固定为 30 秒，可以根据需要调整代码中的 `30*time.Second`。

## 注意事项

1. 确保 Redis 服务正在运行
2. 网络问题可能导致锁无法释放，需要手动清理
3. 锁的粒度是仓库级别，同一仓库的不同分支会互相阻塞
4. 建议在finally块中调用Close()释放Redis连接

## 示例

运行 main.go 可以看到一个完整的多仓库合并示例：

```bash
go run main.go
```

该示例会：
1. 创建两个裸仓库
2. 在每个仓库中创建源分支和目标分支
3. 使用事务将源分支合并到目标分支
4. 整个过程由 Redis 锁保护，确保并发安全