package main

import (
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"os"
	"path/filepath"
	"time"
)

var (
	serverRepoPath string
)

func main() {
	createBareRepo()

	createBranch("feature/new")

	createCommit("feature/new")

	mergeBranch("feature/new")
}

// 创建一个裸仓库
func createBareRepo() {
	// 1. 定义目录路径
	baseDir, _ := os.Getwd()
	serverRepoPath = filepath.Join(baseDir, "server-repo.git") // 服务端裸仓库

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

// 创建一个分支  git branch feature/new
func createBranch(branchName string) {
	fmt.Printf("=== 创建分支：%s ===\n", branchName)

	// 打开裸仓库
	repo, err := git.PlainOpen(serverRepoPath)
	if err != nil {
		panic(fmt.Sprintf("打开仓库失败：%v", err))
	}

	// 获取HEAD引用
	headRef, err := repo.Head()
	if err != nil {
		panic(fmt.Sprintf("获取HEAD引用失败：%v", err))
	}

	// 创建分支引用
	branchRefName := plumbing.NewBranchReferenceName(branchName)

	// 直接创建引用
	branchRef := plumbing.NewHashReference(branchRefName, headRef.Hash())
	err = repo.Storer.SetReference(branchRef)
	if err != nil {
		panic(fmt.Sprintf("创建分支引用失败：%v", err))
	}

	fmt.Printf(" 分支 %s 创建成功\n", branchName)
}

/*
  1. 直接打开裸仓库 - 使用 git.PlainOpen() 直接访问裸仓库
  2. 创建 blob 对象 - 将 "hello" 内容作为 blob 存储到仓库
  3. 创建 tree 对象 - 创建包含 test.txt 的 tree
  4. 创建 commit 对象 - 创建提交对象，设置作者、提交者、消息等信息
  5. 存储所有对象 - 将 blob、tree 和 commit 对象存储到仓库
  6. 更新分支引用 - 将分支引用指向新创建的提交
*/
// 提交文件 test.txt文件，内容是hello，提交一个commit
func createCommit(branchName string) {
	fmt.Printf("=== 在分支 %s 创建提交 ===\n", branchName)

	// 直接打开裸仓库
	repo, err := git.PlainOpen(serverRepoPath)
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

	fmt.Printf(" 在分支 %s 成功创建提交，commit hash: %s\n", branchName, commitHash)
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

// 在裸仓，将分支branchName向master进行合并，
// 如果支持Fast-Forward Merge，使用Fast-Forward Merge;
// 否则使用普通 Merge；
// 如果合并冲突，将返回失败
func mergeBranch(branchName string) error {
	fmt.Printf("=== 合并分支 %s 到 master ===\n", branchName)

	// 打开裸仓库
	repo, err := git.PlainOpen(serverRepoPath)
	if err != nil {
		return fmt.Errorf("打开仓库失败：%v", err)
	}

	// 获取 master 分支引用
	masterRefName := plumbing.NewBranchReferenceName("master")
	masterRef, err := repo.Reference(masterRefName, true)
	if err != nil {
		return fmt.Errorf("获取 master 分支引用失败：%v", err)
	}

	// 获取要合并的分支引用
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef, err := repo.Reference(branchRefName, true)
	if err != nil {
		return fmt.Errorf("获取分支 %s 引用失败：%v", branchName, err)
	}

	// 获取 master 分支的提交对象
	masterCommit, err := repo.CommitObject(masterRef.Hash())
	if err != nil {
		return fmt.Errorf("获取 master 分支提交对象失败：%v", err)
	}

	// 获取要合并分支的提交对象
	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return fmt.Errorf("获取分支 %s 提交对象失败：%v", branchName, err)
	}

	// 检查是否可以进行 Fast-Forward 合并
	if isAncestor(repo, masterCommit.Hash, branchCommit.Hash) {
		// Fast-Forward 合并
		fmt.Printf(" 执行 Fast-Forward 合并\n")
		newMasterRef := plumbing.NewHashReference(masterRefName, branchRef.Hash())
		if err := repo.Storer.SetReference(newMasterRef); err != nil {
			return fmt.Errorf("Fast-Forward 合并失败：%v", err)
		}
		fmt.Printf(" Fast-Forward 合并成功：master 指向 %s\n", branchRef.Hash())
		return nil
	}

	// 普通合并（创建合并提交）
	fmt.Printf(" 执行普通合并（创建合并提交）\n")

	// 获取两个分支的 tree
	masterTree, err := masterCommit.Tree()
	if err != nil {
		return fmt.Errorf("获取 master 分支 tree 失败：%v", err)
	}

	branchTree, err := branchCommit.Tree()
	if err != nil {
		return fmt.Errorf("获取分支 %s tree 失败：%v", branchName, err)
	}

	// 合并 tree（这里简化处理，实际需要处理冲突）
	mergedTreeHash, err := mergeTrees(repo, masterTree, branchTree)
	if err != nil {
		return fmt.Errorf("合并 tree 失败：%v", err)
	}

	// 创建合并提交
	mergeCommit := &object.Commit{
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
		Message:      fmt.Sprintf("Merge branch '%s' into master", branchName),
		TreeHash:     mergedTreeHash,
		ParentHashes: []plumbing.Hash{masterCommit.Hash, branchCommit.Hash},
	}

	// 编码并存储合并提交
	obj := repo.Storer.NewEncodedObject()
	if err := mergeCommit.Encode(obj); err != nil {
		return fmt.Errorf("编码合并提交失败：%v", err)
	}

	mergeCommitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("存储合并提交失败：%v", err)
	}

	// 更新 master 分支引用
	newMasterRef := plumbing.NewHashReference(masterRefName, mergeCommitHash)
	if err := repo.Storer.SetReference(newMasterRef); err != nil {
		return fmt.Errorf("更新 master 分支引用失败：%v", err)
	}

	fmt.Printf(" 普通合并成功：master 指向合并提交 %s\n", mergeCommitHash)
	return nil
}

