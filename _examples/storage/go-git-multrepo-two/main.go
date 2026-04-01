package main

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"go-git-multrepo-two/multirepo"
	"os"
	"path/filepath"
	"time"
)

type MergeInfo struct {
	RepoName     string
	RepoPath     string
	Repo         *git.Repository
	SourceBranch string
	TargetBranch string
}

func getRepoPath(repoName string) string {
	baseDir, _ := os.Getwd()
	return filepath.Join(baseDir, "repo_temp", repoName)
}

/*
	createBareRepo(repoPath, sourceBranch)
	准备工作（使用命令-版本）
	1.创建分支: git branch dev
	2.提交commit。创建临时工作目录，然后添加文件，并push到裸仓
	mkdir repo_01_temp && cd repo_01_temp && git clone ../repo_01.git  ./
	&& git checkout dev && echo "1">1.txt && git add . && git commit -m "feat: add 1.txt" && git push origin dev
	&& cd .. &&  rm -rf repo_01_temp
*/

func main() {
	var mergeInfo []MergeInfo
	// 创建用于合并的仓库
	for i := 0; i < 2; i++ {
		repoName := fmt.Sprintf("repo_%02d.git", i+1)
		repoPath := getRepoPath(repoName)
		sourceBranch := fmt.Sprintf("s%02d", i+1)
		targetBranch := fmt.Sprintf("t%02d", i+1)

		// 创建裸仓（使用targetBranch作为默认分支）
		_ = os.RemoveAll(repoPath)
		createBareRepo(repoPath, targetBranch)
		createBranch(repoPath, sourceBranch)
		createCommit(repoPath, sourceBranch)

		// 打开仓库
		repo, err := git.PlainOpen(repoPath)
		if err != nil {
			panic(fmt.Sprintf("open repo %s failed: %v", repoName, err))
		}

		mergeInfo = append(mergeInfo, MergeInfo{
			RepoName:     repoName,
			RepoPath:     repoPath,
			SourceBranch: sourceBranch,
			TargetBranch: targetBranch,
			Repo:         repo,
		})
	}

	if err := mergeBranch(mergeInfo); err != nil {
		fmt.Printf("mergeBranch failed: %v\n", err)
	}
}

// bareName的示例："server-mergeInfo.Repo.git"
func createBareRepo(repoPath, branchName string) {
	// 直接创建裸仓库
	repo, err := git.PlainInit(repoPath, true)
	if err != nil {
		panic(fmt.Sprintf("服务端初始化裸仓库失败：%v", err))
	}

	// 创建初始提交对象
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Message:  "Initial commit",
		TreeHash: createEmptyTree(repo),
	}

	// 将提交对象编码并存储到仓库
	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		panic(fmt.Sprintf("编码初始提交对象失败：%v", err))
	}

	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		panic(fmt.Sprintf("存储初始提交对象失败：%v", err))
	}

	// 创建分支引用
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef := plumbing.NewHashReference(branchRefName, commitHash)
	err = repo.Storer.SetReference(branchRef)
	if err != nil {
		panic(fmt.Sprintf("创建分支引用失败：%v", err))
	}

	// 更新HEAD指向指定的分支
	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, branchRefName)
	err = repo.Storer.SetReference(headRef)
	if err != nil {
		panic(fmt.Sprintf("设置HEAD引用失败：%v", err))
	}

	fmt.Printf(" create bare repo success. %s:%s\n", repoPath, branchName)
}

// createEmptyTree 创建空的tree对象
func createEmptyTree(repo *git.Repository) plumbing.Hash {
	// 创建空的tree对象
	tree := &object.Tree{
		Entries: []object.TreeEntry{},
	}

	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.TreeObject)
	if err := tree.Encode(obj); err != nil {
		panic(fmt.Sprintf("编码空tree对象失败：%v", err))
	}

	treeHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		panic(fmt.Sprintf("存储空tree对象失败：%v", err))
	}

	return treeHash
}

