package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// SeedanceAssetGroup 素材组（AIGC 数字人 / LivenessFace 真人人像）
type SeedanceAssetGroup struct {
	ID              int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          int    `json:"user_id" gorm:"index"`
	ChannelID       int    `json:"channel_id" gorm:"index"`
	UpstreamGroupID string `json:"upstream_group_id" gorm:"type:varchar(191);index"`
	Name            string `json:"name" gorm:"type:varchar(255)"`
	Description     string `json:"description" gorm:"type:text"`
	GroupType       string `json:"group_type" gorm:"type:varchar(50)"` // AIGC or LivenessFace
	RawData         string `json:"raw_data" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       int64  `json:"deleted_at" gorm:"index;default:0"`
}

func (SeedanceAssetGroup) TableName() string {
	return "seedance_asset_groups"
}

// SeedanceAsset 素材
type SeedanceAsset struct {
	ID              int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          int    `json:"user_id" gorm:"index"`
	ChannelID       int    `json:"channel_id" gorm:"index"`
	UpstreamAssetID string `json:"upstream_asset_id" gorm:"type:varchar(191);index"`
	UpstreamGroupID string `json:"upstream_group_id" gorm:"type:varchar(191);index"`
	Name            string `json:"name" gorm:"type:varchar(255)"`
	AssetType       string `json:"asset_type" gorm:"type:varchar(20)"` // Image / Video / Audio
	SourceURL       string `json:"source_url" gorm:"type:text"`
	Status          string `json:"status" gorm:"type:varchar(30);index"` // Processing / Active / Failed
	RawData         string `json:"raw_data" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       int64  `json:"deleted_at" gorm:"index;default:0"`
}

func (SeedanceAsset) TableName() string {
	return "seedance_assets"
}

// ---- SeedanceAssetGroup CRUD ----

func CreateSeedanceAssetGroup(g *SeedanceAssetGroup) error {
	return DB.Create(g).Error
}

func GetSeedanceAssetGroupByID(id int64, userID int) (*SeedanceAssetGroup, error) {
	var g SeedanceAssetGroup
	err := DB.Where("id = ? AND user_id = ? AND deleted_at = 0", id, userID).First(&g).Error
	return &g, err
}

func GetSeedanceAssetGroupByUpstreamID(upstreamGroupID string, userID int, channelID int) (*SeedanceAssetGroup, error) {
	var g SeedanceAssetGroup
	err := DB.Where("upstream_group_id = ? AND user_id = ? AND channel_id = ? AND deleted_at = 0", upstreamGroupID, userID, channelID).First(&g).Error
	return &g, err
}

func ListSeedanceAssetGroups(userID int, page, pageSize int) ([]*SeedanceAssetGroup, int64, error) {
	var groups []*SeedanceAssetGroup
	var total int64
	query := DB.Model(&SeedanceAssetGroup{}).Where("user_id = ? AND deleted_at = 0", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&groups).Error
	return groups, total, err
}

// ListSeedanceAssetGroupsByChannel 按用户+渠道查询素材组，避免跨渠道引用无效 group。
func ListSeedanceAssetGroupsByChannel(userID int, channelID int, page, pageSize int) ([]*SeedanceAssetGroup, int64, error) {
	var groups []*SeedanceAssetGroup
	var total int64
	query := DB.Model(&SeedanceAssetGroup{}).Where("user_id = ? AND channel_id = ? AND deleted_at = 0", userID, channelID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&groups).Error
	return groups, total, err
}

func UpdateSeedanceAssetGroup(g *SeedanceAssetGroup) error {
	return DB.Save(g).Error
}

func SoftDeleteSeedanceAssetGroupByUpstreamID(upstreamGroupID string, userID int) error {
	return DB.Model(&SeedanceAssetGroup{}).
		Where("upstream_group_id = ? AND user_id = ? AND deleted_at = 0", upstreamGroupID, userID).
		Update("deleted_at", time.Now().Unix()).Error
}

func SoftDeleteSeedanceAssetGroup(id int64, userID int) error {
	return DB.Model(&SeedanceAssetGroup{}).
		Where("id = ? AND user_id = ? AND deleted_at = 0", id, userID).
		Update("deleted_at", time.Now().Unix()).Error
}

