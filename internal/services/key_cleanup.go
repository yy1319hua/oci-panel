package services

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
)

// keysDir 是 OCI 私钥文件的存放目录（相对进程工作目录）。
// 与 util.KeyFilePath、UploadKey 中使用的路径保持一致。
const keysDir = "./keys"

// keyFileExts 是允许被清理的私钥扩展名。
// 只处理这两类，其他任何文件（.txt/.bak/子目录等）一律不碰。
var keyFileExts = map[string]struct{}{
	".pem": {},
	".key": {},
}

// CleanupOrphanKeys 删除 keys 目录中「不再被任何 OCI 配置引用」的私钥文件。
//
// 【孤儿是怎么产生的】
// UploadKey 每次上传都生成一个新的 UUID 文件名，且不清理旧文件；更换配置里的
// key 时旧文件也会留下。久而久之 keys 目录会堆积一堆无人引用的私钥——
// 既是磁盘垃圾，也是不该长期留存的敏感凭据。
//
// 【安全设计：宁可漏删，绝不误删】
// 这是「删除用户密钥」的危险操作，任何误判都可能让在用的配置失效。因此：
//  1. 查库失败 → 直接放弃本轮清理（绝不基于不完整数据做删除）；
//  2. 只删扩展名在白名单内的文件，其余一律跳过；
//  3. 只比较文件名（basename），与数据库里存的 oci_key_path 保持一致；
//  4. 删除失败（如权限/占用）只记日志，不中断、不返回错误。
//
// 返回被清理的文件名列表，便于调用方记录或测试断言。
func CleanupOrphanKeys() []string {
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("清理孤儿密钥：读取 %s 失败，跳过本轮：%v", keysDir, err)
		}
		return nil
	}

	// 收集「在用」的文件名集合。查库失败即放弃清理。
	referenced := make(map[string]struct{})
	if database.GetDB() == nil {
		log.Printf("清理孤儿密钥：数据库不可用，跳过本轮")
		return nil
	}
	var users []models.OciUser
	if err := database.GetDB().Select("oci_key_path").Find(&users).Error; err != nil {
		log.Printf("清理孤儿密钥：查询配置失败，跳过本轮以免误删在用密钥：%v", err)
		return nil
	}
	for _, u := range users {
		name := strings.TrimSpace(u.OciKeyPath)
		if name == "" {
			continue
		}
		// 数据库里存的是 UUID 文件名；稳妥起见取 basename，兼容历史上可能
		// 存过带路径的值。
		referenced[filepath.Base(name)] = struct{}{}
	}

	var removed []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := keyFileExts[ext]; !ok {
			continue // 非私钥文件，绝不碰
		}
		if _, ok := referenced[name]; ok {
			continue // 正在被某个配置使用
		}

		fullPath := filepath.Join(keysDir, name)
		if err := os.Remove(fullPath); err != nil {
			log.Printf("清理孤儿密钥：删除 %s 失败：%v", name, err)
			continue
		}
		removed = append(removed, name)
	}

	if len(removed) > 0 {
		log.Printf("清理孤儿密钥：已删除 %d 个未被引用的私钥文件：%v", len(removed), removed)
	}
	return removed
}
