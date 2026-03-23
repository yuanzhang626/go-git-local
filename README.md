# go-git-local


## 初始化 2026-03-23
本次，采用的是方式一。
- 方式一: create a new repository on the command line
echo "# go-git-local" >> README.md
git init   //产生一个本地的.git空仓库
git add README.md
git commit -m "first commit"
git branch -M main  //创建一个main分支(一般git init产生默认分支为 master)
git remote add origin https://github.com/yuanzhang626/go-git-local.git
git push -u origin main  //需要进行权限验证(可先将本地ssh密钥，放入远程仓库)

- 方式二: push an existing repository from the command line
  git remote add origin https://github.com/yuanzhang626/go-git-local.git
  git branch -M main
  git push -u origin main

## go-git相关的demo
### 克隆远程仓库
使用用户账号密码，对仓库进行克隆到本地的操作。
git命令操作: git clone http://yuanz001:zy6198803@sit.gitee.work/yuan_002/yuanz001/test_004.git
代码路径: _examples/clone/auth/basic/username_password
  ./username_password.exe http://sit.gitee.work/yuan_002/yuanz001/test_004.git ./temp yuanz001 zy6198803

说明：output是git clone命令获取的仓库；temp是程序代码获取的仓库。

### 裸仓库
裸仓克隆示例：支持两种存储方式（内存/文件系统）
git命令操作: git clone --bare http://yuanz001:zy6198803@sit.gitee.work/yuan_002/yuanz001/test_004.git
代码路径: _examples/clone/bare
  ./bare.exe  http://sit.gitee.work/yuan_002/yuanz001/test_004.git ./temp yuanz001 zy6198803

### 多代码仓合并
添加存储层的事务支持，
将所有写操作路由到「临时存储（temporal）」，读操作优先读取临时存储（未命中则读基础存储）；
只有调用 Commit() 时，才将临时存储的内容原子性合并到「基础存储（base）」，实现事务的「提交」语义；
若放弃事务，直接丢弃临时存储即可实现「回滚」

代码路径: _examples/storage/go-git-multrepo


