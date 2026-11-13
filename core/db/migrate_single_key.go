package db

import (
	"fmt"
	"log"

	"gochat/global"
)

// BackfillSingleConversationKeys 为历史单聊会话回填 single_key。
//
// 【为什么需要】
// 新增 single_key 字段后，老的单聊会话该列是 NULL。
// 如果不回填，同一个用户再次点开聊天时会查不到旧会话，从而又建一条新的——相当于制造重复。
//
// 【幂等性】
// 只更新 single_key IS NULL 的行，重复执行不会覆盖已有值。
//
// 【历史重复会话怎么处理】
// 如果数据库里已经存在「同一对用户两条会话」（旧实现的并发 bug 造成），
// 较早的那条会占住唯一键，较晚的更新时会冲突 → 这里跳过并计数，不删除用户数据。
// 生产环境若要清理，应该先做数据核对，再软删多余会话。
func BackfillSingleConversationKeys() error {
	type pairRow struct {
		ConversationID uint
		UserA          uint
		UserB          uint
	}

	var rows []pairRow
	err := global.DB.Raw(`
		SELECT m.conversation_id AS conversation_id,
		       MIN(m.user_id)     AS user_a,
		       MAX(m.user_id)     AS user_b
		FROM conversation_members m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.type = 1 AND c.deleted_at IS NULL
		GROUP BY m.conversation_id
		HAVING COUNT(*) = 2
		ORDER BY m.conversation_id ASC
	`).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("查询历史单聊会话失败: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	updated, skipped := 0, 0
	for _, r := range rows {
		key := fmt.Sprintf("%d:%d", r.UserA, r.UserB)
		res := global.DB.Exec(
			"UPDATE conversations SET single_key = ? WHERE id = ? AND single_key IS NULL",
			key, r.ConversationID,
		)
		if res.Error != nil {
			// 唯一索引冲突：说明存在历史重复会话，保留较早那条
			skipped++
			continue
		}
		if res.RowsAffected > 0 {
			updated++
		}
	}

	if updated > 0 || skipped > 0 {
		log.Printf("[migrate] 单聊会话 single_key 回填完成：更新 %d 条，跳过历史重复 %d 条", updated, skipped)
	}
	return nil
}