func createBranch(repoPath, branchName string) {
	// 1. 打开裸仓
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		panic(fmt.Sprintf("open bare repo %s failed: %v", repoPath, err))
	}

	branchRefName := plumbing.NewBranchReferenceName(branchName)
	// 2. 检查分支是否已存在
	_, err = repo.Reference(branchRefName, true)
	if err == nil {
		fmt.Printf("Branch %s already exists in %s\n", branchName, repoPath)
		return
	}

	// 3. 获取HEAD引用
	headRef, err := repo.Reference(plumbing.HEAD, true)
	if err != nil {
		// HEAD引用不存在，说明仓库是完全空的
		fmt.Printf("HEAD reference not found. Repository is completely empty.\n")
		return
	}

	// 创建分支引用
	branchRef := plumbing.NewHashReference(branchRefName, headRef.Hash())
	err = repo.Storer.SetReference(branchRef)
	if err != nil {
		panic(fmt.Sprintf("创建分支引用失败：%v", err))
	}

	fmt.Printf("created branch success. %s:%s\n", repoPath, branchName)
}

func createCommit(repoPath, branchName string) {
	// 直接打开裸仓库
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		panic(fmt.Sprintf("打开仓库失败：%v", err))
	}

	// 获取分支的引用
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef, err := repo.Reference(branchRefName, true)
	if err != nil {
		panic(fmt.Sprintf("获取分支引用失败：%v", err))
	}

	// 创建一个新的提交对象
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Message:  "feat: 添加 test.txt",
		TreeHash: createBlobInBareRepo(repo, "test.txt", []byte("hello")),
		ParentHashes: []plumbing.Hash{
			branchRef.Hash(),
		},
	}

	// 将提交对象编码并存储到仓库
	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		panic(fmt.Sprintf("编码提交对象失败：%v", err))
	}

	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		panic(fmt.Sprintf("存储提交对象失败：%v", err))
	}

	// 更新分支引用指向新的提交
	newBranchRef := plumbing.NewHashReference(branchRefName, commitHash)
	if err := repo.Storer.SetReference(newBranchRef); err != nil {
		panic(fmt.Sprintf("更新分支引用失败：%v", err))
	}

	fmt.Printf(" create commit success. %s@%s\n", branchName, commitHash)
}

// 在裸仓库中创建blob对象并返回tree hash
func createBlobInBareRepo(repo *git.Repository, filename string, content []byte) plumbing.Hash {
	// 创建blob对象
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)

	writer, err := obj.Writer()
	if err != nil {
		panic(fmt.Sprintf("获取blob writer失败：%v", err))
	}
	defer writer.Close()

	if _, err := writer.Write(content); err != nil {
		panic(fmt.Sprintf("写入blob内容失败：%v", err))
	}

	// 存储blob对象
	blobHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		panic(fmt.Sprintf("存储blob对象失败：%v", err))
	}

	// 创建tree entry
	entry := object.TreeEntry{
		Name: filename,
		Mode: filemode.Regular,
		Hash: blobHash,
	}

	// 创建tree对象
	tree := &object.Tree{
		Entries: []object.TreeEntry{entry},
	}

	obj = repo.Storer.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		panic(fmt.Sprintf("编码tree对象失败：%v", err))
	}

	// 存储tree对象
	treeHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		panic(fmt.Sprintf("存储tree对象失败：%v", err))
	}

	return treeHash
}