// ---- SeedanceAsset CRUD ----

func CreateSeedanceAsset(a *SeedanceAsset) error {
	return DB.Create(a).Error
}

func GetSeedanceAssetByID(id int64, userID int) (*SeedanceAsset, error) {
	var a SeedanceAsset
	err := DB.Where("id = ? AND user_id = ? AND deleted_at = 0", id, userID).First(&a).Error
	return &a, err
}

func GetSeedanceAssetByUpstreamID(upstreamAssetID string, userID int, channelID int) (*SeedanceAsset, error) {
	var a SeedanceAsset
	err := DB.Where("user_id = ? AND deleted_at = 0 AND channel_id = ? AND upstream_asset_id = ?", userID, channelID, upstreamAssetID).First(&a).Error
	return &a, err
}

func GetSeedanceAssetBySourceURL(sourceURL string, userID int, channelID int) (*SeedanceAsset, error) {
	var a SeedanceAsset
	err := DB.Where("user_id = ? AND deleted_at = 0 AND channel_id = ? AND source_url = ?", userID, channelID, sourceURL).Order("id desc").First(&a).Error
	return &a, err
}

func ListSeedanceAssets(userID int, groupID string, id int64, upstreamAssetID string, page, pageSize int) ([]*SeedanceAsset, int64, error) {
	var assets []*SeedanceAsset
	var total int64
	query := DB.Model(&SeedanceAsset{}).Where("user_id = ? AND deleted_at = 0", userID)
	if groupID != "" {
		query = query.Where("upstream_group_id = ?", groupID)
	}
	if id > 0 {
		query = query.Where("id = ?", id)
	}
	if upstreamAssetID != "" {
		query = query.Where("upstream_asset_id = ?", upstreamAssetID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&assets).Error
	return assets, total, err
}

func UpdateSeedanceAsset(a *SeedanceAsset) error {
	return DB.Save(a).Error
}

func SoftDeleteSeedanceAsset(id int64, userID int) error {
	return DB.Model(&SeedanceAsset{}).
		Where("id = ? AND user_id = ? AND deleted_at = 0", id, userID).
		Update("deleted_at", time.Now().Unix()).Error
}

// ListAllSeedanceAssetGroups 管理员用，不过滤 userID
func ListAllSeedanceAssetGroups(page, pageSize int, userID int) ([]*SeedanceAssetGroup, int64, error) {
	var groups []*SeedanceAssetGroup
	var total int64
	query := DB.Model(&SeedanceAssetGroup{}).Where("deleted_at = 0")
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&groups).Error
	return groups, total, err
}

// ListAllSeedanceAssets 管理员用，不过滤 userID
func ListAllSeedanceAssets(page, pageSize int, userID int, groupID string, id int64, upstreamAssetID string) ([]*SeedanceAsset, int64, error) {
	var assets []*SeedanceAsset
	var total int64
	query := DB.Model(&SeedanceAsset{}).Where("deleted_at = 0")
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if groupID != "" {
		query = query.Where("upstream_group_id = ?", groupID)
	}
	if id > 0 {
		query = query.Where("id = ?", id)
	}
	if upstreamAssetID != "" {
		query = query.Where("upstream_asset_id = ?", upstreamAssetID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&assets).Error
	return assets, total, err
}

// UpdateSeedanceAssetStatus 仅更新状态和 raw_data
func UpdateSeedanceAssetStatus(id int64, status, rawData string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now().Unix(),
	}
	if rawData != "" {
		updates["raw_data"] = rawData
	}
	return DB.Model(&SeedanceAsset{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateSeedanceAssetGroupRaw 更新 raw_data 及可变字段
func UpdateSeedanceAssetGroupRaw(id int64, name, description, rawData string) error {
	updates := map[string]interface{}{
		"raw_data":   rawData,
		"updated_at": time.Now().Unix(),
	}
	if name != "" {
		updates["name"] = name
	}
	if description != "" {
		updates["description"] = description
	}
	return DB.Model(&SeedanceAssetGroup{}).Where("id = ?", id).Updates(updates).Error
}

// ensure common import used
var _ = common.Marshal
