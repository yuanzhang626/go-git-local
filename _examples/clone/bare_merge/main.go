package main

import (
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"io"
	"os"
	"path/filepath"
)

func main() {
	// 1. 定义目录路径
	baseDir, _ := os.Getwd()
	serverRepoPath := filepath.Join(baseDir, "server-repo.git") // 服务端裸仓库
	clientRepoPath := filepath.Join(baseDir, "client-repo")     // 客户端本地仓库

	// 清空旧目录（方便重复测试）
	_ = os.RemoveAll(serverRepoPath)
	_ = os.RemoveAll(clientRepoPath)

	fmt.Println("=== 1. 服务端：初始化裸仓库 ===")
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
	err = os.WriteFile(readmeFile, []byte("# Initial Repository\n\nThis is an initial repository."), 0644)
	if err != nil {
		panic(fmt.Sprintf("创建README失败：%v", err))
	}

	// 添加文件
	_, err = worktree.Add("README.md")
	if err != nil {
		panic(fmt.Sprintf("添加文件失败：%v", err))
	}

	// 提交
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
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

	fmt.Println("✅ 服务端裸仓库创建完成：", serverRepoPath)

	fmt.Println("\n=== 2. 客户端：克隆仓库 ===")
	// 克隆裸仓库
	var clientRepo *git.Repository
	clientRepo, err = git.PlainClone(clientRepoPath, &git.CloneOptions{
		URL: serverRepoPath, // 本地文件路径作为仓库地址
	})
	if err != nil {
		panic(fmt.Sprintf("客户端克隆失败：%v", err))
	}
	fmt.Println("✅ 客户端克隆完成：", clientRepoPath)

	// 获取工作树
	worktree, err = clientRepo.Worktree()
	if err != nil {
		panic(err)
	}

	fmt.Println("\n=== 3. 客户端：创建新分支 ===")
	// 创建并切换到新分支 feature/test
	branchName := "feature/test"
	headRef, err := clientRepo.Head()
	if err != nil {
		panic(err)
	}
	// 基于当前最新提交创建新分支
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), headRef.Hash())
	err = clientRepo.Storer.SetReference(ref)
	if err != nil {
		panic(fmt.Sprintf("创建分支失败：%v", err))
	}
	// 切换到新分支
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: ref.Name(),
	})
	if err != nil {
		panic(fmt.Sprintf("切换分支失败：%v", err))
	}
	fmt.Println("✅ 新分支创建并切换成功：", branchName)

	fmt.Println("\n=== 4. 客户端：创建文件并提交 ===")
	// 在工作区创建测试文件
	testFile := filepath.Join(clientRepoPath, "hello.txt")
	err = os.WriteFile(testFile, []byte("hello go-git!"), 0644)
	if err != nil {
		panic(err)
	}

	// 添加到暂存区
	_, err = worktree.Add("hello.txt")
	if err != nil {
		panic(fmt.Sprintf("添加文件失败：%v", err))
	}

	// 提交
	commitHash, err := worktree.Commit("feat: add hello.txt", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
		},
	})
	if err != nil {
		panic(fmt.Sprintf("提交失败：%v", err))
	}
	fmt.Println("✅ 文件提交成功，commit：", commitHash.String())

	fmt.Println("\n=== 5. 客户端：推送新分支到服务端 ===")
	// 推送分支到 origin
	err = clientRepo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{
			// 推送本地分支到远程同名分支
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName)),
		},
	})
	if err != nil {
		panic(fmt.Sprintf("推送失败：%v", err))
	}
	fmt.Println("✅ 分支推送至服务端成功！")

	fmt.Println("\n=== 6. 客户端：切换回master分支 ===")
	// 切换回master分支
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	})
	if err != nil {
		panic(fmt.Sprintf("切换到master分支失败：%v", err))
	}
	fmt.Println("✅ 成功切换到master分支")

	fmt.Println("\n=== 7. 客户端：合并feature/test分支到master ===")
	// 获取feature/test分支的引用
	branchRef, err := clientRepo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		panic(fmt.Sprintf("获取分支引用失败：%v", err))
	}

	// 获取feature/test分支的提交对象
	_, err = clientRepo.CommitObject(branchRef.Hash())
	if err != nil {
		panic(fmt.Sprintf("获取分支提交失败：%v", err))
	}

	// 在go-git中，我们需要先切换到feature分支，获取文件，然后切换回master并添加
	// 保存当前master状态
	currentHash, err := clientRepo.Head()
	if err != nil {
		panic(fmt.Sprintf("获取当前HEAD失败：%v", err))
	}

	// 切换到feature分支
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		panic(fmt.Sprintf("切换到feature分支失败：%v", err))
	}

	// 获取hello.txt文件的内容
	featureFile, err := worktree.Filesystem.Open("hello.txt")
	if err != nil {
		panic(fmt.Sprintf("打开feature分支文件失败：%v", err))
	}
	featureContent, err := io.ReadAll(featureFile)
	featureFile.Close()
	if err != nil {
		panic(fmt.Sprintf("读取feature分支文件失败：%v", err))
	}

	// 切换回master分支
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	})
	if err != nil {
		panic(fmt.Sprintf("切换回master分支失败：%v", err))
	}

	// 在master分支创建hello.txt文件
	helloFile := filepath.Join(clientRepoPath, "hello.txt")
	err = os.WriteFile(helloFile, featureContent, 0644)
	if err != nil {
		panic(fmt.Sprintf("在master创建文件失败：%v", err))
	}

	// 添加文件到暂存区
	_, err = worktree.Add("hello.txt")
	if err != nil {
		panic(fmt.Sprintf("添加文件到暂存区失败：%v", err))
	}

	// 创建合并提交
	mergeCommit, err := worktree.Commit(fmt.Sprintf("Merge branch '%s' into master", branchName), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test-user",
			Email: "test@example.com",
		},
		Parents: []plumbing.Hash{
			currentHash.Hash(), // 当前master分支
			branchRef.Hash(),   // 要合并的feature/test分支
		},
	})
	if err != nil {
		panic(fmt.Sprintf("合并提交失败：%v", err))
	}
	fmt.Printf("✅ 合并成功！合并提交：%s\n", mergeCommit.String()[:7])

	fmt.Println("\n=== 8. 客户端：推送合并后的master分支到服务端 ===")
	// 推送合并后的master分支
	err = clientRepo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/master:refs/heads/master"),
		},
	})
	if err != nil {
		panic(fmt.Sprintf("推送master分支失败：%v", err))
	}
	fmt.Println("✅ master分支推送至服务端成功！")

	fmt.Println("\n=== 9. 验证：查看合并后的提交历史 ===")
	// 获取master分支的HEAD引用
	headRef, err = clientRepo.Head()
	if err != nil {
		panic(fmt.Sprintf("获取HEAD引用失败：%v", err))
	}

	// 获取提交历史
	var commitIter object.CommitIter
	commitIter, err = clientRepo.Log(&git.LogOptions{
		From:  headRef.Hash(),
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		panic(fmt.Sprintf("获取提交历史失败：%v", err))
	}
	defer commitIter.Close()

	fmt.Println("📝 提交历史：")
	err = commitIter.ForEach(func(commit *object.Commit) error {
		fmt.Printf("  • %s - %s\n", commit.Hash.String()[:7], commit.Message)
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("遍历提交历史失败：%v", err))
	}

	fmt.Println("\n=== 10. 验证：查看分支状态 ===")
	// 列出所有分支
	branchesIter, err := clientRepo.Branches()
	if err != nil {
		panic(fmt.Sprintf("获取分支列表失败：%v", err))
	}

	fmt.Println("🌿 分支列表：")
	err = branchesIter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branchName := ref.Name().Short()
			if headRef.Hash() == ref.Hash() {
				fmt.Printf("  * %s (当前分支)\n", branchName)
			} else {
				fmt.Printf("    %s\n", branchName)
			}
		}
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("遍历分支失败：%v", err))
	}

	fmt.Println("\n🎉 全部流程执行完成！新分支已成功合并到master！")
}
