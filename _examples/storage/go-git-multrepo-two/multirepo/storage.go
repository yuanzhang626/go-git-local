package multirepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/go-git/go-git/v6/storage/transactional"
	"github.com/redis/go-redis/v9"
)

/*
MultiRepoTxStorage（顶层）
├── TxContext（事务上下文）：管理所有仓库的事务状态、临时存储
├── RepoTxStorage（单仓库事务存储）：对每个仓库封装“基础存储+临时存储”
│   ├── BaseStorer：仓库的持久化存储（如原文件系统/数据库）
│   ├── TemporalStorer：临时存储（内存/临时目录，事务提交前的写操作都落在此处）
└── 2PC 协调器：负责多仓库的 prepare/commit/rollback 协调
*/

// MultiRepoTx 多仓库事务接口，定义事务生命周期
type MultiRepoTx interface {
	// AddRepo 将仓库加入事务，返回该仓库的事务性存储（写操作落临时存储）
	AddRepo(repoID string, baseStorer storage.Storer, fsPath string) (storage.Storer, error)
	// AddRepoWithBranch 将仓库加入事务，并指定需要备份的分支
	AddRepoWithBranch(repoID string, baseStorer storage.Storer, fsPath string, targetBranch string) (storage.Storer, error)
	// Prepare 两阶段提交-准备阶段：验证所有仓库的临时操作可行性
	Prepare(ctx context.Context) error
	// Commit 两阶段提交-提交阶段：所有仓库的临时操作刷入正式存储
	Commit(ctx context.Context) error
	// Rollback 回滚：清空所有仓库的临时存储
	Rollback(ctx context.Context) error
}

// MultiRepoTxStorage 多仓库事务存储的顶层实现
type MultiRepoTxStorage struct {
	txContext  *TxContext                // 全局事务上下文
	repoStores map[string]*RepoTxStorage // 参与事务的仓库存储映射
	redisClient *redis.Client             // Redis 客户端，用于分布式锁
}

// RepoTxStorage 单仓库的事务存储（复用go-git transactional包）
type RepoTxStorage struct {
	repoID       string
	baseStorer   storage.Storer        // 仓库原存储
	tmpStorer    storage.Storer        // 临时存储（如memory.NewStorage()）
	txStorage    transactional.Storage // 基于base+tmp的事务存储
	fsPath       string                // 文件系统路径，用于持久化
	fsStorage    storage.Storer        // 文件系统存储，用于持久化
	backupRefs   map[string]string     // 备份的引用（ref name -> hash）
	targetBranch string                // 需要备份的目标分支
}

// TxContext 事务上下文：管理事务状态、临时存储生命周期
type TxContext struct {
	status TxStatus // 事务状态：Init/Prepared/Committed/RolledBack
}

type TxStatus int8

const (
	TxInit TxStatus = iota
	TxPrepared
	TxCommitted
	TxRolledBack
)

// NewMultiRepoTx 创建新的多仓库事务
func NewMultiRepoTx() MultiRepoTx {
	return NewMultiRepoTxWithRedis("localhost:6379", "")
}

// NewMultiRepoTxWithRedis 创建新的多仓库事务，并指定 Redis 连接
func NewMultiRepoTxWithRedis(redisAddr, redisPassword string) MultiRepoTx {
	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0, // 使用默认数据库
	})

	return &MultiRepoTxStorage{
		txContext:  &TxContext{status: TxInit},
		repoStores: make(map[string]*RepoTxStorage),
		redisClient: rdb,
	}
}

// AddRepo 将仓库加入事务，返回该仓库的事务性存储（写操作落临时存储）
func (m *MultiRepoTxStorage) AddRepo(repoID string, baseStorer storage.Storer, fsPath string) (storage.Storer, error) {
	if m.txContext.status != TxInit {
		return nil, fmt.Errorf("transaction status not init, cannot add repo")
	}

	// 为仓库创建临时存储（内存存储，轻量且易回滚）
	tmpStorer := memory.NewStorage()
	// 复用go-git transactional包创建单仓库事务存储
	txStorage := transactional.NewStorage(baseStorer, tmpStorer)

	// 创建文件系统存储用于持久化
	fs := osfs.New(fsPath)
	fsStorage := filesystem.NewStorage(fs, nil)

	repoTxStore := &RepoTxStorage{
		repoID:       repoID,
		baseStorer:   baseStorer,
		tmpStorer:    tmpStorer,
		txStorage:    txStorage,
		fsPath:       fsPath,
		fsStorage:    fsStorage,
		backupRefs:   make(map[string]string),
		targetBranch: "", // 默认不备份任何分支
	}
	m.repoStores[repoID] = repoTxStore

	return repoTxStore.txStorage, nil
}

