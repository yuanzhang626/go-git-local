package main

import (
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/object"
	"os"
	"path/filepath"
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

	return nil
}
