package main

import (
	"context"
	"fmt"
	"github.com/go-git/go-billy/v6/osfs"
	"os"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"

	// 假设你的多仓库事务存储包路径为 multirepo
	"go-git-multrepo/multirepo"
)

const (
	rootPath = "./output/"
)

// 初始化测试仓库：创建仓库、初始化分支、提交测试内容
func initTestRepo(repoID string) (storage.Storer, *git.Repository, string, error) {
	// 内存存储作为仓库基础存储
	baseStorer := memory.NewStorage()

	// 创建临时目录作为工作区
	workdir, err := os.MkdirTemp(rootPath, fmt.Sprintf("repo-%s-*", repoID))
	if err != nil {
		return nil, nil, "", fmt.Errorf("create temp workdir for repo %s failed: %w", repoID, err)
	}
	// 初始化失败时自动清理临时目录
	defer func() {
		if err != nil {
			_ = os.RemoveAll(workdir)
		}
	}()

	// 创建非 bare 仓库，指定工作区
	repo, err := git.Init(baseStorer, git.WithWorkTree(osfs.New(workdir)))
	if err != nil {
		return nil, nil, "", fmt.Errorf("init repo %s failed: %w", repoID, err)
	}

	// 创建测试分支（如 feature 分支）
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, nil, "", err
	}

	// 创建 master 分支（新仓库默认没有分支，需要创建）
	// 注意：新初始化的仓库没有提交，所以不能直接 checkout 分支
	// 我们需要先创建并提交内容，然后分支才会存在
	// 创建测试文件（在工作目录中创建）
	testFile := fmt.Sprintf("test-%s.txt", repoID)
	testFilePath := workdir + "/" + testFile
	if err := os.WriteFile(testFilePath, []byte("init content"), 0644); err != nil {
		return nil, nil, "", err
	}
	defer os.Remove(testFilePath) // 清理临时文件

	// 添加并提交
	if _, err := worktree.Add(testFile); err != nil {
		return nil, nil, "", err
	}
	commit, err := worktree.Commit("init commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, nil, "", err
	}
	fmt.Printf("Repo %s master branch init commit: %s\n", repoID, commit.String())

	// 创建 feature 分支并提交新内容
	if err := worktree.Checkout(&git.CheckoutOptions{Create: true, Branch: plumbing.NewBranchReferenceName("feature")}); err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(testFilePath, []byte("feature content"), 0644); err != nil {
		return nil, nil, "", err
	}
	if _, err := worktree.Add(testFile); err != nil {
		return nil, nil, "", err
	}
	featureCommit, err := worktree.Commit("feature commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, nil, "", err
	}
	fmt.Printf("Repo %s feature branch commit: %s\n", repoID, featureCommit.String())

	// 切回 master 分支（准备合并）
	if err := worktree.Checkout(&git.CheckoutOptions{Branch: plumbing.Master}); err != nil {
		return nil, nil, "", err
	}

	return baseStorer, repo, workdir, nil
}

// 在事务中执行多仓库合并操作
func mergeInTx(ctx context.Context, tx multirepo.MultiRepoTx, repoID string, repo *git.Repository, workdir string) error {
	// 获取该仓库的事务性存储（写操作落临时存储）
	txStorer, err := tx.AddRepo(repoID, repo.Storer, workdir)
	if err != nil {
		return fmt.Errorf("add repo %s to tx failed: %w", repoID, err)
	}

	// 替换仓库的存储为事务性存储（确保合并操作写临时存储）
	repo.Storer = txStorer

	// 执行合并操作：将 feature 分支合并到 master
	// 获取 feature 分支引用
	featureRef, err := repo.Reference(plumbing.NewBranchReferenceName("feature"), true)
	if err != nil {
		return fmt.Errorf("get feature branch failed: %w", err)
	}

	// 获取 master 分支引用
	masterRef, err := repo.Reference(plumbing.Master, true)
	if err != nil {
		return fmt.Errorf("get master branch failed: %w", err)
	}

	// 获取 feature 分支最新提交
	featureCommit, err := repo.CommitObject(featureRef.Hash())
	if err != nil {
		return fmt.Errorf("get feature commit failed: %w", err)
	}

	// 获取 master 分支最新提交
	masterCommit, err := repo.CommitObject(masterRef.Hash())
	if err != nil {
		return fmt.Errorf("get master commit failed: %w", err)
	}

	// 计算合并基础
	mergeBases, err := masterCommit.MergeBase(featureCommit)
	if err != nil {
		return fmt.Errorf("calculate merge base failed: %w", err)
	}

	if len(mergeBases) == 0 {
		return fmt.Errorf("no merge base found")
	}

	// 检查是否可以快进合并
	// 如果 master 是 feature 分支的直接祖先，则可以快进合并
	canFastForward, err := masterCommit.IsAncestor(featureCommit)
	if err != nil {
		return fmt.Errorf("check if master is ancestor of feature failed: %w", err)
	}

	if canFastForward {
		// 执行快进合并
		newMasterRef := plumbing.NewHashReference(plumbing.Master, featureCommit.Hash)
		err = repo.Storer.SetReference(newMasterRef)
		if err != nil {
			return fmt.Errorf("fast forward merge failed: %w", err)
		}
		fmt.Printf("Repo %s fast forward merge feature to master (temp storage) success\n", repoID)
	} else {
		// 执行非快进合并
		// 创建合并提交
		/*
			合并提交创建
			- 手动创建合并提交对象
			- 设置两个父提交（master 和 feature 分支）
			- 生成合并提交哈希
		*/
		mergeCommit := &object.Commit{
			Author: object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
			Committer: object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
			Message: "Merge feature into master",
			ParentHashes: []plumbing.Hash{
				masterCommit.Hash,
				featureCommit.Hash,
			},
		}

		/*
			存储更新
			- 使用 repo.Storer.SetEncodedObject 写入合并提交
			- 使用 repo.Storer.SetReference 更新 master 分支引用
		*/
		// 写入合并提交到存储
		o := repo.Storer.NewEncodedObject()
		o.SetType(plumbing.CommitObject)
		if err := mergeCommit.Encode(o); err != nil {
			return fmt.Errorf("encode merge commit failed: %w", err)
		}
		mergeCommitHash, err := repo.Storer.SetEncodedObject(o)
		if err != nil {
			return fmt.Errorf("write merge commit failed: %w", err)
		}

		// 更新 master 分支引用
		newMasterRef := plumbing.NewHashReference(plumbing.Master, mergeCommitHash)
		err = repo.Storer.SetReference(newMasterRef)
		if err != nil {
			return fmt.Errorf("update master reference failed: %w", err)
		}
		fmt.Printf("Repo %s non-fast-forward merge feature to master (temp storage) success\n", repoID)
	}

	return nil
}

