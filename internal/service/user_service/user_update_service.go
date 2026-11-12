package userservice

import (
	"context"
	"errors"
	"time"

	"gochat/global"
	usermodel "gochat/internal/model/user_model"
)

// UpdateProfileInput 是「更新资料」的入参。
//
// 【核心设计：全部用指针】
// 为什么不用值类型？因为要区分「用户没传这个字段」和「用户传了这个字段但值是零值」。
//
//	例：用户只想改昵称，请求体是 {"nickname":"新名字"}
//	    如果用值类型，Avatar 字段是 ""，直接 Updates 会把头像清空！
//	    用指针：Avatar == nil 表示「没传」，就不更新；Avatar != nil 表示「传了」，才更新。
//
// 【面试点】
//   - GORM 部分更新（partial update）的坑：结构体的零值会被忽略，导致「想清空某个字段」做不到，
//     或者「没传的字段被意外覆盖」。标准解法是「指针 / map / Select 指定字段」三选一。
type UpdateProfileInput struct {
	Nickname  *string    `json:"nickname"`
	Avatar    *string    `json:"avatar"`
	Signature *string    `json:"signature"`
	Gender    *int8      `json:"gender"`
	Region    *string    `json:"region"`
	Birthday  *time.Time `json:"birthday"`
}

// UpdateProfile 部分更新用户资料。
// 这里用 map[string]any 组装更新字段（map 里 false/0/"" 不会被 GORM 忽略），
// 相比 Updates(struct) 更安全。
func UpdateProfile(ctx context.Context, userID uint64, in UpdateProfileInput) error {
	updates := map[string]any{}
	if in.Nickname != nil {
		updates["nickname"] = *in.Nickname
	}
	if in.Avatar != nil {
		updates["avatar"] = *in.Avatar
	}
	if in.Signature != nil {
		updates["signature"] = *in.Signature
	}
	if in.Gender != nil {
		updates["gender"] = *in.Gender
	}
	if in.Region != nil {
		updates["region"] = *in.Region
	}
	if in.Birthday != nil {
		updates["birthday"] = *in.Birthday
	}
	if len(updates) == 0 {
		return errors.New("没有需要更新的字段")
	}

	return global.DB.WithContext(ctx).
		Model(&usermodel.User{}).
		Where("id = ?", userID).
		Updates(updates).Error
}

// GetSettings 查询用户设置（注册时会自动创建默认设置，所以这里正常都能查到）。
func GetSettings(ctx context.Context, userID uint64) (*usermodel.UserSettings, error) {
	var s usermodel.UserSettings
	if err := global.DB.WithContext(ctx).Where("user_id = ?", userID).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateSettingsInput 同样用指针区分「没传」和「传了 false」。
// 注意：bool 的零值是 false，如果用值类型，用户「关闭某开关」和「没传」就分不清了。
type UpdateSettingsInput struct {
	AllowSearchByPhone  *bool `json:"allow_search_by_phone"`
	AllowFriendRequest  *bool `json:"allow_friend_request"`
	NeedVerify          *bool `json:"need_verify"`
	ShowReadReceipt     *bool `json:"show_read_receipt"`
	NotificationEnabled *bool `json:"notification_enabled"`
}

// UpdateSettings 部分更新用户设置。
func UpdateSettings(ctx context.Context, userID uint64, in UpdateSettingsInput) error {
	updates := map[string]any{}
	if in.AllowSearchByPhone != nil {
		updates["allow_search_by_phone"] = *in.AllowSearchByPhone
	}
	if in.AllowFriendRequest != nil {
		updates["allow_friend_request"] = *in.AllowFriendRequest
	}
	if in.NeedVerify != nil {
		updates["need_verify"] = *in.NeedVerify
	}
	if in.ShowReadReceipt != nil {
		updates["show_read_receipt"] = *in.ShowReadReceipt
	}
	if in.NotificationEnabled != nil {
		updates["notification_enabled"] = *in.NotificationEnabled
	}
	if len(updates) == 0 {
		return errors.New("没有需要更新的字段")
	}

	return global.DB.WithContext(ctx).
		Model(&usermodel.UserSettings{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
}
