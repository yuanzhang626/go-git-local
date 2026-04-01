package main

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"go-git-multrepo-two/multirepo"
	"os"
	"path/filepath"
	"time"
)

func main() {
	createBareRepo("repo_01.git")
	createBareRepo("repo_02.git")
	/*
		准备工作
		1.创建分支: git branch dev
		2.提交commit。创建临时工作目录，然后添加文件，并push到裸仓
		mkdir repo_01_temp && cd repo_01_temp && git clone ../repo_01.git  ./
		&& git checkout dev && echo "1">1.txt && git add . && git commit -m "feat: add 1.txt" && git push origin dev
		&& cd .. &&  rm -rf repo_01_temp
	*/

	//裸仓路径：repo_temp/repo_01.git  repo_temp/repo_02.git等等
	//每个裸仓包含master|dev分支
	//合并操作为，从dev到master操作
	_ = mergeBranch([]string{"repo_01", "repo_02"})
}

// bareName的示例："server-repo.git"
func createBareRepo(bareName string) {
	// 1. 定义目录路径
	baseDir, _ := os.Getwd()
	serverRepoPath := filepath.Join(baseDir, "repo_temp", bareName)

	// 清空旧目录（方便重复测试）
	_ = os.RemoveAll(serverRepoPath)

	fmt.Println("=== 服务端：初始化裸仓库 ===")
	// 首先创建一个普通仓库来初始化
	tempRepoPath := serverRepoPath + "-temp"
	_ = os.RemoveAll(tempRepoPath)

	// 初始化普通仓库
	tempRepo, err := git.PlainInit(tempRepoPath, false)
	if err != nil {
		panic(fmt.Sprintf("初始化临时仓库失败：%v", err))
	}

	// 创建初始提交
	worktree, err := tempRepo.Worktree()
	if err != nil {
		panic(fmt.Sprintf("获取工作树失败：%v", err))
	}

	// 创建README文件
	readmeFile := filepath.Join(tempRepoPath, "README.md")
	err = os.WriteFile(readmeFile, []byte("# 仓库\n\n这是个代码仓库."), 0644)
	if err != nil {
		panic(fmt.Sprintf("创建README失败：%v", err))
	}

	// 添加文件
	_, err = worktree.Add("README.md")
	if err != nil {
		panic(fmt.Sprintf("添加文件失败：%v", err))
	}

	// 提交
	_, err = worktree.Commit("feat: 初始化", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
		},
	})
	if err != nil {
		panic(fmt.Sprintf("初始提交失败：%v", err))
	}

	// 现在创建裸仓库并推送
	_ = os.RemoveAll(serverRepoPath)
	_, err = git.PlainInit(serverRepoPath, true)
	if err != nil {
		panic(fmt.Sprintf("服务端初始化裸仓库失败：%v", err))
	}

	// 将临时仓库推送到裸仓库
	// 首先添加远程仓库
	_, err = tempRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{serverRepoPath},
	})
	if err != nil {
		panic(fmt.Sprintf("创建远程仓库失败：%v", err))
	}

	err = tempRepo.Push(&git.PushOptions{
		RemoteName: "origin",
	})
	if err != nil {
		panic(fmt.Sprintf("推送到裸仓库失败：%v", err))
	}

	// 清理临时仓库
	_ = os.RemoveAll(tempRepoPath)

	fmt.Println(" 服务端裸仓库创建完成：", serverRepoPath)
}

// 在裸仓，将多个仓库进行合并
func mergeBranch(repos []string) error {
	ctx := context.Background()
	tx := multirepo.NewMultiRepoTx()

	for _, repo := range repos {
		if err := mergeInTx(tx, repo, nil, ""); err != nil {
			fmt.Printf("merge repo1 failed: %v\n", err)
		}
	}
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
	return nil
}

func mergeInTx(tx multirepo.MultiRepoTx, repoID string, repo *git.Repository, workdir string) error {
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
