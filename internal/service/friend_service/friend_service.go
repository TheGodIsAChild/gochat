// Package friendservice 好友业务：申请、列表、删除、备注、黑名单。
//
// 本文件集中体现了 M1 的「业务含金量」：
//  1. 好友关系「双向两行」的设计
//  2. 接受申请必须用【事务】
//  3. 用「条件更新 + RowsAffected」天然抗并发重复处理
//  4. 软删除（status）与唯一索引冲突的坑，以及恢复技巧
package friendservice

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gochat/global"
	friendmodel "gochat/internal/model/friend_model"
	usermodel "gochat/internal/model/user_model"
)

const (
	RequestStatusPending  int8 = 0 // 待处理
	RequestStatusAccepted int8 = 1 // 已接受
	RequestStatusRejected int8 = 2 // 已拒绝

	FriendshipStatusNormal  int8 = 1 // 好友关系正常
	FriendshipStatusDeleted int8 = 2 // 好友关系已删除（软删）
)

// ============================ 发送好友申请 ============================

// SendRequest 发送好友申请。
//
// 【业务规则】（真实产品都会拦这些，面试时能说出来是加分项）
//  1. 不能加自己
//  2. 目标用户必须存在
//  3. 不能已经是好友
//  4. 不能重复发送「待处理」的申请（被拒绝后可以重新申请，所以只查 status=0）
//  5. 对方把我拉黑了 → 拒绝（且不告诉对方具体原因，避免暴露拉黑关系）
//  6. 对方关闭了「允许加好友」设置
//
// 【注意点：先查后写的竞态】
// 上面 3/4/5 都是「先查后写」，并发下两个请求可能同时通过检查。
// 真正兜底的是数据库唯一索引 uk_from_to(from_user_id,to_user_id) —— 让第二个插入失败。
// 「主动查重」只是为了给用户友好提示，不是安全边界。
//
// 【面试点】
//   - 为什么不能只靠查重？因为 check-then-act 不是原子操作，属于典型 TOCTOU 竞态。
//   - 唯一索引是最后一道防线，业务层要把 Duplicate 错误翻译成友好提示。
func SendRequest(ctx context.Context, fromID, toID uint64, message, source string) error {
	if fromID == toID {
		return errors.New("不能添加自己为好友")
	}

	// 2. 目标用户必须存在（软删除用户会被 GORM 自动过滤）
	var target usermodel.User
	if err := global.DB.WithContext(ctx).First(&target, toID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}

	var cnt int64

	// 3. 不能已经是好友（好友关系双向两行，查一行即可判断）
	if err := global.DB.WithContext(ctx).Model(&friendmodel.Friendship{}).
		Where("user_id = ? AND friend_id = ? AND status = ?", fromID, toID, FriendshipStatusNormal).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return errors.New("你们已经是好友了")
	}

	// 4. 不能重复发送待处理申请
	if err := global.DB.WithContext(ctx).Model(&friendmodel.FriendRequest{}).
		Where("from_user_id = ? AND to_user_id = ? AND status = ?", fromID, toID, RequestStatusPending).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return errors.New("已发送过好友申请，请等待对方处理")
	}

	// 5. 对方是否把我拉黑了（blacklist: user_id=拉黑者, blocked_user_id=被拉黑者）
	if err := global.DB.WithContext(ctx).Model(&friendmodel.Blacklist{}).
		Where("user_id = ? AND blocked_user_id = ?", toID, fromID).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		// 统一模糊提示，不暴露「你被拉黑了」这个事实
		return errors.New("无法向该用户发送好友申请")
	}

	// 6. 对方的好友申请开关（user_settings 1:1，注册时已创建）
	var settings usermodel.UserSettings
	if err := global.DB.WithContext(ctx).Where("user_id = ?", toID).First(&settings).Error; err == nil {
		if !settings.AllowFriendRequest {
			return errors.New("对方已关闭好友申请")
		}
	}

	// 7. 落库
	//
	// 【关键坑：唯一索引 uk_from_to 只允许同一对用户存在一行申请记录】
	// 如果无脑 Create，那么两人之前申请过（已接受 / 已拒绝）时，再次申请会撞唯一索引，
	// 表现为「删除好友后重新加好友失败」。
	// 正确做法：先 Unscoped 查旧行，存在就复用（重置为待处理），不存在才插入。
	var existing friendmodel.FriendRequest
	err := global.DB.WithContext(ctx).Unscoped().
		Where("from_user_id = ? AND to_user_id = ?", fromID, toID).
		First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		req := &friendmodel.FriendRequest{
			FromUserID: uint(fromID),
			ToUserID:   uint(toID),
			Message:    message,
			Source:     source,
		}
		if err := global.DB.WithContext(ctx).Create(req).Error; err != nil {
			return errors.New("发送好友申请失败，请稍后重试")
		}
		return nil
	}
	if err != nil {
		return err
	}

	// 复用旧行：重置为待处理，并清除软删除标记
	return global.DB.WithContext(ctx).Unscoped().Model(&existing).Updates(map[string]any{
		"status":     RequestStatusPending,
		"message":    message,
		"source":     source,
		"deleted_at": nil,
	}).Error
}