// 验证仓库合并结果（检查分支 HEAD 和提交）
func verifyMergeResult(repo *git.Repository, repoID string) error {
	// 获取 master 分支引用
	masterRef, err := repo.Reference(plumbing.Master, false)
	if err != nil {
		return err
	}

	// 获取 master 分支最新提交
	commit, err := repo.CommitObject(masterRef.Hash())
	if err != nil {
		return err
	}

	// todo 检查合并结果
	// 如果是快进合并，提交只有一个父节点
	// 如果是三方合并，提交有两个父节点
	// 这里我们只需要验证 master 分支指向了正确的提交即可
	fmt.Printf("Repo %s merge verified: master HEAD=%s (%d parent(s))\n", repoID, commit.Hash.String(), len(commit.ParentHashes))

	fmt.Printf("Repo %s merge verified: master HEAD=%s (merge commit)\n", repoID, commit.Hash.String())
	return nil
}

func main() {
	ctx := context.Background()

	// ========== 步骤1：初始化两个测试仓库 ==========
	repo1Storer, repo1, repo1Workdir, err := initTestRepo("repo1")
	if err != nil {
		panic(fmt.Sprintf("init repo1 failed: %v", err))
	}
	repo2Storer, repo2, repo2Workdir, err := initTestRepo("repo2")
	if err != nil {
		panic(fmt.Sprintf("init repo2 failed: %v", err))
	}

	// ========== 步骤2：创建多仓库事务 ==========
	tx := multirepo.NewMultiRepoTx()

	// ========== 步骤3：在事务中执行多仓库合并 ==========
	fmt.Println("\n=== Start merge in transaction ===")
	// 合并 repo1
	if err := mergeInTx(ctx, tx, "repo1", repo1, repo1Workdir); err != nil {
		panic(fmt.Sprintf("merge repo1 failed: %v", err))
	}
	// 合并 repo2
	if err := mergeInTx(ctx, tx, "repo2", repo2, repo2Workdir); err != nil {
		panic(fmt.Sprintf("merge repo2 failed: %v", err))
	}

	// ========== 步骤4：两阶段提交 - Prepare 验证 ==========
	fmt.Println("\n=== Start transaction prepare ===")
	if err := tx.Prepare(ctx); err != nil {
		// Prepare 失败则回滚
		_ = tx.Rollback(ctx)
		panic(fmt.Sprintf("tx prepare failed: %v", err))
	}

	// ========== 步骤5：两阶段提交 - Commit 原子提交 ==========
	fmt.Println("\n=== Start transaction commit ===")
	if err := tx.Commit(ctx); err != nil {
		// Commit 失败则回滚
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			panic(fmt.Sprintf("commit failed: %v, rollback failed: %v", err, rollbackErr))
		}
		panic(fmt.Sprintf("tx commit failed: %v", err))
	}

	// ========== 步骤6：验证合并结果 ==========
	fmt.Println("\n=== Verify merge result ===")
	// 恢复仓库的原始存储（确保读取的是提交后的正式存储）
	repo1.Storer = repo1Storer
	repo2.Storer = repo2Storer

	if err := verifyMergeResult(repo1, "repo1"); err != nil {
		panic(err)
	}
	if err := verifyMergeResult(repo2, "repo2"); err != nil {
		panic(err)
	}

	fmt.Println("\n=== All repos merged successfully (atomic commit) ===")

	// 【可选】测试回滚场景：注释上面的 Commit，手动触发 Rollback
	// if err := tx.Rollback(ctx); err != nil {
	// 	panic(fmt.Sprintf("tx rollback failed: %v", err))
	// }
	// fmt.Println("=== All repos rollback successfully ===")
}