// 检查 commit2 是否是 commit1 的后代（即 commit1 是否是 commit2 的祖先）
func isAncestor(repo *git.Repository, commit1Hash, commit2Hash plumbing.Hash) bool {
	// 获取 commit2 的所有祖先
	_, err := repo.CommitObject(commit2Hash)
	if err != nil {
		return false
	}

	// 遍历 commit2 的所有祖先，看是否包含 commit1
	currentHash := commit2Hash
	for {
		if currentHash == commit1Hash {
			return true
		}

		commit, err := repo.CommitObject(currentHash)
		if err != nil {
			break
		}

		if len(commit.ParentHashes) == 0 {
			break
		}

		// todo 只检查第一个父提交（简化处理）
		currentHash = commit.ParentHashes[0]
	}

	return false
}

// todo 合并两个 tree（简化版本，实际需要处理文件冲突）
func mergeTrees(repo *git.Repository, tree1, tree2 *object.Tree) (plumbing.Hash, error) {
	// 创建新的 tree entries
	var mergedEntries []object.TreeEntry

	// 添加 tree1 的所有条目
	entryMap := make(map[string]object.TreeEntry)
	for _, entry := range tree1.Entries {
		entryMap[entry.Name] = entry
	}

	// 添加 tree2 的条目，如果有冲突则使用 tree2 的版本（简化处理）
	for _, entry := range tree2.Entries {
		entryMap[entry.Name] = entry
	}

	// 转换为 slice
	for _, entry := range entryMap {
		mergedEntries = append(mergedEntries, entry)
	}

	// 创建新的 tree 对象
	mergedTree := &object.Tree{
		Entries: mergedEntries,
	}

	// 编码并存储 tree 对象
	obj := repo.Storer.NewEncodedObject()
	if err := mergedTree.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("编码合并 tree 失败：%v", err)
	}

	treeHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("存储合并 tree 失败：%v", err)
	}

	return treeHash, nil
}