// ============================ 收到的好友申请 ============================

// RequestItem 是「收到的好友申请」列表项，带上申请人信息，方便前端直接展示。
type RequestItem struct {
	ID         uint      `json:"id"`
	FromUserID uint64    `json:"from_user_id"`
	Username   string    `json:"username"`
	Nickname   string    `json:"nickname"`
	Avatar     string    `json:"avatar"`
	Message    string    `json:"message"`
	Status     int8      `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListIncomingRequests 查询「我收到的、待处理的」好友申请。
//
// 【注意点：避免 N+1 查询】
// 如果遍历申请列表、每条都去查一次用户表，就是 N+1 查询（1 次列表 + N 次用户查询）。
// 正确做法是「先收集所有 from_user_id，再一次性 IN 查询」，总共 2 次查询。
//
// 【面试点】
//   - N+1 查询是什么、怎么发现（慢查询日志/ORM 日志）、怎么优化（批量 IN / JOIN / 预加载）。
func ListIncomingRequests(ctx context.Context, userID uint64) ([]RequestItem, error) {
	var reqs []friendmodel.FriendRequest
	if err := global.DB.WithContext(ctx).
		Where("to_user_id = ? AND status = ?", userID, RequestStatusPending).
		Order("id DESC").
		Find(&reqs).Error; err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return []RequestItem{}, nil
	}

	// 批量查申请人信息（避免 N+1）
	ids := make([]uint, 0, len(reqs))
	for _, r := range reqs {
		ids = append(ids, r.FromUserID)
	}
	var users []usermodel.User
	if err := global.DB.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	userMap := make(map[uint]usermodel.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]RequestItem, 0, len(reqs))
	for _, r := range reqs {
		u := userMap[r.FromUserID]
		items = append(items, RequestItem{
			ID:         r.ID,
			FromUserID: uint64(r.FromUserID),
			Username:   u.Username,
			Nickname:   u.Nickname,
			Avatar:     u.Avatar,
			Message:    r.Message,
			Status:     r.Status,
			CreatedAt:  r.CreatedAt,
		})
	}
	return items, nil
}

// ============================ 接受 / 拒绝 ============================

// AcceptRequest 接受好友申请 —— M1 最典型的【事务】场景。
//
// 【为什么必须用事务】
// 一次「接受」要完成两件事：① 把申请状态改成已接受 ② 插入两条好友关系（A→B、B→A）。
// 如果不用事务：
//   - 改了状态但插好友失败 → 申请显示已通过，但两人还不是好友（数据不一致）
//   - 插了好友但状态没改 → 用户可以再次接受，重复插入
//
// 【为什么要「条件更新 + RowsAffected」】
// 用 WHERE id=? AND to_user_id=? AND status=0 做 UPDATE：
//   - 只有「发给我的、还没处理的」申请才能被更新
//   - 影响行数 = 0 说明被别人抢先处理 / 不是发给我的 / 不存在
//   - 这一步本身就是「原子判断 + 修改」，天然抗并发重复请求（双击、重试都安全）
//
// 【为什么好友关系要插两行】
// A↔B 成为好友后插 (A,B) 和 (B,A)。这样「查我的好友」只需 WHERE user_id=?，
// 不用 OR、不用排序，查询最简单。代价是写入要写两行（所以必须在事务里）。
//
// 【软删除 + 唯一索引的坑】
// friendships 有唯一索引 uk_user_friend(user_id, friend_id)。
// 如果之前删过好友（status=2 但行还在），再次加好友时直接 INSERT 会撞唯一索引。
// 所以这里用 upsertFriendship()：先 Unscoped 查（含软删行），存在就恢复 status=1，不存在才插入。
func AcceptRequest(ctx context.Context, userID, requestID uint64) error {
	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ① 条件更新：原子地把「发给我的待处理申请」改为已接受
		res := tx.Model(&friendmodel.FriendRequest{}).
			Where("id = ? AND to_user_id = ? AND status = ?", requestID, userID, RequestStatusPending).
			Update("status", RequestStatusAccepted)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("申请不存在或已被处理")
		}

		// ② 查出申请人，用于建立双向关系
		var req friendmodel.FriendRequest
		if err := tx.First(&req, requestID).Error; err != nil {
			return err
		}

		// ③ 建立双向好友关系（同一事务，任一失败整体回滚）
		if err := upsertFriendship(tx, req.FromUserID, req.ToUserID); err != nil {
			return err
		}
		return upsertFriendship(tx, req.ToUserID, req.FromUserID)
	})
}

// upsertFriendship 确保 (userID, friendID) 这条好友关系处于「正常」状态。
// 用 Unscoped 查是为了能看到被软删（status=2）的历史行，从而恢复而不是重复插入。
func upsertFriendship(tx *gorm.DB, userID, friendID uint) error {
	var f friendmodel.Friendship
	err := tx.Unscoped().
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		First(&f).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&friendmodel.Friendship{
			UserID:   userID,
			FriendID: friendID,
			Origin:   "request",
			Status:   FriendshipStatusNormal,
		}).Error
	}
	if err != nil {
		return err
	}

	// 已存在（可能已删除）→ 恢复为正常好友
	return tx.Unscoped().Model(&f).Updates(map[string]any{
		"status":     FriendshipStatusNormal,
		"deleted_at": nil,
	}).Error
}

// RejectRequest 拒绝好友申请。
// 同样用「条件更新 + RowsAffected」，重复拒绝/越权拒绝都安全。
func RejectRequest(ctx context.Context, userID, requestID uint64) error {
	res := global.DB.WithContext(ctx).Model(&friendmodel.FriendRequest{}).
		Where("id = ? AND to_user_id = ? AND status = ?", requestID, userID, RequestStatusPending).
		Update("status", RequestStatusRejected)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("申请不存在或已被处理")
	}
	return nil
}

// ============================ 好友列表 / 删除 / 备注 ============================

// FriendItem 是好友列表项，合并了「关系表字段」和「用户表字段」。
type FriendItem struct {
	FriendshipID uint64 `json:"friendship_id"`
	FriendID     uint64 `json:"friend_id"`
	Username     string `json:"username"`
	Nickname     string `json:"nickname"`
	Avatar       string `json:"avatar"`
	Remark       string `json:"remark"`
	GroupID      uint64 `json:"group_id"`
	IsStarred    int8   `json:"is_starred"`
}

// ListFriends 查询我的好友列表（含备注、分组、星标）。
//
// 【注意点】
//   - 只查 status=1 的关系（status=2 表示已删除）
//   - 依然用「批量 IN 查用户」避免 N+1
//   - 排序：星标在前，其次按关系 id
func ListFriends(ctx context.Context, userID uint64) ([]FriendItem, error) {
	var fs []friendmodel.Friendship
	if err := global.DB.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, FriendshipStatusNormal).
		Order("is_starred DESC, id ASC").
		Find(&fs).Error; err != nil {
		return nil, err
	}
	if len(fs) == 0 {
		return []FriendItem{}, nil
	}

	ids := make([]uint, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.FriendID)
	}
	var users []usermodel.User
	if err := global.DB.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	userMap := make(map[uint]usermodel.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]FriendItem, 0, len(fs))
	for _, f := range fs {
		u := userMap[f.FriendID]
		items = append(items, FriendItem{
			FriendshipID: uint64(f.ID),
			FriendID:     uint64(f.FriendID),
			Username:     u.Username,
			Nickname:     u.Nickname,
			Avatar:       u.Avatar,
			Remark:       f.Remark,
			GroupID:      uint64(f.GroupID),
			IsStarred:    f.IsStarred,
		})
	}
	return items, nil
}

// DeleteFriend 删除好友：把【双向两行】都标记为已删除。
// 用事务保证两行同时失效，不会出现「我删了他但他在我列表里还在」的不一致。
func DeleteFriend(ctx context.Context, userID, friendID uint64) error {
	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&friendmodel.Friendship{}).
			Where("user_id = ? AND friend_id = ? AND status = ?", userID, friendID, FriendshipStatusNormal).
			Update("status", FriendshipStatusDeleted)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("好友不存在")
		}
		return tx.Model(&friendmodel.Friendship{}).
			Where("user_id = ? AND friend_id = ? AND status = ?", friendID, userID, FriendshipStatusNormal).
			Update("status", FriendshipStatusDeleted).Error
	})
}

// SetRemark 设置好友备注（只影响「我这一侧」的关系行）。
func SetRemark(ctx context.Context, userID, friendID uint64, remark string) error {
	res := global.DB.WithContext(ctx).Model(&friendmodel.Friendship{}).
		Where("user_id = ? AND friend_id = ? AND status = ?", userID, friendID, FriendshipStatusNormal).
		Update("remark", remark)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("好友不存在")
	}
	return nil
}

// ============================ 黑名单 ============================

// AddBlacklist 拉黑某人。
// 用 OnConflict DoNothing 保证幂等：重复拉黑不报错。
func AddBlacklist(ctx context.Context, userID, blockedID uint64) error {
	if userID == blockedID {
		return errors.New("不能拉黑自己")
	}

	var u usermodel.User
	if err := global.DB.WithContext(ctx).First(&u, blockedID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}

	return global.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&friendmodel.Blacklist{
			UserID:        uint(userID),
			BlockedUserID: uint(blockedID),
		}).Error
}

// RemoveBlacklist 移除黑名单。
//
// 【注意点：必须 Unscoped 硬删除】
// blacklist 表继承了 gorm.Model，Delete 默认是「软删除」（写 deleted_at）。
// 但 uk_user_blocked(user_id, blocked_user_id) 唯一索引依然会挡住下次「重新拉黑」。
// 所以黑名单这种「拉黑/取消」的开关型数据，应该物理删除。
func RemoveBlacklist(ctx context.Context, userID, blockedID uint64) error {
	return global.DB.WithContext(ctx).Unscoped().
		Where("user_id = ? AND blocked_user_id = ?", userID, blockedID).
		Delete(&friendmodel.Blacklist{}).Error
}

// BlacklistItem 黑名单列表项。
type BlacklistItem struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// ListBlacklist 查询我的黑名单。
func ListBlacklist(ctx context.Context, userID uint64) ([]BlacklistItem, error) {
	var rows []friendmodel.Blacklist
	if err := global.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []BlacklistItem{}, nil
	}

	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BlockedUserID)
	}
	var users []usermodel.User
	if err := global.DB.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	userMap := make(map[uint]usermodel.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]BlacklistItem, 0, len(rows))
	for _, r := range rows {
		u := userMap[r.BlockedUserID]
		items = append(items, BlacklistItem{
			UserID:   uint64(r.BlockedUserID),
			Username: u.Username,
			Nickname: u.Nickname,
			Avatar:   u.Avatar,
		})
	}
	return items, nil
}
