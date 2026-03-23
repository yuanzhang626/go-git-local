package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/go-git/go-git/v6/storage/filesystem"
	//"github.com/go-git/go-git/v6/storage/memory"
	. "github.com/go-git/go-git/v6/_examples"
)

// git clone --bare http://yuanz001:zy6198803@sit.gitee.work/yuan_002/yuanz001/test_004.git
// 裸仓克隆示例：支持两种存储方式（内存/文件系统）
func main() {
	CheckArgs("<url>", "<directory>", "<github_username>", "<github_password>")
	url, directory, username, password := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	repo := url[strings.LastIndex(url, "/"):]
	Info("git clone --bare %s %s", url, directory)

	// 方式1：内存存储（仅在内存中创建裸仓，无本地文件）
	// storage := memory.NewStorage()
	// 方式2：文件系统存储（将裸仓保存到本地目录，模拟 git clone --bare）
	bareRepoPath := filepath.Join(directory, repo) //"./temp/test_004.git" // 裸仓存储路径
	os.MkdirAll(bareRepoPath, 0755)
	fs := osfs.New(bareRepoPath)
	storage := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	// 2. 执行裸仓克隆（核心：工作目录为 nil）
	// Clone 参数说明：
	// - storage: 裸仓的存储实现（内存/文件系统）
	// - nil: 工作目录设为 nil（无工作目录 = 裸仓）
	// - CloneOptions: 克隆配置（URL、认证、深度等）
	r, err := git.Clone(storage, nil, &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout, // 打印克隆进度（可选）
		// 如需认证（私有仓库），添加 Auth 配置：
		Auth: &http.BasicAuth{
			Username: username,
			Password: password,
		},
		// Depth: 1, // 浅克隆（可选）
	})
	if err != nil {
		fmt.Printf("裸仓克隆失败: %v\n", err)
		return
	}

	// 3. 验证裸仓（示例：打印 HEAD 引用）
	ref, err := r.Head()
	if err != nil {
		fmt.Printf("获取 HEAD 失败: %v\n", err)
		return
	}
	fmt.Printf("裸仓克隆成功！HEAD 指向: %s\n", ref.Hash())
}