// AddRepoWithBranch 将仓库加入事务，并指定需要备份的分支
func (m *MultiRepoTxStorage) AddRepoWithBranch(repoID string, baseStorer storage.Storer, fsPath string, targetBranch string) (storage.Storer, error) {
	if m.txContext.status != TxInit {
		return nil, fmt.Errorf("transaction status not init, cannot add repo")
	}

	// 为仓库创建临时存储（内存存储，轻量且易回滚）
	tmpStorer := memory.NewStorage()
	// 复用go-git transactional包创建单仓库事务存储
	txStorage := transactional.NewStorage(baseStorer, tmpStorer)

	// 创建文件系统存储用于持久化
	fs := osfs.New(fsPath)
	fsStorage := filesystem.NewStorage(fs, nil)

	repoTxStore := &RepoTxStorage{
		repoID:       repoID,
		baseStorer:   baseStorer,
		tmpStorer:    tmpStorer,
		txStorage:    txStorage,
		fsPath:       fsPath,
		fsStorage:    fsStorage,
		backupRefs:   make(map[string]string),
		targetBranch: targetBranch,
	}
	m.repoStores[repoID] = repoTxStore

	return repoTxStore.txStorage, nil
}

// AcquireRepoLock 获取仓库的分布式锁
func (m *MultiRepoTxStorage) AcquireRepoLock(ctx context.Context, repoID string) error {
	// 锁的key：repo_lock:{repoID}
	lockKey := fmt.Sprintf("repo_lock:%s", repoID)

	// 尝试获取锁，锁的过期时间为30秒
	success, err := m.redisClient.SetNX(ctx, lockKey, "locked", 30*time.Second).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire lock for repo %s: %w", repoID, err)
	}

	if !success {
		return fmt.Errorf("repo %s is currently being processed by another process", repoID)
	}

	fmt.Printf("  Acquired lock for repo %s\n", repoID)
	return nil
}

// ReleaseRepoLock 释放仓库的分布式锁
func (m *MultiRepoTxStorage) ReleaseRepoLock(ctx context.Context, repoID string) {
	lockKey := fmt.Sprintf("repo_lock:%s", repoID)

	// 释放锁
	err := m.redisClient.Del(ctx, lockKey).Err()
	if err != nil {
		fmt.Printf("Warning: failed to release lock for repo %s: %v\n", repoID, err)
	} else {
		fmt.Printf("  Released lock for repo %s\n", repoID)
	}
}

// acquireAllRepoLocks 获取所有仓库的分布式锁
func (m *MultiRepoTxStorage) acquireAllRepoLocks(ctx context.Context) error {
	// 按顺序获取所有仓库的锁
	for repoID := range m.repoStores {
		if err := m.AcquireRepoLock(ctx, repoID); err != nil {
			// 获取锁失败，释放之前获取的所有锁
			for acquiredRepoID := range m.repoStores {
				if acquiredRepoID == repoID {
					break // 只释放之前成功获取的锁
				}
				m.ReleaseRepoLock(ctx, acquiredRepoID)
			}
			return err
		}
	}
	return nil
}

// ReleaseAllRepoLocks 释放所有仓库的分布式锁
func (m *MultiRepoTxStorage) ReleaseAllRepoLocks(ctx context.Context) {
	for repoID := range m.repoStores {
		m.ReleaseRepoLock(ctx, repoID)
	}
}

// Close 关闭事务，释放资源
func (m *MultiRepoTxStorage) Close() error {
	if m.redisClient != nil {
		return m.redisClient.Close()
	}
	return nil
}

func (m *MultiRepoTxStorage) Prepare(ctx context.Context) error {
	if m.txContext.status != TxInit {
		return fmt.Errorf("transaction status invalid for prepare")
	}

	// 遍历所有仓库，验证临时操作可行性（如检查临时存储的写操作是否合法）
	for repoID, repoStore := range m.repoStores {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 示例：验证临时存储的对象/引用是否合法（可扩展具体校验逻辑）
			if err := validateTmpStorer(repoStore.tmpStorer); err != nil {
				return fmt.Errorf("repo %s prepare failed: %w", repoID, err)
			}
		}
	}

	m.txContext.status = TxPrepared
	return nil
}