// 在裸仓，将多个仓库进行合并
func mergeBranch(mergeInfos []MergeInfo) error {
	ctx := context.Background()
	tx := multirepo.NewMultiRepoTx()

	// 遍历每个仓库，执行合并操作
	for _, mergeInfo := range mergeInfos {
		if err := mergeInTx(tx, mergeInfo); err != nil {
			fmt.Printf("merge repo %s failed: %v\n", mergeInfo.RepoName, err)
			return err
		}
	}

	fmt.Println("\n=== Start transaction prepare ===")
	if err := tx.Prepare(ctx); err != nil {
		// Prepare 失败则回滚
		_ = tx.Rollback(ctx)
		return fmt.Errorf("tx prepare failed: %w", err)
	}

	// ========== 步骤5：两阶段提交 - Commit 原子提交 ==========
	fmt.Println("\n=== Start transaction commit ===")
	if err := tx.Commit(ctx); err != nil {
		// Commit 失败则回滚
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			return fmt.Errorf("commit failed: %w, rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("tx commit failed: %w", err)
	}
	return nil
}

func mergeInTx(tx multirepo.MultiRepoTx, mergeInfo MergeInfo) error {
	// 获取该仓库的事务性存储（写操作落临时存储）
	txStorer, err := tx.AddRepo(mergeInfo.RepoName, mergeInfo.Repo.Storer, mergeInfo.RepoPath)
	if err != nil {
		return fmt.Errorf("add repo %s to tx failed: %w", mergeInfo.RepoName, err)
	}

	// 替换仓库的存储为事务性存储（确保合并操作写临时存储）
	mergeInfo.Repo.Storer = txStorer

	// 执行合并操作：将 source 分支合并到 target
	// 获取 source 分支引用
	sourceRef, err := mergeInfo.Repo.Reference(plumbing.NewBranchReferenceName(mergeInfo.SourceBranch), true)
	if err != nil {
		return fmt.Errorf("get %s branch failed: %w", mergeInfo.SourceBranch, err)
	}

	// 获取 target 分支引用
	targetRef, err := mergeInfo.Repo.Reference(plumbing.NewBranchReferenceName(mergeInfo.TargetBranch), true)
	if err != nil {
		return fmt.Errorf("get %s branch failed: %w", mergeInfo.TargetBranch, err)
	}

	// 获取 source 分支最新提交
	sourceCommit, err := mergeInfo.Repo.CommitObject(sourceRef.Hash())
	if err != nil {
		return fmt.Errorf("get %s commit failed: %w", mergeInfo.SourceBranch, err)
	}

	// 获取 target 分支最新提交
	targetCommit, err := mergeInfo.Repo.CommitObject(targetRef.Hash())
	if err != nil {
		return fmt.Errorf("get %s commit failed: %w", mergeInfo.TargetBranch, err)
	}

	// 计算合并基础
	mergeBases, err := targetCommit.MergeBase(sourceCommit)
	if err != nil {
		return fmt.Errorf("calculate merge base failed: %w", err)
	}

	if len(mergeBases) == 0 {
		return fmt.Errorf("no merge base found")
	}

	// 检查是否可以快进合并
	// 如果 target 是 source 分支的直接祖先，则可以快进合并
	canFastForward, err := targetCommit.IsAncestor(sourceCommit)
	if err != nil {
		return fmt.Errorf("check if %s is ancestor of %s failed: %w", mergeInfo.TargetBranch, mergeInfo.SourceBranch, err)
	}

	if canFastForward {
		// 执行快进合并
		newTargetRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(mergeInfo.TargetBranch), sourceCommit.Hash)
		err = mergeInfo.Repo.Storer.SetReference(newTargetRef)
		if err != nil {
			return fmt.Errorf("fast forward merge failed: %w", err)
		}
		fmt.Printf("Repo %s fast forward merge %s to %s (temp storage) success\n", mergeInfo.RepoName, mergeInfo.SourceBranch, mergeInfo.TargetBranch)
	} else {
		// 执行非快进合并
		// 创建合并提交
		/*
			合并提交创建
			- 手动创建合并提交对象
			- 设置两个父提交（target 和 source 分支）
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
			Message: fmt.Sprintf("Merge %s into %s", mergeInfo.SourceBranch, mergeInfo.TargetBranch),
			ParentHashes: []plumbing.Hash{
				targetCommit.Hash,
				sourceCommit.Hash,
			},
		}

		/*
			存储更新
			- 使用 mergeInfo.Repo.Storer.SetEncodedObject 写入合并提交
			- 使用 mergeInfo.Repo.Storer.SetReference 更新 main 分支引用
		*/
		// 写入合并提交到存储
		o := mergeInfo.Repo.Storer.NewEncodedObject()
		o.SetType(plumbing.CommitObject)
		if err := mergeCommit.Encode(o); err != nil {
			return fmt.Errorf("encode merge commit failed: %w", err)
		}
		mergeCommitHash, err := mergeInfo.Repo.Storer.SetEncodedObject(o)
		if err != nil {
			return fmt.Errorf("write merge commit failed: %w", err)
		}

		// 更新 main 分支引用
		newMasterRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mergeCommitHash)
		err = mergeInfo.Repo.Storer.SetReference(newMasterRef)
		if err != nil {
			return fmt.Errorf("update main reference failed: %w", err)
		}
		fmt.Printf("Repo %s non-fast-forward merge dev to main (temp storage) success\n", mergeInfo.RepoName)
	}

	return nil
}
