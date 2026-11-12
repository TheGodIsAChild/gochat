package userservice

import (
	"context"
	"errors"
	"strings"

	"gochat/global"
	usermodel "gochat/internal/model/user_model"
)

// SearchUsers 按用户名/昵称模糊搜索用户。
//
// 【思路】
//  1. 关键词为空直接拒绝（防止扫全表）
//  2. LIKE '%kw%' 同时匹配 username 和 nickname
//  3. 必须排除自己（id <> selfID），否则搜索结果里会看到自己
//  4. 必须限流（limit 上限 50），避免一次拉太多数据
//
// 【注意点】
//   - LIKE '%xxx%' 这种「前后都模糊」的写法【用不上索引】，数据量大时会全表扫描。
//     生产环境要么改成前缀匹配 `xxx%`（能用索引），要么上 ES / 搜索引擎。
//   - 真实产品还要尊重 user_settings 的搜索隐私开关（allow_search_by_phone 等），
//     这里 M1 先简化，只搜用户名/昵称。
//   - 搜索接口天然可被用来批量扫用户，线上必须加限流（如每分钟 10 次）。
//
// 【面试点】
//   - 为什么 LIKE '%x%' 不走索引？因为 B+ 树只能做前缀匹配，前面的 % 让索引失效。
//   - 如何优化模糊搜索？前缀匹配 / 全文索引 / ES。
func SearchUsers(ctx context.Context, keyword string, selfID uint64, limit int) ([]usermodel.User, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	like := "%" + keyword + "%"
	var users []usermodel.User
	err := global.DB.WithContext(ctx).
		Where("(username LIKE ? OR nickname LIKE ?) AND id <> ?", like, like, selfID).
		Limit(limit).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}