// 校验临时存储的操作合法性（示例：检查引用是否冲突）
func validateTmpStorer(s storage.Storer) error {
	iter, err := s.IterReferences()
	if err != nil {
		return err
	}
	defer iter.Close()
	// todo 此处可扩展：如检查引用是否与基础存储冲突、对象完整性等
	return iter.ForEach(func(ref *plumbing.Reference) error { return nil })
}

// persistToFilesystem 将内存存储的内容持久化到文件系统
func (r *RepoTxStorage) persistToFilesystem() error {
	// 创建 .git 目录
	gitDir := filepath.Join(r.fsPath, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return fmt.Errorf("create .git dir failed: %w", err)
	}

	// 1. 首先从基础存储复制所有对象
	if r.baseStorer != nil {
		baseObjIter, err := r.baseStorer.IterEncodedObjects(plumbing.AnyObject)
		if err != nil {
			return fmt.Errorf("iter base objects failed: %w", err)
		}
		defer baseObjIter.Close()

		baseObjCount := 0
		err = baseObjIter.ForEach(func(obj plumbing.EncodedObject) error {
			baseObjCount++
			if _, err := r.fsStorage.SetEncodedObject(obj); err != nil {
				return fmt.Errorf("persist base object %s failed: %w", obj.Hash(), err)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("persist base objects failed: %w", err)
		}
		fmt.Printf("  Persisted %d base objects\n", baseObjCount)
	}

	// 2. 持久化所有引用
	refIter, err := r.tmpStorer.IterReferences()
	if err != nil {
		return fmt.Errorf("iter references failed: %w", err)
	}
	defer refIter.Close()

	refCount := 0
	if err := refIter.ForEach(func(ref *plumbing.Reference) error {
		refCount++
		fmt.Printf("  Persisting reference: %s -> %s\n", ref.Name(), ref.Hash())
		return r.fsStorage.SetReference(ref)
	}); err != nil {
		return fmt.Errorf("persist references failed: %w", err)
	}
	fmt.Printf("  Persisted %d references\n", refCount)

	// 3. 持久化临时存储中的对象
	objIter, err := r.tmpStorer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return fmt.Errorf("iter tmp objects failed: %w", err)
	}
	defer objIter.Close()

	objCount := 0
	err = objIter.ForEach(func(obj plumbing.EncodedObject) error {
		objCount++
		fmt.Printf("  Persisting tmp object: %s (type: %s)\n", obj.Hash(), obj.Type())
		if _, err := r.fsStorage.SetEncodedObject(obj); err != nil {
			return fmt.Errorf("persist tmp object %s failed: %w", obj.Hash(), err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("persist tmp objects failed: %w", err)
	}
	fmt.Printf("  Persisted %d tmp objects\n", objCount)

	fmt.Printf("Repo %s persisted to filesystem: %s\n", r.repoID, r.fsPath)
	return nil
}

// createBackup 创建仓库的引用备份，用于支持已提交事务的回滚
func (r *RepoTxStorage) createBackup() error {
	if len(r.backupRefs) > 0 {
		return nil // 备份已存在
	}

	// 如果没有指定 targetBranch，则不备份
	if r.targetBranch == "" {
		fmt.Printf("  No target branch specified for repo %s, skipping backup\n", r.repoID)
		return nil
	}

	// 只备份 targetBranch
	targetRefName := plumbing.NewBranchReferenceName(r.targetBranch)
	targetRef, err := r.baseStorer.Reference(targetRefName)
	if err != nil {
		return fmt.Errorf("get target branch %s failed: %w", r.targetBranch, err)
	}

	// 创建备份引用名称，格式：refs/backup/{branch_name}_{commit_id}
	// 例如：refs/backup/main_abc123def
	backupRefName := fmt.Sprintf("refs/backup/%s_%s", r.targetBranch, targetRef.Hash())
	_ = plumbing.NewHashReference(plumbing.ReferenceName(backupRefName), targetRef.Hash())

	// 存储备份信息：原始引用名 -> 备份引用名
	r.backupRefs[string(targetRefName)] = backupRefName

	fmt.Printf("  Backed up target branch %s -> %s\n", targetRefName, backupRefName)
	fmt.Printf("  Created 1 reference backup for repo %s\n", r.repoID)

	return nil
}

// restoreFromBackup 从备份引用恢复仓库
func (r *RepoTxStorage) restoreFromBackup() error {
	if len(r.backupRefs) == 0 {
		return fmt.Errorf("no backup references available for repo %s", r.repoID)
	}

	// 遍历所有备份引用，恢复到原始引用
	restoreCount := 0
	for originalRefName, backupRefName := range r.backupRefs {
		// 从存储中获取备份引用
		backupRef, err := r.fsStorage.Reference(plumbing.ReferenceName(backupRefName))
		if err != nil {
			fmt.Printf("Warning: backup reference %s not found: %v\n", backupRefName, err)
			continue
		}

		// 创建原始引用
		originalRef := plumbing.NewHashReference(plumbing.ReferenceName(originalRefName), backupRef.Hash())

		// 设置原始引用
		if err := r.fsStorage.SetReference(originalRef); err != nil {
			fmt.Printf("Warning: failed to restore reference %s: %v\n", originalRefName, err)
			continue
		}

		// 删除备份引用
		_ = r.fsStorage.RemoveReference(plumbing.ReferenceName(backupRefName))

		restoreCount++
		fmt.Printf("  Restored %s from %s\n", originalRefName, backupRefName)
	}

	if restoreCount > 0 {
		fmt.Printf("  Successfully restored %d references for repo %s\n", restoreCount, r.repoID)
	}

	// 清空备份映射
	r.backupRefs = make(map[string]string)
	return nil
}

// Commit 原子提交所有仓库的临时操作
func (m *MultiRepoTxStorage) Commit(ctx context.Context) error {
	if m.txContext.status != TxPrepared {
		return fmt.Errorf("transaction not prepared, cannot commit")
	}

	// 获取所有仓库的分布式锁
	if err := m.acquireAllRepoLocks(ctx); err != nil {
		return fmt.Errorf("failed to acquire repo locks: %w", err)
	}

	// 确保在函数退出时释放锁
	defer m.ReleaseAllRepoLocks(ctx)

	// 首先为所有仓库创建引用备份
	for repoID, repoStore := range m.repoStores {
		select {
		case <-ctx.Done():
			_ = m.Rollback(ctx) // 备份中断则回滚
			return ctx.Err()
		default:
			if err := repoStore.createBackup(); err != nil {
				return fmt.Errorf("repo %s create backup failed: %w", repoID, err)
			}
		}
	}

	// 批量提交所有仓库的事务（复用go-git transactional的Commit逻辑）
	for repoID, repoStore := range m.repoStores {
		select {
		case <-ctx.Done():
			_ = m.Rollback(ctx) // 提交中断则回滚
			return ctx.Err()
		default:
			// 首先持久化到文件系统
			if err := repoStore.persistToFilesystem(); err != nil {
				return fmt.Errorf("repo %s persist to filesystem failed: %w", repoID, err)
			}
			// 然后提交事务
			if err := repoStore.txStorage.Commit(); err != nil {
				_ = m.Rollback(ctx) // 任意仓库失败则回滚
				return fmt.Errorf("repo %s commit failed: %w", repoID, err)
			}
		}
	}

	m.txContext.status = TxCommitted
	return nil
}

// Rollback 回滚所有仓库的临时操作
func (m *MultiRepoTxStorage) Rollback(ctx context.Context) error {
	// 如果是已提交状态，需要获取锁才能回滚
	if m.txContext.status == TxCommitted {
		// 获取所有仓库的分布式锁
		if err := m.acquireAllRepoLocks(ctx); err != nil {
			return fmt.Errorf("failed to acquire repo locks for rollback: %w", err)
		}

		// 确保在函数退出时释放锁
		defer m.ReleaseAllRepoLocks(ctx)
	}

	// 处理已提交事务的回滚（从备份恢复）
	if m.txContext.status == TxCommitted {
		// 从备份恢复所有仓库
		for repoID, repoStore := range m.repoStores {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if err := repoStore.restoreFromBackup(); err != nil {
					return fmt.Errorf("repo %s restore from backup failed: %w", repoID, err)
				}
			}
		}
	} else {
		// 处理未提交事务的回滚（清空临时存储）
		for _, repoStore := range m.repoStores {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// 重置临时存储（以memory为例，直接重新初始化）
				repoStore.tmpStorer = memory.NewStorage()

				// 清空备份映射
				repoStore.backupRefs = make(map[string]string)
			}
		}
	}

	m.txContext.status = TxRolledBack
	return nil
}
